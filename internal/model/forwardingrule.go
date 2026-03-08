package model

type ForwardingRule struct {
	ID               uint   `gorm:"primaryKey"`
	Source           string `gorm:"uniqueIndex"`
	Target           string
	VerifySecret     string
	VerifyHeader     string
	ToleranceSeconds int
	OutgoingSecret   string
}
