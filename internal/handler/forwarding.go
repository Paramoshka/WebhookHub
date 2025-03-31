package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"webhookhub/internal/storage"
)

func ForwardingUI(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rules := db.GetForwardingRules()
		tmpl := template.Must(template.ParseFiles(
			"web/templates/base.html",
			"web/templates/forwarding.html",
		))
		tmpl.ExecuteTemplate(w, "base", rules)
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

func EditForwardingForm(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := r.URL.Query().Get("source")
		if source == "" {
			http.Error(w, "Missing source", http.StatusBadRequest)
			return
		}

		target := db.GetTargetForSource(source)

		fmt.Fprintf(w, `
        <h3>Edit Rule</h3>
        <form method="POST" action="/forwarding/update">
          <input type="hidden" name="source" value="%s">
          <input type="text" name="target" value="%s" required style="width: 100%%; padding: 0.5rem;">
          <button type="submit">Save</button>
        </form>
        `, source, target)
	}
}

func UpdateForwardingRule(db *storage.DB) http.HandlerFunc {
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

func DeleteForwardingRule(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}

		source := r.FormValue("source")
		if source != "" {
			db.DeleteForwardingRule(source)
		}

		http.Redirect(w, r, "/forwarding", http.StatusSeeOther)
	}
}
