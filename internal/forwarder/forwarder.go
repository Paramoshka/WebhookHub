package forwarder

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"time"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

var timeout = 5 * time.Second

const maxBody = int64(1 << 20)

func Forward(db *storage.DB, h *model.Webhook) {
	target := db.GetTargetForSource(h.Source)
	if target == "" {
		log.Printf("⚠️ No forwarding target for source '%s'\n", h.Source)
		db.UpdateStatus(int(h.ID), "skipped")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", target, bytes.NewBuffer(h.Payload))
	if err != nil {
		log.Println(err)
		cancel()
		return
	}

	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Timeout: timeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Forwarding to %s failed: %v\n", target, err)
		db.UpdateStatus(int(h.ID), "failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("❌ Forwarding failed with status %d: %s\n", resp.StatusCode, target)
		db.UpdateStatus(int(h.ID), "failed")
		return
	}

	limited := io.LimitReader(resp.Body, maxBody)
	body, err := io.ReadAll(limited)
	if err != nil {
		db.UpdateResponseFromForward(int(h.ID), body)
		log.Printf("❌ Forwarding error reading body: %v\n", err)
		return
	}

	db.UpdateResponseFromForward(int(h.ID), body)

	log.Printf("✅ Forwarded webhook ID %d to %s\n", h.ID, target)
	db.UpdateStatus(int(h.ID), "success")
}
