package handler

import (
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

const webhookPageSize = 10

type WebhookPageData struct {
	Webhooks       []model.Webhook
	CurrentPage    int
	CurrentURL     string
	CurrentPageURL string
	PrevPageURL    string
	NextPageURL    string
	PrevURL        string
	NextURL        string
	DisablePrev    bool
	DisableNext    bool
	Source         string
	Status         string
	Query          string
	From           string
	To             string
	Sort           string
	CSRFToken      string
}

type webhookListStore interface {
	Filtered(storage.WebhookFilter, int, int) ([]model.Webhook, error)
	CountFiltered(storage.WebhookFilter) (int, error)
}

func WebhookPartial(db webhookListStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, page := parseWebhookFilterAndPage(r)
		offset := (page - 1) * webhookPageSize

		hooks, err := db.Filtered(filter, webhookPageSize, offset)
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		total, err := db.CountFiltered(filter)
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}

		query := r.URL.Query()

		data := WebhookPageData{
			Webhooks:       hooks,
			CurrentPage:    page,
			CurrentURL:     buildWebhookListURL(query, page),
			PrevURL:        buildWebhookListURL(query, page-1),
			NextURL:        buildWebhookListURL(query, page+1),
			CurrentPageURL: buildDashboardURL(query, page),
			PrevPageURL:    buildDashboardURL(query, page-1),
			NextPageURL:    buildDashboardURL(query, page+1),
			DisablePrev:    page <= 1,
			DisableNext:    page*webhookPageSize >= total,
			Source:         filter.Source,
			Status:         filter.Status,
			Query:          filter.Query,
			From:           formatDateTimeInput(filter.From),
			To:             formatDateTimeInput(filter.To),
			Sort:           filter.Sort,
			CSRFToken:      CSRFToken(r),
		}

		if r.Header.Get("HX-Request") == "true" && r.Header.Get("X-Update-History") == "true" {
			w.Header().Set("HX-Push-Url", data.CurrentPageURL)
		}
		if err := logsTemplates.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func parseWebhookFilterAndPage(r *http.Request) (storage.WebhookFilter, int) {
	page := 1
	pageStr := r.URL.Query().Get("page")
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 && p <= math.MaxInt/webhookPageSize {
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
		if t, err := time.ParseInLocation(layout, raw, time.UTC); err == nil {
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

	return v.UTC().Format("2006-01-02T15:04")
}

func buildWebhookListURL(values url.Values, page int) string {
	return "/partials/webhooks?" + webhookListQuery(values, page)
}

func buildDashboardURL(values url.Values, page int) string {
	return "/dashboard?" + webhookListQuery(values, page)
}

func webhookListQuery(values url.Values, page int) string {
	if page < 1 {
		page = 1
	}

	filtered := url.Values{}
	for _, name := range []string{"source", "status", "q", "from", "to", "sort"} {
		if value := strings.TrimSpace(values.Get(name)); value != "" {
			filtered.Set(name, value)
		}
	}
	filtered.Set("page", strconv.Itoa(page))
	return filtered.Encode()
}
