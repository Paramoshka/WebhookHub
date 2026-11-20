package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

type webhookExportStore interface {
	StreamWebhooks(context.Context, storage.WebhookFilter, func(model.Webhook) error) error
}

// ExportWebhooks streams every event matching the logs filters as NDJSON, one
// JSON object per line, in the same shape as the paginated JSON API. Headers
// are deferred until the first row so a storage failure before streaming can
// still return 503; once the attachment is streaming, the status cannot change.
func ExportWebhooks(db webhookExportStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, _, err := parseWebhookFilterAndPage(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		started := false
		enc := json.NewEncoder(w)
		err = db.StreamWebhooks(r.Context(), filter, func(hook model.Webhook) error {
			if !started {
				started = true
				setExportHeaders(w)
			}
			return enc.Encode(hook)
		})
		if err != nil {
			if !started {
				http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
				return
			}
			log.Printf("export webhooks: %v", err)
			return
		}
		if !started {
			setExportHeaders(w)
		}
	}
}

func setExportHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="webhooks-%s.ndjson"`, time.Now().UTC().Format("20060102-150405")))
}
