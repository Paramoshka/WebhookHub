package handler

import (
	"html/template"
	"net/http"
	"webhookhub/internal/storage"
)

func DashboardUI(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl := template.Must(template.ParseFiles(
			"web/templates/base.html",
			"web/templates/dashboard.html",
		))
		tmpl.ExecuteTemplate(w, "base", nil)
	}
}

func DeliveryMetricsPartial(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl := template.Must(template.ParseFiles("web/templates/metrics.html"))
		tmpl.Execute(w, db.DeliveryMetrics())
	}
}
