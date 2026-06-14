package handler

import (
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

type DLQPageData struct {
	Webhooks    []model.Webhook
	Source      string
	CurrentPage int
	CurrentURL  string
	PrevURL     string
	NextURL     string
	DisablePrev bool
	DisableNext bool
}

func DLQUI(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := strings.TrimSpace(r.URL.Query().Get("source"))
		page := parsePage(r.URL.Query().Get("page"))
		pageSize := 20
		offset := (page - 1) * pageSize

		webhooks := db.Filtered(source, "dead_lettered", pageSize, offset)
		total := db.CountFiltered(source, "dead_lettered")

		data := DLQPageData{
			Webhooks:    webhooks,
			Source:      source,
			CurrentPage: page,
			CurrentURL:  buildDLQPageURL(source, page),
			PrevURL:     buildDLQPageURL(source, page-1),
			NextURL:     buildDLQPageURL(source, page+1),
			DisablePrev: page <= 1,
			DisableNext: page*pageSize >= total,
		}

		tmpl := template.Must(template.ParseFiles(
			"web/templates/base.html",
			"web/templates/dlq.html",
		))
		if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
			http.Error(w, "Template render failed", http.StatusInternalServerError)
		}
	}
}

func parsePage(raw string) int {
	page := 1
	if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
		page = parsed
	}
	return page
}

func buildDLQPageURL(source string, page int) string {
	if page < 1 {
		page = 1
	}

	values := url.Values{}
	values.Set("page", strconv.Itoa(page))
	if source != "" {
		values.Set("source", source)
	}

	return "/dlq?" + values.Encode()
}
