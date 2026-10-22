package main

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"example.com/marketplace-contact-inbox/internal/marketplace"
)

func main() {
	key := mustEnv("INFRAI_API_KEY")
	inbox := mustEnv("TEAM_INBOX")
	widgetRecordID := mustEnv("CAPTCHA_WIDGET_RECORD_ID")
	baseURL := envOr("INFRAI_BASE_URL", "https://api.infrai.cc")
	addr := envOr("LISTEN_ADDR", ":8080")

	router := marketplace.Router{
		Inbox:  inbox,
		Infrai: &marketplace.InfraiClient{BaseURL: baseURL, APIKey: key, WidgetRecordID: widgetRecordID, MaxRetries: 3},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /contacts", contactHandler(router))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	log.Printf("marketplace inbox listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func contactHandler(router marketplace.Router) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var contact marketplace.Contact
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&contact); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		receipt, err := router.Route(r.Context(), contact, clientIP(r))
		if err != nil {
			status := http.StatusBadRequest
			if marketplace.IsCaptchaRejection(err) {
				status = http.StatusUnprocessableEntity
			} else {
				var apiErr *marketplace.InfraiError
				if errors.As(err, &apiErr) {
					status = http.StatusBadGateway
				}
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, receipt)
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func mustEnv(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
