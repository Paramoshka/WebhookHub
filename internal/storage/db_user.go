package storage

import (
	"context"
	"errors"

	"webhookhub/internal/model"

	"gorm.io/gorm"
)

func (d *DB) FindUserByEmail(ctx context.Context, email string) (model.User, error) {
	var user model.User
	err := d.conn.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.User{}, ErrNotFound
	}
	return user, err
}
