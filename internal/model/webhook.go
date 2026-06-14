package model

import "time"

type Webhook struct {
	ID               uint `gorm:"primaryKey"`
	Source           string
	Headers          string
	Payload          []byte
	Response         []byte
	ReceivedAt       time.Time
	Status           string `gorm:"index:idx_webhooks_status_next_retry"`
	FailureCount     int
	LastError        string
	NextRetryAt      *time.Time `gorm:"index:idx_webhooks_status_next_retry"`
	DeadLetteredAt   *time.Time
	DeadLetterReason string
}
