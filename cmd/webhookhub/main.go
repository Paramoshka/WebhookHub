package main

import (
	"log"
	"net/http"
	"os"

	"webhookhub/internal/handler"
	"webhookhub/internal/storage"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ .env file not found or failed to load, continuing...")
	}

	// Init DB (PostgreSQL via GORM)
	db := storage.InitDB()

	// Set up HTTP mux
	mux := http.NewServeMux()

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))

	// Auth
	mux.HandleFunc("/login", handler.Login(db))
	mux.HandleFunc("/logout", handler.Logout())

	// Public hook endpoint
	mux.HandleFunc("/hook/", handler.ReceiveWebhook(db)) // Optionally add auth later

	// Protected
	protected := func(h http.HandlerFunc) http.HandlerFunc {
		return handler.RequireAuth(h)
	}

	mux.HandleFunc("/", protected(handler.ServeIndex(db)))
	mux.HandleFunc("/dashboard", protected(handler.DashboardUI(db)))
	mux.HandleFunc("/api/webhooks", protected(handler.ListWebhooks(db)))
	mux.HandleFunc("/api/webhooks/replay", protected(handler.ReplayWebhook(db)))
	mux.HandleFunc("/api/webhooks/delete", handler.RequireAuth(handler.DeleteWebhook(db)))
	mux.HandleFunc("/partials/webhooks", protected(handler.WebhookPartial(db)))
	mux.HandleFunc("/partials/webhook/", protected(handler.InspectWebhook(db)))

	mux.HandleFunc("/forwarding", protected(handler.ForwardingUI(db)))
	mux.HandleFunc("/forwarding/save", protected(handler.SaveForwardingRule(db)))
	mux.HandleFunc("/forwarding/edit", protected(handler.EditForwardingForm(db)))
	mux.HandleFunc("/forwarding/update", protected(handler.UpdateForwardingRule(db)))
	mux.HandleFunc("/forwarding/delete", protected(handler.DeleteForwardingRule(db)))

	// Start server
	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	log.Println("🚀 Listening on", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
