package storage

import (
	"context"
	"errors"
	"webhookhub/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (d *DB) SaveForwardingRule(ctx context.Context, rule model.ForwardingRule) error {
	return d.conn.WithContext(ctx).
		Clauses(
			clause.OnConflict{
				Columns:   []clause.Column{{Name: "source"}},
				UpdateAll: true,
			},
		).
		Create(&rule).Error
}

func (d *DB) GetForwardingRules(ctx context.Context) ([]model.ForwardingRule, error) {
	var rules []model.ForwardingRule
	err := d.conn.WithContext(ctx).Order("source asc").Find(&rules).Error
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
