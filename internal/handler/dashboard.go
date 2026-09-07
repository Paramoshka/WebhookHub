package handler

import (
	"html/template"
	"net/http"
	"strings"
	"webhookhub/internal/storage"
)

func DashboardUI(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles(
			"web/templates/base.html",
			"web/templates/dashboard.html",
		)
		if err != nil {
			http.Error(w, "Template load failed", http.StatusInternalServerError)
			return
		}

		data := WebhookPageData{
			CurrentURL: buildWebhookListURL(r.URL.Query(), 1),
			Source:     strings.TrimSpace(r.URL.Query().Get("source")),
			Status:     strings.TrimSpace(r.URL.Query().Get("status")),
			Query:      strings.TrimSpace(r.URL.Query().Get("q")),
			Sort:       strings.TrimSpace(r.URL.Query().Get("sort")),
			From:       strings.TrimSpace(r.URL.Query().Get("from")),
			To:         strings.TrimSpace(r.URL.Query().Get("to")),
			CSRFToken:  CSRFToken(r),
		}
		if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
			http.Error(w, "Template render failed", http.StatusInternalServerError)
		}
	}
}

type DeliveryMetricsData struct {
	storage.DeliveryMetrics
	CSRFToken string
}

func DeliveryMetricsPartial(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metrics, err := db.DeliveryMetrics()
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		tmpl, err := template.ParseFiles("web/templates/metrics.html")
		if err != nil {
			http.Error(w, "Template load failed", http.StatusInternalServerError)
			return
		}
		if err := tmpl.Execute(w, DeliveryMetricsData{DeliveryMetrics: metrics, CSRFToken: CSRFToken(r)}); err != nil {
			http.Error(w, "Template render failed", http.StatusInternalServerError)
		}
	}
}
