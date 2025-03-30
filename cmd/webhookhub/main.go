package main

import (
	"log"
	"net/http"
	"webhookhub/internal/handler"
	"webhookhub/internal/storage"
)

func main() {
	db := storage.InitDB("webhooks.db")
	mux := http.NewServeMux()
	mux.HandleFunc("/hook/", handler.ReceiveWebhook(db))
	mux.HandleFunc("/api/webhooks", handler.ListWebhooks(db))
	mux.HandleFunc("/api/webhooks/replay", handler.ReplayWebhook(db))
	mux.HandleFunc("/", handler.ServeUI(db))

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
