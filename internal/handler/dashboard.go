package handler

import (
	"net/http"
	"strings"
	"webhookhub/internal/storage"
)

func DashboardUI(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, page, err := parseWebhookFilterAndPage(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		data := WebhookPageData{
			CurrentURL:  buildWebhookListURL(canonicalDateQuery(r.URL.Query(), filter.From, filter.To), page),
			Source:      strings.TrimSpace(r.URL.Query().Get("source")),
			Status:      strings.TrimSpace(r.URL.Query().Get("status")),
			Query:       strings.TrimSpace(r.URL.Query().Get("q")),
			Sort:        strings.TrimSpace(r.URL.Query().Get("sort")),
			From:        formatDateTimeInput(filter.From),
			To:          formatDateTimeInput(filter.To),
			FromInstant: dateTimeInstant(filter.From),
			ToInstant:   dateTimeInstant(filter.To),
			CSRFToken:   CSRFToken(r),
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
		metrics, err := db.DeliveryMetrics(r.Context())
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := metricsTemplates.Execute(w, DeliveryMetricsData{DeliveryMetrics: metrics, CSRFToken: CSRFToken(r)}); err != nil {
			http.Error(w, "Template render failed", http.StatusInternalServerError)
		}
	}
}
