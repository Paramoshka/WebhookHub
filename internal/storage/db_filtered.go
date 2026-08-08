package storage

import (
	"strings"
	"time"
	"webhookhub/internal/model"

	"gorm.io/gorm"
)

type WebhookFilter struct {
	Source string
	Status string
	Query  string
	From   *time.Time
	To     *time.Time
	Sort   string
}

func (d *DB) Filtered(filter WebhookFilter, limit, offset int) ([]model.Webhook, error) {
	var list []model.Webhook

	query := d.applyWebhookFilter(d.conn.Model(&model.Webhook{}), filter)
	query = d.applyWebhookSort(query, filter.Sort)
	err := query.
		Limit(limit).
		Offset(offset).
		Find(&list).Error

	return list, err
}

func (d *DB) CountFiltered(filter WebhookFilter) (int, error) {
	var count int64

	query := d.applyWebhookFilter(d.conn.Model(&model.Webhook{}), filter)
	err := query.Count(&count).Error
	return int(count), err
}

func (d *DB) applyWebhookFilter(query *gorm.DB, filter WebhookFilter) *gorm.DB {
	if filter.Source != "" {
		query = query.Where("source = ?", strings.TrimSpace(filter.Source))
	}
	if filter.Status != "" {
		query = query.Where("status = ?", strings.TrimSpace(filter.Status))
	}
	if filter.Query != "" {
		pattern := "%" + strings.TrimSpace(filter.Query) + "%"
		query = query.Where(
			"source ILIKE ? OR CAST(payload AS TEXT) ILIKE ? OR headers ILIKE ? OR last_error ILIKE ? OR dead_letter_reason ILIKE ?",
			pattern, pattern, pattern, pattern, pattern,
		)
	}
	if filter.From != nil {
		query = query.Where("received_at >= ?", filter.From)
	}
	if filter.To != nil {
		query = query.Where("received_at < ?", filter.To)
	}

	return query
}

func (d *DB) applyWebhookSort(query *gorm.DB, sort string) *gorm.DB {
	switch strings.TrimSpace(sort) {
	case "id_asc":
		return query.Order("id ASC")
	case "received_asc":
		return query.Order("received_at ASC")
	case "received_desc":
		return query.Order("received_at DESC")
	case "id_desc":
		return query.Order("id DESC")
	default:
		return query.Order("id DESC")
	}
}
