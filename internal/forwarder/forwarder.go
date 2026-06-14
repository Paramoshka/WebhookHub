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
const DefaultMaxAttempts = 3
const DefaultBackoffSeconds = 2
const maxBackoffDelay = 30 * time.Second

func Forward(db *storage.DB, h *model.Webhook) {
	rule, found := db.GetForwardingRule(h.Source)
	if !found || rule.Target == "" {
		recordSkippedAttempt(db, h, rule.Target)
		return
	}

	maxAttempts := normalizeMaxAttempts(rule.RetryMaxAttempts)
	backoffSeconds := normalizeBackoffSeconds(rule.RetryBackoffSeconds)
	attemptNumber := h.FailureCount + 1

	startedAt := time.Now()
	attemptID := db.CreateDeliveryAttempt(&model.DeliveryAttempt{
		WebhookID: h.ID,
		Source:    h.Source,
		Target:    rule.Target,
		Status:    "pending",
		StartedAt: startedAt,
	})

	httpStatus, responseBody, errorMessage, success := performDeliveryAttempt(rule, h)
	db.UpdateResponseFromForward(int(h.ID), responseBody)

	attemptStatus := "failed"
	if success {
		attemptStatus = "success"
	}

	db.FinishDeliveryAttempt(attemptID, attemptStatus, httpStatus, errorMessage, time.Since(startedAt).Milliseconds())

	if success {
		db.MarkWebhookDeliverySuccess(int(h.ID))
		log.Printf("✅ Forwarded webhook ID %d to %s on attempt %d/%d\n", h.ID, rule.Target, attemptNumber, maxAttempts)
		return
	}

	if attemptNumber >= maxAttempts {
		finalStatus := db.MarkWebhookDeliveryFailed(int(h.ID), errorMessage, maxAttempts)
		log.Printf("❌ Webhook ID %d moved to %s after %d failed attempts\n", h.ID, finalStatus, attemptNumber)
		return
	}

	delay := retryDelay(backoffSeconds, attemptNumber)
	nextRetryAt := time.Now().Add(delay)
	db.MarkWebhookRetryScheduled(int(h.ID), attemptNumber, errorMessage, nextRetryAt)
	log.Printf("⚠️ Delivery attempt %d/%d for webhook ID %d failed, scheduled retry at %s\n", attemptNumber, maxAttempts, h.ID, nextRetryAt.Format(time.RFC3339))
}

func StartRetryWorker(db *storage.DB, interval time.Duration, batchSize int) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if batchSize <= 0 {
		batchSize = 20
	}

	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			hooks := db.ClaimDueRetryWebhooks(batchSize, time.Now())
			for i := range hooks {
				hook := hooks[i]
				go Forward(db, &hook)
			}
		}
	}()
}

func performDeliveryAttempt(rule model.ForwardingRule, h *model.Webhook) (int, []byte, string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rule.Target, bytes.NewBuffer(h.Payload))
	if err != nil {
		errMsg := fmt.Sprintf("failed to create forwarding request: %v", err)
		log.Printf("❌ Failed to create forwarding request for %s: %v\n", rule.Target, err)
		return 0, nil, errMsg, false
	}

	req.Header.Set("Content-Type", "application/json")
	if rule.OutgoingSecret != "" {
		req.Header.Set(hmacsig.OutgoingHeader, hmacsig.SignHeader(rule.OutgoingSecret, h.Payload, time.Now()))
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Forwarding to %s failed: %v\n", rule.Target, err)
		return 0, nil, err.Error(), false
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxBody)
	body, err := io.ReadAll(limited)
	if err != nil {
		errMsg := fmt.Sprintf("failed to read response body: %v", err)
		log.Printf("❌ Forwarding error reading body: %v\n", err)
		return resp.StatusCode, body, errMsg, false
	}

	if resp.StatusCode >= 400 {
		errMsg := fmt.Sprintf("target responded with status %d", resp.StatusCode)
		log.Printf("❌ Forwarding failed with status %d: %s\n", resp.StatusCode, rule.Target)
		return resp.StatusCode, body, errMsg, false
	}

	return resp.StatusCode, body, "", true
}

func recordSkippedAttempt(db *storage.DB, h *model.Webhook, target string) {
	startedAt := time.Now()
	attemptID := db.CreateDeliveryAttempt(&model.DeliveryAttempt{
		WebhookID: h.ID,
		Source:    h.Source,
		Target:    target,
		Status:    "pending",
		StartedAt: startedAt,
	})

	db.FinishDeliveryAttempt(attemptID, "skipped", 0, "no forwarding target configured", time.Since(startedAt).Milliseconds())
	db.MarkWebhookDeliverySkipped(int(h.ID))
	log.Printf("⚠️ No forwarding target for source '%s'\n", h.Source)
}

func normalizeMaxAttempts(value int) int {
	if value <= 0 {
		return DefaultMaxAttempts
	}
	return value
}

func normalizeBackoffSeconds(value int) int {
	if value <= 0 {
		return DefaultBackoffSeconds
	}
	return value
}

func retryDelay(baseSeconds, attemptNumber int) time.Duration {
	if attemptNumber < 1 {
		attemptNumber = 1
	}

	delay := time.Duration(baseSeconds) * time.Second
	for step := 1; step < attemptNumber; step++ {
		delay *= 2
		if delay >= maxBackoffDelay {
			return maxBackoffDelay
		}
	}

	if delay > maxBackoffDelay {
		return maxBackoffDelay
	}
	return delay
}
