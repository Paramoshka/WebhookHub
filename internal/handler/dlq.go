package handler

import (
	"fmt"
	"math"
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
	BulkMessage string
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

		webhooks, err := db.FilteredSummary(r.Context(), query, pageSize, offset)
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		total, err := db.CountFiltered(r.Context(), query)
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
			BulkMessage: bulkReplayMessage(r.URL.Query()),
		}

		if err := dlqTemplates.ExecuteTemplate(w, "base", data); err != nil {
			http.Error(w, "Template render failed", http.StatusInternalServerError)
		}
	}
}

func parsePage(raw string) int {
	page := 1
	if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= math.MaxInt/20 {
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
	filtered.Del("requeued")
	filtered.Del("skipped")
	filtered.Set("page", strconv.Itoa(page))

	return "/dlq?" + filtered.Encode()
}

func bulkReplayMessage(query url.Values) string {
	requeued, err := strconv.Atoi(query.Get("requeued"))
	skipped, skippedErr := strconv.Atoi(query.Get("skipped"))
	if err != nil || skippedErr != nil || requeued < 0 || skipped < 0 || requeued > storage.MaxBulkReplay || skipped > storage.MaxBulkReplay-requeued {
		return ""
	}
	return fmt.Sprintf("Queued for delivery: %d. Skipped: %d (deleted or no longer in DLQ).", requeued, skipped)
}
