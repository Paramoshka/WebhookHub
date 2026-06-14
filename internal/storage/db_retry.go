package storage

import (
	"log"
	"time"
	"webhookhub/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (d *DB) ClaimDueRetryWebhooks(limit int, now time.Time) []model.Webhook {
	if limit <= 0 {
		return nil
	}

	var hooks []model.Webhook
	err := d.conn.Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND next_retry_at IS NOT NULL AND next_retry_at <= ?", "retrying", now).
			Order("next_retry_at asc").
			Limit(limit).
			Find(&hooks).Error; err != nil {
			return err
		}

		if len(hooks) == 0 {
			return nil
		}

		ids := make([]uint, 0, len(hooks))
		for _, hook := range hooks {
			ids = append(ids, hook.ID)
		}

		return tx.Model(&model.Webhook{}).
			Where("id IN ?", ids).
			Updates(map[string]any{
				"status":        "pending",
				"next_retry_at": nil,
			}).Error
	})
	if err != nil {
		log.Println("DB ClaimDueRetryWebhooks Error:", err)
		return nil
	}

	return hooks
}
