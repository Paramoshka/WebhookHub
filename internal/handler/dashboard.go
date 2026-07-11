package handler

import (
	"html/template"
	"net/http"
	"strings"
	"webhookhub/internal/storage"
)

func DashboardUI(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl := template.Must(template.ParseFiles(
			"web/templates/base.html",
			"web/templates/dashboard.html",
		))

		data := WebhookPageData{
			Source: strings.TrimSpace(r.URL.Query().Get("source")),
			Status: strings.TrimSpace(r.URL.Query().Get("status")),
			Query:  strings.TrimSpace(r.URL.Query().Get("q")),
			Sort:   strings.TrimSpace(r.URL.Query().Get("sort")),
			From:   strings.TrimSpace(r.URL.Query().Get("from")),
			To:     strings.TrimSpace(r.URL.Query().Get("to")),
		}
		tmpl.ExecuteTemplate(w, "base", data)
	}
}

func DeliveryMetricsPartial(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl := template.Must(template.ParseFiles("web/templates/metrics.html"))
		tmpl.Execute(w, db.DeliveryMetrics())
	}
}
