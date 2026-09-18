package storage

import (
	"context"
	"time"
	"webhookhub/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (d *DB) ClaimDeliverableWebhooks(ctx context.Context, limit int, now, leaseUntil time.Time) ([]model.Webhook, error) {
	if limit <= 0 {
		return nil, nil
	}

	var hooks []model.Webhook
	err := d.conn.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where(
				"status = ? OR (status = ? AND next_retry_at IS NOT NULL AND next_retry_at <= ?) OR (status = ? AND delivery_lease_until IS NOT NULL AND delivery_lease_until <= ?)",
				"pending", "retrying", now, "processing", now,
			).
			Order("COALESCE(next_retry_at, received_at) asc").
			Limit(limit).
			Find(&hooks).Error; err != nil {
			return err
		}

		if len(hooks) == 0 {
			return nil
		}

		ids := make([]uint, 0, len(hooks))
		staleProcessingIDs := make([]uint, 0, len(hooks))
		for _, hook := range hooks {
			ids = append(ids, hook.ID)
			if hook.Status == "processing" {
				staleProcessingIDs = append(staleProcessingIDs, hook.ID)
			}
		}

		if len(staleProcessingIDs) > 0 {
			completedAt := now
			if err := tx.Model(&model.DeliveryAttempt{}).
				Where("webhook_id IN ? AND status = ?", staleProcessingIDs, "pending").
				Updates(map[string]any{
					"status":        "interrupted",
					"error_message": "delivery worker lease expired",
					"completed_at":  &completedAt,
				}).Error; err != nil {
				return err
			}
		}

		return tx.Model(&model.Webhook{}).
			Where("id IN ?", ids).
			Updates(map[string]any{
				"status":                 "processing",
				"next_retry_at":          nil,
				"delivery_lease_until":   &leaseUntil,
				"delivery_lease_version": gorm.Expr("delivery_lease_version + 1"),
			}).Error
	})
	if err != nil {
		return nil, err
	}

	for i := range hooks {
		hooks[i].DeliveryLeaseVersion++
		hooks[i].Status = "processing"
		hooks[i].NextRetryAt = nil
		hooks[i].DeliveryLeaseUntil = &leaseUntil
	}

	return hooks, nil
}
