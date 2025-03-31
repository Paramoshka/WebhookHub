package handler

import (
	"html/template"
	"net/http"
	"webhookhub/internal/storage"
)

func ServeDashboard(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl := template.Must(template.ParseGlob("web/templates/*.html"))
		err := tmpl.ExecuteTemplate(w, "base", nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
