package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"webhookhub/internal/storage"
)

const MaxBulkReplayBodyBytes = 64 << 10

type bulkReplayStore interface {
	BulkReplayDeadLetters(context.Context, []int) (storage.BulkReplayResult, error)
}

func BulkReplayWebhooks(db bulkReplayStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		ids, err := parseBulkReplayIDs(r.PostForm["ids"])
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := db.BulkReplayDeadLetters(r.Context(), ids)
		if err != nil {
			http.Error(w, "Failed to requeue webhooks. Reload the list before retrying.", http.StatusServiceUnavailable)
			return
		}
		target, err := url.Parse(r.PostForm.Get("redirect_to"))
		if err != nil || !isLocalRedirect(r.PostForm.Get("redirect_to")) || target.Path != "/dlq" || target.Fragment != "" {
			target = &url.URL{Path: "/dlq"}
		}
		query := target.Query()
		query.Set("requeued", strconv.Itoa(result.Requeued))
		query.Set("skipped", strconv.Itoa(result.Skipped))
		target.RawQuery = query.Encode()
		http.Redirect(w, r, target.String(), http.StatusSeeOther)
	}
}

func parseBulkReplayIDs(values []string) ([]int, error) {
	ids := make([]int, 0, storage.MaxBulkReplay)
	seen := make(map[int]bool)
	for _, raw := range values {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			return nil, errors.New("Invalid webhook ID")
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
		if len(ids) > storage.MaxBulkReplay {
			return nil, errors.New("Select at most 20 webhooks")
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("Select at least one webhook")
	}
	return ids, nil
}
