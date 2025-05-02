package storage

import "webhookhub/internal/model"

func (d *DB) FindUserByEmail(email string) (model.User, bool) {
	var user model.User
	err := d.conn.Where("email = ?", email).First(&user).Error
	return user, err == nil
}
