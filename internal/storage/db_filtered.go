package storage

import (
	"log"
	"webhookhub/internal/model"
)

func (d *DB) Filtered(source, status string, limit, offset int) []model.Webhook {
	var list []model.Webhook

	query := d.conn.Model(&model.Webhook{})

	if source != "" {
		query = query.Where("source = ?", source)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Order("id DESC").
		Limit(limit).
		Offset(offset).
		Find(&list).Error

	if err != nil {
		log.Println("DB Filtered Query Error:", err)
		return nil
	}

	return list
}

func (d *DB) CountFiltered(source, status string) int {
	var count int64

	query := d.conn.Model(&model.Webhook{})

	if source != "" {
		query = query.Where("source = ?", source)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Count(&count).Error
	if err != nil {
		log.Println("DB CountFiltered Error:", err)
		return 0
	}

	return int(count)
}
