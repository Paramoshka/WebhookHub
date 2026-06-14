package model

import "time"

type DeliveryAttempt struct {
	ID           uint `gorm:"primaryKey"`
	WebhookID    uint `gorm:"index"`
	Source       string
	Target       string
	Status       string
	HTTPStatus   int
	ErrorMessage string
	DurationMS   int64
	StartedAt    time.Time
	CompletedAt  *time.Time
}
