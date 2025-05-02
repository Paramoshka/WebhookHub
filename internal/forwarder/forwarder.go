package forwarder

import (
	"bytes"
	"log"
	"net/http"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

func Forward(db *storage.DB, h model.Webhook) {
	target := db.GetTargetForSource(h.Source)
	if target == "" {
		log.Printf("⚠️ No forwarding target for source '%s'\n", h.Source)
		db.UpdateStatus(int(h.ID), "skipped")
		return
	}

	resp, err := http.Post(target, "application/json", bytes.NewBuffer(h.Payload))
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

	log.Printf("✅ Forwarded webhook ID %d to %s\n", h.ID, target)
	db.UpdateStatus(int(h.ID), "success")
}
