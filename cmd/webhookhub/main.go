package main

import (
	"log"
	"net/http"
	"webhookhub/internal/handler"
	"webhookhub/internal/storage"
)

func main() {
	db := storage.InitDB("webhooks.db")
	db.InitForwardingTable()

	mux := http.NewServeMux()
	mux.HandleFunc("/hook/", handler.ReceiveWebhook(db))
	mux.HandleFunc("/api/webhooks", handler.ListWebhooks(db))
	mux.HandleFunc("/api/webhooks/replay", handler.ReplayWebhook(db))
	mux.HandleFunc("/", handler.ServeUI(db))
	mux.HandleFunc("/forwarding", handler.ForwardingUI(db))
	mux.HandleFunc("/forwarding/save", handler.SaveForwardingRule(db))

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
