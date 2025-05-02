package model

type ForwardingRule struct {
	ID     uint   `gorm:"primaryKey"`
	Source string `gorm:"uniqueIndex"`
	Target string
}
