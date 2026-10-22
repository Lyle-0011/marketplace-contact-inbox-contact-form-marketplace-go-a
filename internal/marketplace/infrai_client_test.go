package marketplace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientUsesBoundariesAndRetriesRateLimit(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost {
			t.Errorf("method = %s; want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header missing")
		}
		if r.URL.Path != "/v1/email/send" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Idempotency-Key") != "form-7" {
			t.Errorf("idempotency key missing")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["body"] != "Body" {
			t.Errorf("body = %#v", payload["body"])
		}
		if _, ok := payload["text"]; ok {
			t.Error("unexpected text field")
		}
		if requests == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]string{"message": "retry later"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]string{"message_id": "msg-9"}, "metadata": map[string]any{}})
	}))
	defer server.Close()

	client := InfraiClient{
		BaseURL: server.URL, APIKey: "test-key", MaxRetries: 2,
		Sleep: func(context.Context, time.Duration) error { return nil },
	}
	messageID, err := client.SendEmail(context.Background(), "form-7", "team@example.com", "Subject", "Body")
	if err != nil {
		t.Fatal(err)
	}
	if messageID != "msg-9" || requests != 2 {
		t.Fatalf("message ID = %q, requests = %d", messageID, requests)
	}
}

func TestClientReturnsFourHundredEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["widget_record_id"] != "widget-1" {
			t.Errorf("widget_record_id = %#v", payload["widget_record_id"])
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": map[string]string{"message": "request rejected"}})
	}))
	defer server.Close()

	client := InfraiClient{BaseURL: server.URL, APIKey: "test-key", WidgetRecordID: "widget-1"}
	err := client.VerifyCaptcha(context.Background(), "token", "192.0.2.1")
	apiErr, ok := err.(*InfraiError)
	if !ok || apiErr.Message != "request rejected" || apiErr.HTTPStatus != http.StatusUnprocessableEntity || apiErr.Path != "/v1/captcha/verify" {
		t.Fatalf("error = %#v", err)
	}
}
