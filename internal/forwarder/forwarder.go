package forwarder

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"time"
	"webhookhub/internal/hmacsig"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

var timeout = 5 * time.Second

const maxBody = int64(1 << 20)

func Forward(db *storage.DB, h *model.Webhook) {
	rule, found := db.GetForwardingRule(h.Source)
	if !found || rule.Target == "" {
		log.Printf("⚠️ No forwarding target for source '%s'\n", h.Source)
		db.UpdateStatus(int(h.ID), "skipped")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rule.Target, bytes.NewBuffer(h.Payload))
	if err != nil {
		log.Printf("❌ Failed to create forwarding request for %s: %v\n", rule.Target, err)
		db.UpdateStatus(int(h.ID), "failed")
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if rule.OutgoingSecret != "" {
		req.Header.Set(hmacsig.OutgoingHeader, hmacsig.SignHeader(rule.OutgoingSecret, h.Payload, time.Now()))
	}

	client := &http.Client{
		Timeout: timeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Forwarding to %s failed: %v\n", rule.Target, err)
		db.UpdateStatus(int(h.ID), "failed")
		return
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxBody)
	body, err := io.ReadAll(limited)
	if err != nil {
		db.UpdateResponseFromForward(int(h.ID), body)
		log.Printf("❌ Forwarding error reading body: %v\n", err)
		db.UpdateStatus(int(h.ID), "failed")
		return
	}

	db.UpdateResponseFromForward(int(h.ID), body)

	if resp.StatusCode >= 400 {
		log.Printf("❌ Forwarding failed with status %d: %s\n", resp.StatusCode, rule.Target)
		db.UpdateStatus(int(h.ID), "failed")
		return
	}

	log.Printf("✅ Forwarded webhook ID %d to %s\n", h.ID, rule.Target)
	db.UpdateStatus(int(h.ID), "success")
}
