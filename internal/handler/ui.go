package handler

import (
    "html/template"
    "net/http"
    "webhookhub/internal/storage"
)

func ServeUI(db *storage.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        tmpl := template.Must(template.ParseFiles("web/templates/index.html"))
        data := db.All()
        tmpl.Execute(w, data)
    }
}