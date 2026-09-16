package forwarder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
	"webhookhub/internal/hmacsig"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

const timeout = 5 * time.Second

var deliveryClient = &http.Client{Timeout: timeout}

const maxBody = int64(1 << 20)
const DefaultMaxAttempts = 3
const DefaultBackoffSeconds = 2
const maxBackoffDelay = 30 * time.Second

type WorkerConfig struct {
	Count         int
	PollInterval  time.Duration
	LeaseDuration time.Duration
}

func Forward(ctx context.Context, db *storage.DB, h *model.Webhook) error {
	rule, err := db.GetForwardingRule(h.Source)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return recordSkippedAttempt(db, h, "")
		}
		return fmt.Errorf("load forwarding rule: %w", err)
	}
	if rule.Target == "" {
		return recordSkippedAttempt(db, h, "")
	}

	maxAttempts := normalizeMaxAttempts(rule.RetryMaxAttempts)
	backoffSeconds := normalizeBackoffSeconds(rule.RetryBackoffSeconds)
	attemptNumber := h.FailureCount + 1

	startedAt := time.Now()
	attemptID, err := db.CreateDeliveryAttempt(&model.DeliveryAttempt{
		WebhookID: h.ID,
		Source:    h.Source,
		Target:    rule.Target,
		Status:    "pending",
		StartedAt: startedAt,
	})
	if err != nil {
		return fmt.Errorf("create delivery attempt: %w", err)
	}

	response, errorMessage, success := performDeliveryAttempt(ctx, rule, h)

	attemptStatus := "failed"
	if success {
		attemptStatus = "success"
	}
	if ctx.Err() != nil {
		attemptStatus = "cancelled"
	}

	if err := db.FinishDeliveryAttempt(attemptID, attemptStatus, response, errorMessage, time.Since(startedAt).Milliseconds()); err != nil {
		return fmt.Errorf("finish delivery attempt: %w", err)
	}

	if ctx.Err() != nil {
		if err := db.ReleaseWebhookLease(int(h.ID)); err != nil {
			return fmt.Errorf("release cancelled delivery: %w", err)
		}
		return ctx.Err()
	}

	if success {
		if err := db.MarkWebhookDeliverySuccess(int(h.ID)); err != nil {
			return fmt.Errorf("mark delivery successful: %w", err)
		}
		log.Printf("✅ Forwarded webhook ID %d to %s on attempt %d/%d\n", h.ID, rule.Target, attemptNumber, maxAttempts)
		return nil
	}

	if attemptNumber >= maxAttempts {
		finalStatus, err := db.MarkWebhookDeliveryFailed(int(h.ID), errorMessage, maxAttempts)
		if err != nil {
			return fmt.Errorf("mark delivery failed: %w", err)
		}
		log.Printf("❌ Webhook ID %d moved to %s after %d failed attempts\n", h.ID, finalStatus, attemptNumber)
		return nil
	}

	delay := retryDelay(backoffSeconds, attemptNumber)
	nextRetryAt := time.Now().Add(delay)
	if err := db.MarkWebhookRetryScheduled(int(h.ID), attemptNumber, errorMessage, nextRetryAt); err != nil {
		return fmt.Errorf("schedule delivery retry: %w", err)
	}
	log.Printf("⚠️ Delivery attempt %d/%d for webhook ID %d failed, scheduled retry at %s\n", attemptNumber, maxAttempts, h.ID, nextRetryAt.Format(time.RFC3339))
	return nil
}

func StartWorkerPool(ctx context.Context, db *storage.DB, config WorkerConfig) *sync.WaitGroup {
	if config.Count <= 0 {
		config.Count = 4
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.LeaseDuration <= 0 {
		config.LeaseDuration = 30 * time.Second
	}

	wg := &sync.WaitGroup{}
	for workerID := 1; workerID <= config.Count; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			runWorker(ctx, db, config, id)
		}(workerID)
	}

	return wg
}

func runWorker(ctx context.Context, db *storage.DB, config WorkerConfig, workerID int) {
	for {
		if ctx.Err() != nil {
			return
		}

		now := time.Now()
		hooks, err := db.ClaimDeliverableWebhooks(1, now, now.Add(config.LeaseDuration))
		if err != nil {
			log.Printf("delivery worker %d failed to claim webhook: %v", workerID, err)
		} else if len(hooks) > 0 {
			if err := Forward(ctx, db, &hooks[0]); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("delivery worker %d failed webhook %d: %v", workerID, hooks[0].ID, err)
			}
			continue
		}

		timer := time.NewTimer(config.PollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func performDeliveryAttempt(parent context.Context, rule model.ForwardingRule, h *model.Webhook) (model.DeliveryResponse, string, bool) {
	return performDeliveryAttemptWithClient(parent, deliveryClient, rule, h)
}

func performDeliveryAttemptWithClient(parent context.Context, client *http.Client, rule model.ForwardingRule, h *model.Webhook) (model.DeliveryResponse, string, bool) {
	var response model.DeliveryResponse
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rule.Target, bytes.NewBuffer(h.Payload))
	if err != nil {
		errMsg := fmt.Sprintf("failed to create forwarding request: %v", err)
		log.Printf("❌ Failed to create forwarding request for %s: %v\n", rule.Target, err)
		return response, errMsg, false
	}

	if h.Headers == "" {
		// Webhooks saved before header capture keep the original forwarding default.
		req.Header.Set("Content-Type", "application/json")
	} else {
		var headers http.Header
		if err := json.Unmarshal([]byte(h.Headers), &headers); err != nil {
			return response, fmt.Sprintf("failed to decode stored request headers: %v", err), false
		}
		if values, ok := headers["Content-Type"]; ok {
			req.Header["Content-Type"] = values
		}
	}
	if rule.OutgoingSecret != "" {
		req.Header.Set(hmacsig.OutgoingHeader, hmacsig.SignHeader(rule.OutgoingSecret, h.Payload, time.Now()))
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("❌ Forwarding to %s failed: %v\n", rule.Target, err)
		return response, err.Error(), false
	}
	defer resp.Body.Close()

	response.HTTPStatus = resp.StatusCode
	response.Captured = true
	headers, err := json.Marshal(resp.Header)
	if err != nil {
		return response, fmt.Sprintf("failed to serialize response headers: %v", err), false
	}
	response.Headers = string(headers)
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	response.Truncated = int64(len(body)) > maxBody
	response.Body = body[:min(int64(len(body)), maxBody)]
	if err != nil {
		errMsg := fmt.Sprintf("failed to read response body: %v", err)
		log.Printf("❌ Forwarding error reading body: %v\n", err)
		return response, errMsg, false
	}

	if resp.StatusCode >= 400 {
		errMsg := fmt.Sprintf("target responded with status %d", resp.StatusCode)
		log.Printf("❌ Forwarding failed with status %d: %s\n", resp.StatusCode, rule.Target)
		return response, errMsg, false
	}

	return response, "", true
}

func recordSkippedAttempt(db *storage.DB, h *model.Webhook, target string) error {
	startedAt := time.Now()
	attemptID, err := db.CreateDeliveryAttempt(&model.DeliveryAttempt{
		WebhookID: h.ID,
		Source:    h.Source,
		Target:    target,
		Status:    "pending",
		StartedAt: startedAt,
	})
	if err != nil {
		return fmt.Errorf("create skipped delivery attempt: %w", err)
	}

	if err := db.FinishDeliveryAttempt(attemptID, "skipped", model.DeliveryResponse{}, "no forwarding target configured", time.Since(startedAt).Milliseconds()); err != nil {
		return fmt.Errorf("finish skipped delivery attempt: %w", err)
	}
	if err := db.MarkWebhookDeliverySkipped(int(h.ID)); err != nil {
		return fmt.Errorf("mark delivery skipped: %w", err)
	}
	log.Printf("⚠️ No forwarding target for source '%s'\n", h.Source)
	return nil
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
