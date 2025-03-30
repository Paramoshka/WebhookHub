package storage

import (
	"log"
	"webhookhub/internal/model"
)

func (d *DB) Filtered(source, status string, limit, offset int) []model.Webhook {
	query := "SELECT id, source, headers, payload, received_at, status FROM webhooks WHERE 1=1"
	args := []interface{}{}

	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		log.Println("DB Filtered Query Error:", err)
		return nil
	}
	defer rows.Close()

	var list []model.Webhook
	for rows.Next() {
		var h model.Webhook
		rows.Scan(&h.ID, &h.Source, &h.Headers, &h.Payload, &h.ReceivedAt, &h.Status)
		list = append(list, h)
	}
	return list
}

func (d *DB) CountFiltered(source, status string) int {
	query := "SELECT COUNT(*) FROM webhooks WHERE 1=1"
	args := []interface{}{}

	if source != "" {
		query += " AND source = ?"
		args = append(args, source)
	}

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	var count int
	err := d.conn.QueryRow(query, args...).Scan(&count)
	if err != nil {
		log.Println("DB CountFiltered Error:", err)
		return 0
	}
	return count
}
