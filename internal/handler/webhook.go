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
		hook, found := db.FindByID(id)
		if !found {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		go forwarder.Forward(db, hook)

		// возвращаем тот же HTML, что и был
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span style="color:green;">✅ Replayed</span>`)
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
		w.WriteHeader(http.StatusOK)
	}
}
