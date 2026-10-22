package marketplace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type InfraiClient struct {
	BaseURL        string
	APIKey         string
	WidgetRecordID string
	HTTP           *http.Client
	MaxRetries     int
	Sleep          func(context.Context, time.Duration) error
}

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
	Path       string
}

func (e *InfraiError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("infrai %s: %s", e.Code, e.Message)
	}
	return "infrai " + e.Code
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type captchaRequest struct {
	WidgetRecordID string  `json:"widget_record_id"`
	Token          string  `json:"token"`
	Vendor         string  `json:"vendor,omitempty"`
	IP             string  `json:"ip,omitempty"`
	Action         string  `json:"action,omitempty"`
	ScoreThreshold float64 `json:"score_threshold,omitempty"`
}

type emailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type emailResult struct {
	MessageID string `json:"message_id"`
}

func (c *InfraiClient) VerifyCaptcha(ctx context.Context, token, ip string) error {
	return c.post(ctx, "/v1/captcha/verify", captchaRequest{
		WidgetRecordID: c.WidgetRecordID,
		Token:          token,
		IP:             ip,
		Action:         "marketplace_contact",
		ScoreThreshold: 0.5,
	}, "", nil)
}

func (c *InfraiClient) SendEmail(ctx context.Context, idempotencyKey, to, subject, body string) (string, error) {
	var result emailResult
	if err := c.post(ctx, "/v1/email/send", emailRequest{To: to, Subject: subject, Body: body}, idempotencyKey, &result); err != nil {
		return "", err
	}
	return result.MessageID, nil
}

func (c *InfraiClient) post(ctx context.Context, path string, payload any, idempotencyKey string, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	sleep := c.Sleep
	if sleep == nil {
		sleep = sleepContext
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		res, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("call Infrai: %w", err)
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read Infrai response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if !env.OK {
			apiErr := &InfraiError{HTTPStatus: res.StatusCode, Path: path}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
				if apiErr.Message == "" {
					apiErr.Message = env.Error.Hint
				}
			}
			if res.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
				if err := sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return err
				}
				continue
			}
			return apiErr
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode Infrai data: %w", err)
			}
		}
		return nil
	}
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second * time.Duration(1<<attempt)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func IsCaptchaRejection(err error) bool {
	var target *InfraiError
	return errors.As(err, &target) && target.Path == "/v1/captcha/verify" && target.HTTPStatus >= 400 && target.HTTPStatus < 500
}
