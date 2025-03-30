package forwarder

import (
    "bytes"
    "log"
    "net/http"
    "webhookhub/internal/model"
    "webhookhub/internal/storage"
    "webhookhub/internal/config"
)

func Forward(db *storage.DB, h model.Webhook) {
    target := config.GetForwardTarget(h.Source)
    if target == "" {
        log.Println("No forward target for:", h.Source)
        db.UpdateStatus(h.ID, "skipped")
        return
    }

    resp, err := http.Post(target, "application/json", bytes.NewBuffer(h.Payload))
    if err != nil || resp.StatusCode >= 400 {
        log.Println("Forward error:", err)
        db.UpdateStatus(h.ID, "failed")
        return
    }

    db.UpdateStatus(h.ID, "success")
}