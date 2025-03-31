package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"

	"webhookhub/internal/handler"
	"webhookhub/internal/storage"
)

func main() {
	// Load .env if present
	_ = godotenv.Load()

	db := storage.InitDB("webhooks.db")
	db.InitForwardingTable()

	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("/logout", handler.Logout())
	mux.HandleFunc("/login", handler.Login())

	// Protected routes
	mux.HandleFunc("/", handler.RequireAuth(handler.ServeDashboard(db)))
	mux.HandleFunc("/api/webhooks", handler.RequireAuth(handler.ListWebhooks(db)))
	mux.HandleFunc("/api/webhooks/replay", handler.RequireAuth(handler.ReplayWebhook(db)))
	mux.HandleFunc("/partials/webhooks", handler.RequireAuth(handler.WebhookPartial(db)))
	mux.HandleFunc("/partials/webhook/", handler.RequireAuth(handler.InspectWebhook(db)))
	mux.HandleFunc("/forwarding", handler.RequireAuth(handler.ForwardingUI(db)))
	mux.HandleFunc("/forwarding/save", handler.RequireAuth(handler.SaveForwardingRule(db)))
	mux.HandleFunc("/forwarding/edit", handler.RequireAuth(handler.EditForwardingForm(db)))
	mux.HandleFunc("/forwarding/update", handler.RequireAuth(handler.UpdateForwardingRule(db)))
	mux.HandleFunc("/forwarding/delete", handler.RequireAuth(handler.DeleteForwardingRule(db)))

	// Listen
	mux.HandleFunc("/hook/", handler.ReceiveWebhook(db)) // Optionally protect this too

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
