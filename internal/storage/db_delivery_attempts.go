package storage

import (
	"log"
	"sort"
	"time"
	"webhookhub/internal/model"
)

type DeliveryMetrics struct {
	TotalWebhooks   int64
	TotalAttempts   int64
	SuccessCount    int64
	FailedCount     int64
	PendingCount    int64
	SkippedCount    int64
	SuccessRate     float64
	RecentFailures  []model.DeliveryAttempt
	SourceBreakdown []SourceDeliveryMetric
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

func (d *DB) CreateDeliveryAttempt(attempt *model.DeliveryAttempt) uint {
	if err := d.conn.Create(attempt).Error; err != nil {
		log.Println("DB CreateDeliveryAttempt Error:", err)
		return 0
	}
	return attempt.ID
}

func (d *DB) FinishDeliveryAttempt(id uint, status string, httpStatus int, errMsg string, durationMS int64) {
	if id == 0 {
		return
	}

	completedAt := time.Now()
	if err := d.conn.Model(&model.DeliveryAttempt{}).Where("id = ?", id).Updates(map[string]any{
		"status":        status,
		"http_status":   httpStatus,
		"error_message": errMsg,
		"duration_ms":   durationMS,
		"completed_at":  &completedAt,
	}).Error; err != nil {
		log.Println("DB FinishDeliveryAttempt Error:", err)
	}
}

func (d *DB) DeliveryAttemptsByWebhook(webhookID uint) []model.DeliveryAttempt {
	var attempts []model.DeliveryAttempt
	if err := d.conn.Where("webhook_id = ?", webhookID).Order("id desc").Find(&attempts).Error; err != nil {
		log.Println("DB DeliveryAttemptsByWebhook Error:", err)
		return nil
	}
	return attempts
}

func (d *DB) DeliveryMetrics() DeliveryMetrics {
	metrics := DeliveryMetrics{}

	if err := d.conn.Model(&model.Webhook{}).Count(&metrics.TotalWebhooks).Error; err != nil {
		log.Println("DB DeliveryMetrics TotalWebhooks Error:", err)
	}
	if err := d.conn.Model(&model.DeliveryAttempt{}).Count(&metrics.TotalAttempts).Error; err != nil {
		log.Println("DB DeliveryMetrics TotalAttempts Error:", err)
	}
	if err := d.conn.Model(&model.DeliveryAttempt{}).Where("status = ?", "success").Count(&metrics.SuccessCount).Error; err != nil {
		log.Println("DB DeliveryMetrics SuccessCount Error:", err)
	}
	if err := d.conn.Model(&model.DeliveryAttempt{}).Where("status = ?", "failed").Count(&metrics.FailedCount).Error; err != nil {
		log.Println("DB DeliveryMetrics FailedCount Error:", err)
	}
	if err := d.conn.Model(&model.DeliveryAttempt{}).Where("status = ?", "pending").Count(&metrics.PendingCount).Error; err != nil {
		log.Println("DB DeliveryMetrics PendingCount Error:", err)
	}
	if err := d.conn.Model(&model.DeliveryAttempt{}).Where("status = ?", "skipped").Count(&metrics.SkippedCount).Error; err != nil {
		log.Println("DB DeliveryMetrics SkippedCount Error:", err)
	}

	completedAttempts := metrics.SuccessCount + metrics.FailedCount
	if completedAttempts > 0 {
		metrics.SuccessRate = float64(metrics.SuccessCount) * 100 / float64(completedAttempts)
	}

	if err := d.conn.Where("status = ?", "failed").Order("started_at desc").Limit(5).Find(&metrics.RecentFailures).Error; err != nil {
		log.Println("DB DeliveryMetrics RecentFailures Error:", err)
	}

	var rows []sourceStatusCount
	if err := d.conn.Model(&model.DeliveryAttempt{}).
		Select("source, status, count(*) as count").
		Group("source, status").
		Scan(&rows).Error; err != nil {
		log.Println("DB DeliveryMetrics SourceBreakdown Error:", err)
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

	return metrics
}
