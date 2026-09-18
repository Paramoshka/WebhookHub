package storage

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"webhookhub/internal/model"
)

// DeliveryResult is applied to the attempt and its webhook in one transaction.
// A failed attempt without NextRetryAt moves the webhook to the dead-letter queue.
type DeliveryResult struct {
	Status       string
	Response     model.DeliveryResponse
	ErrorMessage string
	DurationMS   int64
	NextRetryAt  *time.Time
}

func (d *DB) CreateDeliveryAttempt(ctx context.Context, claim *model.Webhook, target string) (uint, error) {
	if claim.DeliveryLeaseUntil == nil || !time.Now().Before(*claim.DeliveryLeaseUntil) {
		return 0, ErrLeaseLost
	}
	ctx, cancel := context.WithDeadline(ctx, *claim.DeliveryLeaseUntil)
	defer cancel()
	var attempt model.DeliveryAttempt
	err := d.conn.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		webhook, err := lockDeliveryLease(tx, claim)
		if err != nil {
			return err
		}
		// A claim owns one attempt, even if the caller retries creation.
		var count int64
		if err := tx.Model(&model.DeliveryAttempt{}).Where("webhook_id = ? AND status = ?", webhook.ID, "pending").Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("delivery attempt already in progress")
		}
		attempt = model.DeliveryAttempt{WebhookID: webhook.ID, Source: webhook.Source, Target: target, Status: "pending", StartedAt: time.Now()}
		return tx.Create(&attempt).Error
	})
	return attempt.ID, err
}

func (d *DB) FinishDelivery(ctx context.Context, claim *model.Webhook, attemptID uint, result DeliveryResult) error {
	switch result.Status {
	case "success", "failed", "cancelled", "skipped":
	default:
		return errors.New("invalid delivery result status")
	}
	if claim.DeliveryLeaseUntil == nil || !time.Now().Before(*claim.DeliveryLeaseUntil) {
		return ErrLeaseLost
	}
	ctx, cancel := context.WithDeadline(ctx, *claim.DeliveryLeaseUntil)
	defer cancel()
	return d.conn.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		webhook, err := lockDeliveryLease(tx, claim)
		if err != nil {
			return err
		}
		completedAt := time.Now()
		updated := tx.Model(&model.DeliveryAttempt{}).
			Where("id = ? AND webhook_id = ? AND status = ?", attemptID, webhook.ID, "pending").
			Updates(map[string]any{
				"status":             result.Status,
				"http_status":        result.Response.HTTPStatus,
				"error_message":      result.ErrorMessage,
				"duration_ms":        result.DurationMS,
				"completed_at":       completedAt,
				"response_body":      result.Response.Body,
				"response_headers":   result.Response.Headers,
				"response_captured":  result.Response.Captured,
				"response_truncated": result.Response.Truncated,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrLeaseLost
		}
		updates := map[string]any{"delivery_lease_until": nil}
		if result.Status != "skipped" {
			updates["response"] = result.Response.Body
		}
		switch result.Status {
		case "success", "skipped":
			updates["status"] = result.Status
			updates["failure_count"] = 0
			updates["last_error"] = ""
			updates["next_retry_at"] = nil
			updates["dead_lettered_at"] = nil
			updates["dead_letter_reason"] = ""
		case "cancelled":
			updates["status"] = "pending"
		case "failed":
			updates["failure_count"] = webhook.FailureCount + 1
			updates["last_error"] = result.ErrorMessage
			updates["next_retry_at"] = result.NextRetryAt
			if result.NextRetryAt == nil {
				updates["status"] = "dead_lettered"
				updates["dead_lettered_at"] = completedAt
				updates["dead_letter_reason"] = result.ErrorMessage
			} else {
				updates["status"] = "retrying"
				updates["dead_lettered_at"] = nil
				updates["dead_letter_reason"] = ""
			}
		}
		return tx.Model(&webhook).Updates(updates).Error
	})
}

// All delivery mutations lock the webhook before touching attempts, just as
// claim, replay and retention do. The version fences off a previous worker.
func lockDeliveryLease(tx *gorm.DB, claim *model.Webhook) (model.Webhook, error) {
	webhook, err := lockWebhook(tx, int(claim.ID))
	if errors.Is(err, ErrNotFound) {
		return model.Webhook{}, ErrLeaseLost
	}
	if err != nil {
		return model.Webhook{}, err
	}
	if claim.DeliveryLeaseVersion <= 0 || webhook.DeliveryLeaseVersion != claim.DeliveryLeaseVersion ||
		webhook.Status != "processing" || webhook.DeliveryLeaseUntil == nil || !time.Now().Before(*webhook.DeliveryLeaseUntil) {
		return model.Webhook{}, ErrLeaseLost
	}
	return webhook, nil
}
