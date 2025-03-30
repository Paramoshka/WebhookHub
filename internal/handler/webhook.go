package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"webhookhub/internal/forwarder"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

func ReceiveWebhook(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := strings.TrimPrefix(r.URL.Path, "/hook/")
		payload, _ := io.ReadAll(r.Body)
		headers, _ := json.Marshal(r.Header)

		webhook := model.Webhook{
			Source:     source,
			Headers:    string(headers),
			Payload:    payload,
			ReceivedAt: time.Now(),
			Status:     "pending",
		}

		db.Save(webhook)
		go forwarder.Forward(db, webhook)

		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("Received"))
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
		if hook, found := db.FindByID(id); found {
			go forwarder.Forward(db, hook)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Replaying"))
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
		}
	}
}
