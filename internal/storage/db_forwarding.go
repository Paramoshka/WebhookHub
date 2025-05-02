package storage

import (
	"log"
	"webhookhub/internal/model"

	"gorm.io/gorm/clause"
)

// Save or update forwarding rule
func (d *DB) SaveForwardingRule(source, target string) {
	rule := model.ForwardingRule{
		Source: source,
		Target: target,
	}
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

// Get All forwarding rules in map[source]target
func (d *DB) GetForwardingRules() map[string]string {
	var rules []model.ForwardingRule
	err := d.conn.Find(&rules).Error
	if err != nil {
		log.Println("DB GetForwardingRules Error:", err)
		return nil
	}

	out := map[string]string{}
	for _, r := range rules {
		out[r.Source] = r.Target
	}
	return out
}

// Get target by source
func (d *DB) GetTargetForSource(source string) string {
	var rule model.ForwardingRule
	err := d.conn.Where("source = ?", source).First(&rule).Error
	if err != nil {
		return ""
	}
	return rule.Target
}
