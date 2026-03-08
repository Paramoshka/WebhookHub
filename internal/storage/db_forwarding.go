package storage

import (
	"log"
	"webhookhub/internal/model"

	"gorm.io/gorm/clause"
)

// Save or update forwarding rule
func (d *DB) SaveForwardingRule(rule model.ForwardingRule) {
	err := d.conn.
		Clauses(
			// If source exists to update
			clause.OnConflict{
				Columns:   []clause.Column{{Name: "source"}},
				UpdateAll: true,
			},
		).
		Create(&rule).Error

	if err != nil {
		log.Println("DB SaveForwardingRule Error:", err)
	}
}

// Get all forwarding rules ordered by source
func (d *DB) GetForwardingRules() []model.ForwardingRule {
	var rules []model.ForwardingRule
	err := d.conn.Order("source asc").Find(&rules).Error
	if err != nil {
		log.Println("DB GetForwardingRules Error:", err)
		return nil
	}
	return rules
}

// Get rule by source
func (d *DB) GetForwardingRule(source string) (model.ForwardingRule, bool) {
	var rule model.ForwardingRule
	err := d.conn.Where("source = ?", source).First(&rule).Error
	if err != nil {
		return model.ForwardingRule{}, false
	}
	return rule, true
}
