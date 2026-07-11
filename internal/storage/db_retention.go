package storage

import (
	"log"
	"time"

	"webhookhub/internal/model"

	"gorm.io/gorm"
)

func StartRetentionWorker(db *DB, retentionDays int, interval time.Duration, batchSize int) {
	if db == nil || retentionDays <= 0 || interval <= 0 {
		return
	}

	if batchSize <= 0 {
		batchSize = 300
	}

	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			cutoff := time.Now().AddDate(0, 0, -retentionDays)
			deleted, err := db.CleanupExpiredWebhooks(cutoff, batchSize)
			if err != nil {
				log.Printf("cleanup expired webhooks failed: %v", err)
				continue
			}
			if deleted > 0 {
				log.Printf("cleanup expired webhooks: removed %d records", deleted)
			}
		}
	}()
}

func (d *DB) CleanupExpiredWebhooks(before time.Time, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 300
	}

	var totalDeleted int64

	for {
		var ids []uint
		err := d.conn.Model(&model.Webhook{}).
			Where("received_at < ?", before).
			Order("id asc").
			Limit(batchSize).
			Pluck("id", &ids).
			Error
		if err != nil {
			return totalDeleted, err
		}
		if len(ids) == 0 {
			return totalDeleted, nil
		}

		err = d.conn.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("webhook_id IN ?", ids).Delete(&model.DeliveryAttempt{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", ids).Delete(&model.Webhook{}).Error; err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return totalDeleted, err
		}

		totalDeleted += int64(len(ids))
		if len(ids) < batchSize {
			return totalDeleted, nil
		}
	}
}
