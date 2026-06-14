package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"webhookhub/internal/forwarder"
	"webhookhub/internal/hmacsig"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

func ReceiveWebhook(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := strings.TrimPrefix(r.URL.Path, "/hook/")
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		now := time.Now()
		if rule, found := db.GetForwardingRule(source); found && rule.VerifySecret != "" {
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

		db.Save(&webhook)
		go forwarder.Forward(db, &webhook)

		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("Received"))
	}
}

func ListWebhooks(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hooks := db.All()
		json.NewEncoder(w).Encode(hooks)
	}
}

func ReplayWebhook(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		hook, found := db.FindByID(id)
		if !found {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		db.ResetWebhookDeliveryState(int(hook.ID))
		hook.Status = "pending"
		hook.Response = nil

		go forwarder.Forward(db, &hook)

		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<span style="color:green;">✅ Replayed</span>`)
			return
		}

		http.Redirect(w, r, redirectTarget(r, "/dashboard"), http.StatusSeeOther)
	}
}

func DeleteWebhook(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Query().Get("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}

		db.DeleteWebhook(id)
		if r.Header.Get("HX-Request") == "true" {
			w.WriteHeader(http.StatusOK)
			return
		}

		http.Redirect(w, r, redirectTarget(r, "/dashboard"), http.StatusSeeOther)
	}
}

func redirectTarget(r *http.Request, fallback string) string {
	target := strings.TrimSpace(r.URL.Query().Get("redirect_to"))
	if target != "" {
		return target
	}

	referer := strings.TrimSpace(r.Referer())
	if referer != "" {
		return referer
	}

	return fallback
}
