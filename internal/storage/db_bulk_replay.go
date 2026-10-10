package storage

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"webhookhub/internal/model"
)

const MaxBulkReplay = 20

type BulkReplayResult struct {
	Requeued int
	Skipped  int
}

func (d *DB) BulkReplayDeadLetters(ctx context.Context, ids []int) (BulkReplayResult, error) {
	unique := make([]int, 0, len(ids))
	seen := make(map[int]bool)
	for _, id := range ids {
		if id <= 0 {
			return BulkReplayResult{}, errors.New("invalid webhook ID")
		}
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	if len(unique) == 0 || len(unique) > MaxBulkReplay {
		return BulkReplayResult{}, errors.New("select between 1 and 20 webhooks")
	}

	result := BulkReplayResult{}
	err := d.conn.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var eligible []int
		if err := tx.Model(&model.Webhook{}).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ? AND status = ?", unique, "dead_lettered").
			Order("id asc").Pluck("id", &eligible).Error; err != nil {
			return err
		}
		result.Skipped = len(unique) - len(eligible)
		if len(eligible) == 0 {
			return nil
		}
		updated := tx.Model(&model.Webhook{}).Where("id IN ?", eligible).Updates(replayUpdates())
		if updated.Error != nil {
			return updated.Error
		}
		result.Requeued = int(updated.RowsAffected)
		return nil
	})
	if err != nil {
		return BulkReplayResult{}, err
	}
	return result, nil
}
