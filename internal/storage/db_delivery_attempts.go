package storage

import (
	"sort"
	"time"
	"webhookhub/internal/model"
)

type DeliveryMetrics struct {
	TotalWebhooks     int64
	TotalAttempts     int64
	SuccessCount      int64
	FailedCount       int64
	PendingCount      int64
	SkippedCount      int64
	DeadLetterCount   int64
	SuccessRate       float64
	RecentFailures    []model.DeliveryAttempt
	RecentDeadLetters []model.Webhook
	SourceBreakdown   []SourceDeliveryMetric
}

type SourceDeliveryMetric struct {
	Source      string
	Attempts    int64
	Success     int64
	Failed      int64
	Pending     int64
	Skipped     int64
	SuccessRate float64
}

type sourceStatusCount struct {
	Source string
	Status string
	Count  int64
}

func (d *DB) CreateDeliveryAttempt(attempt *model.DeliveryAttempt) (uint, error) {
	if err := d.conn.Create(attempt).Error; err != nil {
		return 0, err
	}
	return attempt.ID, nil
}

func (d *DB) FinishDeliveryAttempt(id uint, status string, httpStatus int, errMsg string, durationMS int64) error {
	if id == 0 {
		return nil
	}

	completedAt := time.Now()
	return d.conn.Model(&model.DeliveryAttempt{}).Where("id = ?", id).Updates(map[string]any{
		"status":        status,
		"http_status":   httpStatus,
		"error_message": errMsg,
		"duration_ms":   durationMS,
		"completed_at":  &completedAt,
	}).Error
}

func (d *DB) DeliveryAttemptsByWebhook(webhookID uint) ([]model.DeliveryAttempt, error) {
	var attempts []model.DeliveryAttempt
	err := d.conn.Where("webhook_id = ?", webhookID).Order("id desc").Find(&attempts).Error
	return attempts, err
}

func (d *DB) DeliveryMetrics() (DeliveryMetrics, error) {
	metrics := DeliveryMetrics{}

	if err := d.conn.Model(&model.Webhook{}).Count(&metrics.TotalWebhooks).Error; err != nil {
		return metrics, err
	}
	if err := d.conn.Model(&model.DeliveryAttempt{}).Count(&metrics.TotalAttempts).Error; err != nil {
		return metrics, err
	}
	if err := d.conn.Model(&model.DeliveryAttempt{}).Where("status = ?", "success").Count(&metrics.SuccessCount).Error; err != nil {
		return metrics, err
	}
	if err := d.conn.Model(&model.DeliveryAttempt{}).Where("status = ?", "failed").Count(&metrics.FailedCount).Error; err != nil {
		return metrics, err
	}
	if err := d.conn.Model(&model.DeliveryAttempt{}).Where("status = ?", "pending").Count(&metrics.PendingCount).Error; err != nil {
		return metrics, err
	}
	if err := d.conn.Model(&model.DeliveryAttempt{}).Where("status = ?", "skipped").Count(&metrics.SkippedCount).Error; err != nil {
		return metrics, err
	}
	if err := d.conn.Model(&model.Webhook{}).Where("status = ?", "dead_lettered").Count(&metrics.DeadLetterCount).Error; err != nil {
		return metrics, err
	}

	completedAttempts := metrics.SuccessCount + metrics.FailedCount
	if completedAttempts > 0 {
		metrics.SuccessRate = float64(metrics.SuccessCount) * 100 / float64(completedAttempts)
	}

	if err := d.conn.Where("status = ?", "failed").Order("started_at desc").Limit(5).Find(&metrics.RecentFailures).Error; err != nil {
		return metrics, err
	}
	if err := d.conn.Where("status = ?", "dead_lettered").Order("dead_lettered_at desc").Limit(10).Find(&metrics.RecentDeadLetters).Error; err != nil {
		return metrics, err
	}

	var rows []sourceStatusCount
	if err := d.conn.Model(&model.DeliveryAttempt{}).
		Select("source, status, count(*) as count").
		Group("source, status").
		Scan(&rows).Error; err != nil {
		return metrics, err
	}

	bySource := make(map[string]*SourceDeliveryMetric)
	for _, row := range rows {
		item, found := bySource[row.Source]
		if !found {
			item = &SourceDeliveryMetric{Source: row.Source}
			bySource[row.Source] = item
		}

		item.Attempts += row.Count
		switch row.Status {
		case "success":
			item.Success += row.Count
		case "failed":
			item.Failed += row.Count
		case "pending":
			item.Pending += row.Count
		case "skipped":
			item.Skipped += row.Count
		}
	}

	for _, item := range bySource {
		completedAttempts := item.Success + item.Failed
		if completedAttempts > 0 {
			item.SuccessRate = float64(item.Success) * 100 / float64(completedAttempts)
		}
		metrics.SourceBreakdown = append(metrics.SourceBreakdown, *item)
	}

	sort.Slice(metrics.SourceBreakdown, func(i, j int) bool {
		return metrics.SourceBreakdown[i].Source < metrics.SourceBreakdown[j].Source
	})

	return metrics, nil
}
