package handler

import (
	"html/template"
	"net/http"
	"webhookhub/internal/storage"
)

func DashboardUI(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rules := db.GetForwardingRules()
		tmpl := template.Must(template.ParseFiles(
			"web/templates/base.html",
			"web/templates/dashboard.html",
		))
		tmpl.ExecuteTemplate(w, "base", rules)
	}
}
