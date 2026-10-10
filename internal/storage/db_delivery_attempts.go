package storage

import (
	"context"
	"errors"
	"sort"
	"webhookhub/internal/model"

	"gorm.io/gorm"
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

func (d *DB) DeliveryAttemptsByWebhook(ctx context.Context, webhookID uint) ([]model.DeliveryAttempt, error) {
	var attempts []model.DeliveryAttempt
	err := d.conn.WithContext(ctx).Omit("response_body", "response_headers").Where("webhook_id = ?", webhookID).Order("id desc").Find(&attempts).Error
	return attempts, err
}

func (d *DB) DeliveryAttemptByID(ctx context.Context, webhookID, attemptID uint) (model.DeliveryAttempt, error) {
	var attempt model.DeliveryAttempt
	err := d.conn.WithContext(ctx).Where("webhook_id = ? AND id = ?", webhookID, attemptID).First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return attempt, ErrNotFound
	}
	return attempt, err
}

func (d *DB) DeliveryMetrics(ctx context.Context) (DeliveryMetrics, error) {
	metrics := DeliveryMetrics{}
	var counts struct {
		TotalWebhooks   int64
		DeadLetterCount int64
	}

	if err := d.conn.WithContext(ctx).Model(&model.Webhook{}).
		Select("count(*) AS total_webhooks, count(*) FILTER (WHERE status = ?) AS dead_letter_count", "dead_lettered").
		Scan(&counts).Error; err != nil {
		return metrics, err
	}
	metrics.TotalWebhooks = counts.TotalWebhooks
	metrics.DeadLetterCount = counts.DeadLetterCount

	if err := d.conn.WithContext(ctx).Omit("response_body", "response_headers").Where("status = ?", "failed").Order("started_at desc").Limit(5).Find(&metrics.RecentFailures).Error; err != nil {
		return metrics, err
	}
	if err := d.conn.WithContext(ctx).Omit("payload", "headers", "response").Where("status = ?", "dead_lettered").Order("dead_lettered_at desc").Limit(10).Find(&metrics.RecentDeadLetters).Error; err != nil {
		return metrics, err
	}

	var rows []sourceStatusCount
	if err := d.conn.WithContext(ctx).Model(&model.DeliveryAttempt{}).
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

		metrics.TotalAttempts += row.Count
		item.Attempts += row.Count
		switch row.Status {
		case "success":
			metrics.SuccessCount += row.Count
			item.Success += row.Count
		case "failed":
			metrics.FailedCount += row.Count
			item.Failed += row.Count
		case "pending":
			metrics.PendingCount += row.Count
			item.Pending += row.Count
		case "skipped":
			metrics.SkippedCount += row.Count
			item.Skipped += row.Count
		}
	}

	completedAttempts := metrics.SuccessCount + metrics.FailedCount
	if completedAttempts > 0 {
		metrics.SuccessRate = float64(metrics.SuccessCount) * 100 / float64(completedAttempts)
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
