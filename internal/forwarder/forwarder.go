package forwarder

import (
	"bytes"
	"context"
	"fmt"
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
	startedAt := time.Now()
	rule, found := db.GetForwardingRule(h.Source)

	attemptID := db.CreateDeliveryAttempt(&model.DeliveryAttempt{
		WebhookID: h.ID,
		Source:    h.Source,
		Target:    rule.Target,
		Status:    "pending",
		StartedAt: startedAt,
	})

	status := "failed"
	httpStatus := 0
	errorMessage := ""
	responseBody := []byte(nil)

	defer func() {
		db.UpdateResponseFromForward(int(h.ID), responseBody)
		db.UpdateStatus(int(h.ID), status)
		db.FinishDeliveryAttempt(attemptID, status, httpStatus, errorMessage, time.Since(startedAt).Milliseconds())
	}()

	if !found || rule.Target == "" {
		errorMessage = "no forwarding target configured"
		status = "skipped"
		log.Printf("⚠️ No forwarding target for source '%s'\n", h.Source)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rule.Target, bytes.NewBuffer(h.Payload))
	if err != nil {
		errorMessage = fmt.Sprintf("failed to create forwarding request: %v", err)
		log.Printf("❌ Failed to create forwarding request for %s: %v\n", rule.Target, err)
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
		errorMessage = err.Error()
		log.Printf("❌ Forwarding to %s failed: %v\n", rule.Target, err)
		return
	}
	defer resp.Body.Close()

	httpStatus = resp.StatusCode

	limited := io.LimitReader(resp.Body, maxBody)
	body, err := io.ReadAll(limited)
	if err != nil {
		responseBody = body
		errorMessage = fmt.Sprintf("failed to read response body: %v", err)
		log.Printf("❌ Forwarding error reading body: %v\n", err)
		return
	}

	responseBody = body

	if resp.StatusCode >= 400 {
		errorMessage = fmt.Sprintf("target responded with status %d", resp.StatusCode)
		log.Printf("❌ Forwarding failed with status %d: %s\n", resp.StatusCode, rule.Target)
		return
	}

	status = "success"
	log.Printf("✅ Forwarded webhook ID %d to %s\n", h.ID, rule.Target)
}
