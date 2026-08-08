package storage

import (
	"errors"

	"webhookhub/internal/model"

	"gorm.io/gorm"
)

func (d *DB) FindUserByEmail(email string) (model.User, error) {
	var user model.User
	err := d.conn.Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.User{}, ErrNotFound
	}
	return user, err
}
