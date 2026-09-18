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

const DefaultDeliveryTimeout = 5 * time.Second
const DeliveryFinalizationReserve = 5 * time.Second

// deliveryClient has no client-level timeout: every attempt gets its own
// deadline from the configured delivery timeout.
var deliveryClient = &http.Client{}

const maxBody = int64(1 << 20)
const DefaultMaxAttempts = 3
const DefaultBackoffSeconds = 2
const maxBackoffDelay = 30 * time.Second

type WorkerConfig struct {
	Count         int
	PollInterval  time.Duration
	LeaseDuration time.Duration
	Timeout       time.Duration
}

func Forward(ctx context.Context, db *storage.DB, h *model.Webhook, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultDeliveryTimeout
	}
	if h.DeliveryLeaseUntil == nil {
		return storage.ErrLeaseLost
	}
	preparationCtx, cancelPreparation := context.WithDeadline(ctx, h.DeliveryLeaseUntil.Add(-DeliveryFinalizationReserve))
	defer cancelPreparation()
	if err := preparationCtx.Err(); err != nil {
		return err
	}
	rule, err := db.GetForwardingRule(preparationCtx, h.Source)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("load forwarding rule: %w", err)
	}

	startedAt := time.Now()
	attemptID, err := db.CreateDeliveryAttempt(preparationCtx, h, rule.Target)
	if err != nil {
		return fmt.Errorf("create delivery attempt: %w", err)
	}
	result := storage.DeliveryResult{Status: "skipped", ErrorMessage: "no forwarding target configured"}
	if rule.Target != "" {
		response, errorMessage, success := performDeliveryAttempt(preparationCtx, rule, h, timeout)
		result = storage.DeliveryResult{Status: "failed", Response: response, ErrorMessage: errorMessage}
		if success {
			result.Status = "success"
		} else if h.FailureCount+1 < normalizeMaxAttempts(rule.RetryMaxAttempts) {
			nextRetryAt := time.Now().Add(retryDelay(normalizeBackoffSeconds(rule.RetryBackoffSeconds), h.FailureCount+1))
			result.NextRetryAt = &nextRetryAt
		}
	}
	if ctx.Err() != nil {
		result.Status = "cancelled"
		result.ErrorMessage = ctx.Err().Error()
		result.NextRetryAt = nil
	}
	result.DurationMS = time.Since(startedAt).Milliseconds()
	// Worker cancellation must not prevent recording cancellation. This detached
	// operation is still bounded by both the reserve and the claim's lease.
	finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ctx), DeliveryFinalizationReserve)
	defer cancelFinish()
	if err := db.FinishDelivery(finishCtx, h, attemptID, result); err != nil {
		return fmt.Errorf("finish delivery: %w", err)
	}
	log.Printf("delivery webhook %d attempt %d: %s", h.ID, attemptID, result.Status)
	return ctx.Err()
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
	if config.Timeout <= 0 {
		config.Timeout = DefaultDeliveryTimeout
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
		hooks, err := db.ClaimDeliverableWebhooks(ctx, 1, now, now.Add(config.LeaseDuration))
		if err != nil {
			log.Printf("delivery worker %d failed to claim webhook: %v", workerID, err)
		} else if len(hooks) > 0 {
			if err := Forward(ctx, db, &hooks[0], config.Timeout); err != nil && !errors.Is(err, context.Canceled) {
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

func performDeliveryAttempt(parent context.Context, rule model.ForwardingRule, h *model.Webhook, timeout time.Duration) (model.DeliveryResponse, string, bool) {
	return performDeliveryAttemptWithClient(parent, deliveryClient, rule, h, timeout)
}

func performDeliveryAttemptWithClient(parent context.Context, client *http.Client, rule model.ForwardingRule, h *model.Webhook, timeout time.Duration) (model.DeliveryResponse, string, bool) {
	var response model.DeliveryResponse
	if timeout <= 0 {
		timeout = DefaultDeliveryTimeout
	}
	deadline := time.Now().Add(timeout)
	if h.DeliveryLeaseUntil != nil {
		leaseDeadline := h.DeliveryLeaseUntil.Add(-DeliveryFinalizationReserve)
		if leaseDeadline.Before(deadline) {
			deadline = leaseDeadline
		}
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return response, err.Error(), false
	}

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
