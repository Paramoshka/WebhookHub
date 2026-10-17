package model

type ForwardingRule struct {
	ID                  uint   `gorm:"primaryKey"`
	Source              string `gorm:"uniqueIndex"`
	Target              string
	Paused              bool `gorm:"not null;default:false"`
	VerifySecret        string
	VerifyHeader        string
	ToleranceSeconds    int
	OutgoingSecret      string
	RetryMaxAttempts    int
	RetryBackoffSeconds int
}
