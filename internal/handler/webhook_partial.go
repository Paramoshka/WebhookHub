package handler

import (
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

type WebhookPageData struct {
	Webhooks    []model.Webhook
	CurrentPage int
	CurrentURL  string
	PrevURL     string
	NextURL     string
	DisablePrev bool
	DisableNext bool
	Source      string
	Status      string
	Query       string
	From        string
	To          string
	Sort        string
	CSRFToken   string
}

type InspectWebhookData struct {
	Webhook  model.Webhook
	Attempts []model.DeliveryAttempt
}

func WebhookPartial(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, page := parseWebhookFilterAndPage(r)
		pageSize := 10
		offset := (page - 1) * pageSize

		hooks, err := db.Filtered(filter, pageSize, offset)
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		total, err := db.CountFiltered(filter)
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}

		prevURL := ""
		nextURL := ""
		query := cloneURLValues(r.URL.Query())
		query.Set("page", strconv.Itoa(page))
		currentURL := buildWebhookListURL(query, page)
		prevURL = buildWebhookListURL(query, page-1)
		nextURL = buildWebhookListURL(query, page+1)

		tmpl, err := template.ParseFiles("web/templates/logs.html", "web/templates/partials.html")
		if err != nil {
			http.Error(w, "Template load failed", http.StatusInternalServerError)
			return
		}

		data := WebhookPageData{
			Webhooks:    hooks,
			CurrentPage: page,
			CurrentURL:  currentURL,
			PrevURL:     prevURL,
			NextURL:     nextURL,
			DisablePrev: page <= 1,
			DisableNext: page*pageSize >= total,
			Source:      filter.Source,
			Status:      filter.Status,
			Query:       filter.Query,
			From:        formatDateTimeInput(filter.From),
			To:          formatDateTimeInput(filter.To),
			Sort:        filter.Sort,
			CSRFToken:   CSRFToken(r),
		}

		if err := tmpl.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func parseWebhookFilterAndPage(r *http.Request) (storage.WebhookFilter, int) {
	page := 1
	pageStr := r.URL.Query().Get("page")
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}

	filter := storage.WebhookFilter{
		Source: strings.TrimSpace(r.URL.Query().Get("source")),
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Query:  strings.TrimSpace(r.URL.Query().Get("q")),
		Sort:   strings.TrimSpace(r.URL.Query().Get("sort")),
		From:   parseWebhookDateTime(r.URL.Query().Get("from"), false),
		To:     parseWebhookDateTime(r.URL.Query().Get("to"), true),
	}

	return filter, page
}

func parseWebhookDateTime(raw string, isTo bool) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04",
		"2006-01-02",
	}

	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			if layout == "2006-01-02" && isTo {
				t = t.AddDate(0, 0, 1)
			}
			return &t
		}
	}

	return nil
}

func formatDateTimeInput(v *time.Time) string {
	if v == nil {
		return ""
	}

	return v.Format("2006-01-02T15:04")
}

func buildWebhookListURL(values url.Values, page int) string {
	if page < 1 {
		page = 1
	}

	filtered := cloneURLValues(values)
	if page > 0 {
		filtered.Set("page", strconv.Itoa(page))
	}

	return "/partials/webhooks?" + filtered.Encode()
}

func cloneURLValues(values url.Values) url.Values {
	out := url.Values{}
	for key, vals := range values {
		for _, val := range vals {
			out.Add(key, val)
		}
	}
	return out
}

func InspectWebhook(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/partials/webhook/")
		webhook, err := db.FindByID(id)
		if errors.Is(err, storage.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		attempts, err := db.DeliveryAttemptsByWebhook(webhook.ID)
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}

		tmpl, err := template.ParseFiles("web/templates/inspect.html")
		if err != nil {
			http.Error(w, "Template load failed", http.StatusInternalServerError)
			return
		}
		if err := tmpl.Execute(w, InspectWebhookData{
			Webhook:  webhook,
			Attempts: attempts,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
