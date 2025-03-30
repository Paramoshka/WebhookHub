package handler

import (
    "html/template"
    "net/http"
    "webhookhub/internal/storage"
)

func ForwardingUI(db *storage.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        tmpl := template.Must(template.ParseFiles("web/templates/forwarding.html"))
        rules := db.GetForwardingRules()
        tmpl.Execute(w, rules)
    }
}

func SaveForwardingRule(db *storage.DB) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if err := r.ParseForm(); err != nil {
            http.Error(w, "Invalid form", http.StatusBadRequest)
            return
        }

        source := r.FormValue("source")
        target := r.FormValue("target")

        if source != "" && target != "" {
            db.SaveForwardingRule(source, target)
        }

        http.Redirect(w, r, "/forwarding", http.StatusSeeOther)
    }
}