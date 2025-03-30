package handler

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

type WebhookPageData struct {
	Webhooks    []model.Webhook
	CurrentPage int
	PrevPage    int
	NextPage    int
	DisablePrev bool
	DisableNext bool
}

func WebhookPartial(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := strings.TrimSpace(r.URL.Query().Get("source"))
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		pageStr := r.URL.Query().Get("page")
		page := 1
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}

		pageSize := 10
		offset := (page - 1) * pageSize

		hooks := db.Filtered(source, status, pageSize, offset)
		total := db.CountFiltered(source, status)

		tmpl := template.Must(template.ParseFiles("web/templates/partials.html"))

		data := WebhookPageData{
			Webhooks:    hooks,
			CurrentPage: page,
			PrevPage:    page - 1,
			NextPage:    page + 1,
			DisablePrev: page <= 1,
			DisableNext: page*pageSize >= total,
		}

		//log.Printf("WebhookPartial filters: source=%s, status=%s, page=%d", source, status, page)

		tmpl.Execute(w, data)
	}
}

func InspectWebhook(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/partials/webhook/")
		webhook, found := db.FindByID(id)
		if !found {
			http.NotFound(w, r)
			return
		}

		tmpl := template.Must(template.ParseFiles("web/templates/inspect.html"))
		tmpl.Execute(w, webhook)
	}
}
