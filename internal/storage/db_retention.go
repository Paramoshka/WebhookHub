package storage

import (
	"context"
	"log"
	"sync"
	"time"

	"webhookhub/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func StartRetentionWorker(ctx context.Context, db *DB, retentionDays int, interval time.Duration, batchSize int) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	if db == nil || retentionDays <= 0 || interval <= 0 {
		return wg
	}

	if batchSize <= 0 {
		batchSize = 300
	}

	cleanup := func() {
		cutoff := time.Now().AddDate(0, 0, -retentionDays)
		deleted, err := db.CleanupExpiredWebhooks(cutoff, batchSize)
		if err != nil {
			log.Printf("cleanup expired webhooks failed: %v", err)
			return
		}
		if deleted > 0 {
			log.Printf("cleanup expired webhooks: removed %d records", deleted)
		}
	}

	ticker := time.NewTicker(interval)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer ticker.Stop()
		// Run once on startup so daily restarts do not postpone cleanup forever.
		cleanup()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanup()
			}
		}
	}()

	return wg
}

func (d *DB) CleanupExpiredWebhooks(before time.Time, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 300
	}

	var totalDeleted int64

	for {
		var ids []uint
		err := d.conn.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.Webhook{}).
				Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Where("received_at < ? AND status NOT IN ?", before, []string{"pending", "processing", "retrying"}).
				Order("id asc").
				Limit(batchSize).
				Pluck("id", &ids).Error; err != nil {
				return err
			}
			if len(ids) == 0 {
				return nil
			}
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
