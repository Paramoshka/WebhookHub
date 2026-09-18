package handler

import (
	"net/http"
)

type PageData struct {
	CSRFToken string
}

func ServeIndex() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := indexTemplates.ExecuteTemplate(w, "base", PageData{CSRFToken: CSRFToken(r)})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
