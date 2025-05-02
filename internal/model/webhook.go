package model

import "time"

type Webhook struct {
	ID         uint `gorm:"primaryKey"`
	Source     string
	Headers    string
	Payload    []byte
	ReceivedAt time.Time
	Status     string
}
