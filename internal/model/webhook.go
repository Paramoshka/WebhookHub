package model

import "time"

type Webhook struct {
	ID                 uint `gorm:"primaryKey"`
	Source             string
	Headers            string
	Payload            []byte
	Response           []byte
	ReceivedAt         time.Time `gorm:"index"`
	Status             string    `gorm:"index:idx_webhooks_delivery_queue"`
	FailureCount       int
	LastError          string
	NextRetryAt        *time.Time `gorm:"index:idx_webhooks_delivery_queue"`
	DeliveryLeaseUntil *time.Time `gorm:"index:idx_webhooks_delivery_queue"`
	DeadLetteredAt     *time.Time
	DeadLetterReason   string
}
