package handler

import (
	"net/http"
	"strings"
	"webhookhub/internal/storage"
)

func DashboardUI(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, page := parseWebhookFilterAndPage(r)

		data := WebhookPageData{
			CurrentURL: buildWebhookListURL(r.URL.Query(), page),
			Source:     strings.TrimSpace(r.URL.Query().Get("source")),
			Status:     strings.TrimSpace(r.URL.Query().Get("status")),
			Query:      strings.TrimSpace(r.URL.Query().Get("q")),
			Sort:       strings.TrimSpace(r.URL.Query().Get("sort")),
			From:       formatDateTimeInput(filter.From),
			To:         formatDateTimeInput(filter.To),
			CSRFToken:  CSRFToken(r),
		}
		if err := dashboardTemplates.ExecuteTemplate(w, "base", data); err != nil {
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
		if err := metricsTemplates.Execute(w, DeliveryMetricsData{DeliveryMetrics: metrics, CSRFToken: CSRFToken(r)}); err != nil {
			http.Error(w, "Template render failed", http.StatusInternalServerError)
		}
	}
}
