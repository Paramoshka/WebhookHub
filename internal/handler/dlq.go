package handler

import (
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
	CSRFToken   string
}

func DLQUI(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := strings.TrimSpace(r.URL.Query().Get("source"))
		query := storage.WebhookFilter{
			Source: source,
			Status: "dead_lettered",
			Query:  strings.TrimSpace(r.URL.Query().Get("q")),
			From:   parseWebhookDateTime(r.URL.Query().Get("from"), false),
			To:     parseWebhookDateTime(r.URL.Query().Get("to"), true),
		}

		page := parsePage(r.URL.Query().Get("page"))
		pageSize := 20
		offset := (page - 1) * pageSize

		webhooks, err := db.Filtered(query, pageSize, offset)
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		total, err := db.CountFiltered(query)
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}

		filters := r.URL.Query()
		data := DLQPageData{
			Webhooks:    webhooks,
			Source:      source,
			CurrentPage: page,
			CurrentURL:  buildDLQPageURLWithFilters(filters, page),
			PrevURL:     buildDLQPageURLWithFilters(filters, page-1),
			NextURL:     buildDLQPageURLWithFilters(filters, page+1),
			DisablePrev: page <= 1,
			DisableNext: page*pageSize >= total,
			CSRFToken:   CSRFToken(r),
		}

		if err := dlqTemplates.ExecuteTemplate(w, "base", data); err != nil {
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

func buildDLQPageURLWithFilters(values url.Values, page int) string {
	filtered := url.Values{}
	for key, vals := range values {
		for _, val := range vals {
			filtered.Add(key, val)
		}
	}
	filtered.Set("page", strconv.Itoa(page))

	return "/dlq?" + filtered.Encode()
}
