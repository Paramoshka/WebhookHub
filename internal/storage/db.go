package storage

import (
	"database/sql"
	"log"
	"webhookhub/internal/model"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func InitDB(path string) *DB {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		log.Fatal(err)
	}

	stmt := `CREATE TABLE IF NOT EXISTS webhooks (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        source TEXT,
        headers TEXT,
        payload BLOB,
        received_at DATETIME,
        status TEXT
    );`
	_, err = db.Exec(stmt)
	if err != nil {
		log.Fatal(err)
	}

	return &DB{conn: db}
}

func (d *DB) Save(h model.Webhook) {
	_, err := d.conn.Exec(
		"INSERT INTO webhooks (source, headers, payload, received_at, status) VALUES (?, ?, ?, ?, ?)",
		h.Source, h.Headers, h.Payload, h.ReceivedAt, h.Status,
	)
	if err != nil {
		log.Println("DB Save Error:", err)
	}
}

func (d *DB) All() []model.Webhook {
	rows, _ := d.conn.Query("SELECT id, source, headers, payload, received_at, status FROM webhooks ORDER BY id DESC")
	defer rows.Close()

	var list []model.Webhook
	for rows.Next() {
		var h model.Webhook
		rows.Scan(&h.ID, &h.Source, &h.Headers, &h.Payload, &h.ReceivedAt, &h.Status)
		list = append(list, h)
	}
	return list
}

func (d *DB) FindByID(id string) (model.Webhook, bool) {
	row := d.conn.QueryRow("SELECT id, source, headers, payload, received_at, status FROM webhooks WHERE id = ?", id)
	var h model.Webhook
	err := row.Scan(&h.ID, &h.Source, &h.Headers, &h.Payload, &h.ReceivedAt, &h.Status)
	return h, err == nil
}

func (d *DB) UpdateStatus(id int, status string) {
	d.conn.Exec("UPDATE webhooks SET status = ? WHERE id = ?", status, id)
}
