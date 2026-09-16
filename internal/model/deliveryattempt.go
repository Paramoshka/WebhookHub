package model

import "time"

type DeliveryAttempt struct {
	ID                uint `gorm:"primaryKey"`
	WebhookID         uint `gorm:"index"`
	Source            string
	Target            string
	Status            string
	HTTPStatus        int
	ErrorMessage      string
	DurationMS        int64
	StartedAt         time.Time
	CompletedAt       *time.Time
	ResponseBody      []byte
	ResponseHeaders   string
	ResponseCaptured  bool `gorm:"not null;default:false"`
	ResponseTruncated bool `gorm:"not null;default:false"`
}

// DeliveryResponse contains the response received during a single HTTP attempt.
type DeliveryResponse struct {
	HTTPStatus int
	Body       []byte
	Headers    string
	Captured   bool
	Truncated  bool
}
