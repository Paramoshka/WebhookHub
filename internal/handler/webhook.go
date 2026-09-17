package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"webhookhub/internal/hmacsig"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

var sourcePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type webhookMutationStore interface {
	ResetWebhookDeliveryState(int) error
	DeleteWebhook(int) error
}

func ReceiveWebhook(db *storage.DB, maxBodyBytes int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := strings.TrimPrefix(r.URL.Path, "/hook/")
		if !validSource(source) {
			http.Error(w, "Invalid webhook source", http.StatusBadRequest)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			var maxBytesError *http.MaxBytesError
			if errors.As(err, &maxBytesError) {
				http.Error(w, "Webhook payload too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		now := time.Now()
		rule, err := db.GetForwardingRule(r.Context(), source)
		if err != nil && !errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		if err == nil && rule.VerifySecret != "" {
			headerName := rule.VerifyHeader
			if headerName == "" {
				headerName = hmacsig.DefaultIncomingHeader
			}
			signatureHeader := r.Header.Get(headerName)
			tolerance := time.Duration(rule.ToleranceSeconds) * time.Second
			if err := hmacsig.VerifyHeader(rule.VerifySecret, signatureHeader, payload, now, tolerance); err != nil {
				http.Error(w, "Invalid signature", http.StatusUnauthorized)
				return
			}
		}

		headers, err := json.Marshal(r.Header)
		if err != nil {
			http.Error(w, "Failed to serialize headers", http.StatusInternalServerError)
			return
		}

		webhook := model.Webhook{
			Source:     source,
			Headers:    string(headers),
			Payload:    payload,
			ReceivedAt: now,
			Status:     "pending",
		}

		if err := db.Save(&webhook); err != nil {
			http.Error(w, "Failed to persist webhook", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("Received"))
	}
}

func ListWebhooks(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hooks, err := db.All()
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(hooks); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	}
}

func ReplayWebhook(db webhookMutationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseWebhookID(r)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		if err := db.ResetWebhookDeliveryState(id); err != nil {
			writeWebhookMutationError(w, err, "Failed to requeue webhook")
			return
		}

		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<span>Queued for delivery</span>`)
			return
		}

		http.Redirect(w, r, redirectTarget(r, "/dashboard"), http.StatusSeeOther)
	}
}

func DeleteWebhook(db webhookMutationStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseWebhookID(r)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		if err := db.DeleteWebhook(id); err != nil {
			writeWebhookMutationError(w, err, "Failed to delete webhook")
			return
		}
		if r.Header.Get("HX-Request") == "true" {
			w.WriteHeader(http.StatusOK)
			return
		}

		http.Redirect(w, r, redirectTarget(r, "/dashboard"), http.StatusSeeOther)
	}
}

func parseWebhookID(r *http.Request) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("id")))
	if err != nil || id <= 0 {
		return 0, errors.New("invalid webhook ID")
	}
	return id, nil
}

func writeWebhookMutationError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		http.Error(w, "Not found", http.StatusNotFound)
	case errors.Is(err, storage.ErrWebhookProcessing):
		http.Error(w, "Webhook delivery is in progress", http.StatusConflict)
	default:
		http.Error(w, fallback, http.StatusServiceUnavailable)
	}
}

func redirectTarget(r *http.Request, fallback string) string {
	target := strings.TrimSpace(r.URL.Query().Get("redirect_to"))
	if isLocalRedirect(target) {
		return target
	}

	referer := strings.TrimSpace(r.Referer())
	if parsed, err := url.Parse(referer); err == nil && parsed.Host == r.Host && isLocalRedirect(parsed.RequestURI()) {
		return parsed.RequestURI()
	}

	return fallback
}

func validSource(source string) bool {
	return sourcePattern.MatchString(source)
}

func isLocalRedirect(target string) bool {
	if target == "" || !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") {
		return false
	}
	parsed, err := url.Parse(target)
	return err == nil && parsed.Scheme == "" && parsed.Host == ""
}
