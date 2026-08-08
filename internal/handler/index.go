package handler

import (
	"html/template"
	"net/http"
)

type PageData struct {
	CSRFToken string
}

func ServeIndex() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("web/templates/base.html", "web/templates/index.html")
		if err != nil {
			http.Error(w, "Template load failed", http.StatusInternalServerError)
			return
		}
		err = tmpl.ExecuteTemplate(w, "base", PageData{CSRFToken: CSRFToken(r)})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
