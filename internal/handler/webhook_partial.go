package handler

import (
	"context"
	"fmt"
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
	FromInstant    string
	ToInstant      string
	Sort           string
	CSRFToken      string
}

type webhookListStore interface {
	FilteredSummary(context.Context, storage.WebhookFilter, int, int) ([]model.Webhook, error)
	CountFiltered(context.Context, storage.WebhookFilter) (int, error)
}

func WebhookPartial(db webhookListStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, page, err := parseWebhookFilterAndPage(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		offset := (page - 1) * webhookPageSize

		hooks, err := db.FilteredSummary(r.Context(), filter, webhookPageSize, offset)
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		total, err := db.CountFiltered(r.Context(), filter)
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}

		query := canonicalDateQuery(r.URL.Query(), filter.From, filter.To)

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
			FromInstant:    dateTimeInstant(filter.From),
			ToInstant:      dateTimeInstant(filter.To),
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

func parseWebhookFilterAndPage(r *http.Request) (storage.WebhookFilter, int, error) {
	page := 1
	pageStr := r.URL.Query().Get("page")
	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 && p <= math.MaxInt/webhookPageSize {
		page = p
	}

	from, to, err := webhookFilterDates(r.URL.Query())
	if err != nil {
		return storage.WebhookFilter{}, page, err
	}
	filter := storage.WebhookFilter{
		Source: strings.TrimSpace(r.URL.Query().Get("source")),
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Query:  strings.TrimSpace(r.URL.Query().Get("q")),
		Sort:   strings.TrimSpace(r.URL.Query().Get("sort")),
		From:   from,
		To:     to,
	}

	return filter, page, nil
}

func parseWebhookDateTime(raw string, isTo bool, loc *time.Location) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return &t, nil
	}

	layouts := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			if layout == "2006-01-02" && isTo {
				t = t.AddDate(0, 0, 1)
			}
			return resolveLocalTime(t, loc)
		}
	}

	return nil, fmt.Errorf("invalid date %q; enter a date and time or an RFC3339 value", raw)
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
