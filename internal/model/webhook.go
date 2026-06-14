package model

import "time"

type Webhook struct {
	ID               uint `gorm:"primaryKey"`
	Source           string
	Headers          string
	Payload          []byte
	Response         []byte
	ReceivedAt       time.Time
	Status           string
	FailureCount     int
	DeadLetteredAt   *time.Time
	DeadLetterReason string
}
