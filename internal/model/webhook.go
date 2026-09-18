package model

import "time"

type Webhook struct {
	ID                   uint `gorm:"primaryKey"`
	Source               string
	Headers              string
	Payload              []byte
	Response             []byte
	ReceivedAt           time.Time `gorm:"index"`
	Status               string    `gorm:"index:idx_webhooks_delivery_queue"`
	FailureCount         int
	LastError            string
	NextRetryAt          *time.Time `gorm:"index:idx_webhooks_delivery_queue"`
	DeliveryLeaseUntil   *time.Time `gorm:"index:idx_webhooks_delivery_queue"`
	DeliveryLeaseVersion int64      `gorm:"not null;default:0" json:"-"`
	DeadLetteredAt       *time.Time
	DeadLetterReason     string
}
