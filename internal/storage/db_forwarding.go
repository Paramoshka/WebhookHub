package storage

import (
	"context"
	"errors"
	"webhookhub/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (d *DB) SaveForwardingRule(rule model.ForwardingRule) error {
	return d.conn.
		Clauses(
			clause.OnConflict{
				Columns:   []clause.Column{{Name: "source"}},
				UpdateAll: true,
			},
		).
		Create(&rule).Error
}

func (d *DB) GetForwardingRules() ([]model.ForwardingRule, error) {
	var rules []model.ForwardingRule
	err := d.conn.Order("source asc").Find(&rules).Error
	return rules, err
}

func (d *DB) GetForwardingRule(ctx context.Context, source string) (model.ForwardingRule, error) {
	var rule model.ForwardingRule
	err := d.conn.WithContext(ctx).Where("source = ?", source).First(&rule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ForwardingRule{}, ErrNotFound
	}
	return rule, err
}
