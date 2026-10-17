package storage

import (
	"context"
	"errors"
	"time"
	"webhookhub/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (d *DB) SaveForwardingRule(ctx context.Context, rule model.ForwardingRule) error {
	return d.conn.WithContext(ctx).
		Clauses(
			clause.OnConflict{
				Columns: []clause.Column{{Name: "source"}},
				// Pause is changed separately so a stale rule edit cannot resume delivery.
				DoUpdates: clause.AssignmentColumns([]string{
					"target", "verify_secret", "verify_header", "tolerance_seconds",
					"outgoing_secret", "retry_max_attempts", "retry_backoff_seconds",
				}),
			},
		).
		Create(&rule).Error
}

func (d *DB) SetForwardingPaused(ctx context.Context, source string, paused bool) error {
	result := d.conn.WithContext(ctx).Model(&model.ForwardingRule{}).
		Where("source = ?", source).Update("paused", paused)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) QueuedWebhookCounts(ctx context.Context) (map[string]int64, error) {
	var counts []struct {
		Source string
		Count  int64
	}
	if err := d.conn.WithContext(ctx).Model(&model.Webhook{}).
		Select("source, COUNT(*) AS count").
		Where("status IN ? OR (status = ? AND delivery_lease_until <= ?)", []string{"pending", "retrying"}, "processing", time.Now()).
		Group("source").Scan(&counts).Error; err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(counts))
	for _, row := range counts {
		result[row.Source] = row.Count
	}
	return result, nil
}

func (d *DB) GetForwardingRules(ctx context.Context) ([]model.ForwardingRule, error) {
	var rules []model.ForwardingRule
	err := d.conn.WithContext(ctx).Order("source asc").Find(&rules).Error
	return rules, err
}

func (d *DB) GetForwardingRule(ctx context.Context, source string) (model.ForwardingRule, error) {
	var rule model.ForwardingRule
	err := d.conn.WithContext(ctx).Where("source = ?", source).First(&rule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ForwardingRule{}, ErrNotFound
	}
	return rule, err
}
