package storage

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"time"

	"webhookhub/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type DB struct {
	conn *gorm.DB
}

var (
	ErrNotFound          = errors.New("not found")
	ErrWebhookProcessing = errors.New("webhook delivery is in progress")
)

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

func Open(config Config) (*DB, error) {
	databaseURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(config.User, config.Password),
		Host:   net.JoinHostPort(config.Host, config.Port),
		Path:   config.Name,
	}
	query := databaseURL.Query()
	query.Set("sslmode", config.SSLMode)
	databaseURL.RawQuery = query.Encode()

	databaseLogger := logger.New(
		log.New(os.Stdout, "", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
	db, err := gorm.Open(postgres.Open(databaseURL.String()), &gorm.Config{Logger: databaseLogger})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&model.Webhook{}, &model.DeliveryAttempt{}, &model.ForwardingRule{}, &model.User{}); err != nil {
		return nil, err
	}
	if err := initPayloadSearch(db); err != nil {
		return nil, fmt.Errorf("initialize payload search: %w", err)
	}

	return &DB{conn: db}, nil
}

func (d *DB) EnsureAdmin(email, password string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return d.conn.Transaction(func(tx *gorm.DB) error {
		var user model.User
		err := tx.Where("email = ?", email).First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			var users []model.User
			if err := tx.Order("id asc").Limit(2).Find(&users).Error; err != nil {
				return err
			}
			switch len(users) {
			case 0:
				return tx.Create(&model.User{
					Email:    email,
					Password: string(hashed),
					IsAdmin:  true,
				}).Error
			case 1:
				user = users[0]
				err = nil
			default:
				return errors.New("ADMIN_EMAIL does not match an existing user")
			}
		}
		if err != nil {
			return err
		}
		if user.IsAdmin && bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) == nil {
			return nil
		}

		return tx.Model(&user).Updates(map[string]any{
			"email":    email,
			"password": string(hashed),
			"is_admin": true,
		}).Error
	})
}

func (d *DB) Ping(ctx context.Context) error {
	sqlDB, err := d.conn.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (d *DB) Close() error {
	sqlDB, err := d.conn.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (d *DB) Save(h *model.Webhook) error {
	return d.conn.Create(h).Error
}

func (d *DB) All() ([]model.Webhook, error) {
	var list []model.Webhook
	err := d.conn.Order("id desc").Find(&list).Error
	return list, err
}

func (d *DB) FindByID(id int) (model.Webhook, error) {
	var h model.Webhook
	err := d.conn.Where("id = ?", id).First(&h).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Webhook{}, ErrNotFound
	}
	return h, err
}

func (d *DB) UpdateResponseFromForward(id int, resp []byte) error {
	return d.conn.Model(&model.Webhook{}).Where("id = ?", id).Update("response", resp).Error
}

func (d *DB) ResetWebhookDeliveryState(id int) error {
	return d.conn.Transaction(func(tx *gorm.DB) error {
		webhook, err := lockWebhook(tx, id)
		if err != nil {
			return err
		}
		if webhook.Status == "processing" {
			return ErrWebhookProcessing
		}

		return tx.Model(&webhook).Updates(map[string]any{
			"status":               "pending",
			"response":             []byte(nil),
			"failure_count":        0,
			"last_error":           "",
			"next_retry_at":        nil,
			"delivery_lease_until": nil,
			"dead_lettered_at":     nil,
			"dead_letter_reason":   "",
		}).Error
	})
}

func (d *DB) MarkWebhookDeliverySuccess(id int) error {
	return d.conn.Model(&model.Webhook{}).Where("id = ?", id).Updates(map[string]any{
		"status":               "success",
		"failure_count":        0,
		"last_error":           "",
		"next_retry_at":        nil,
		"delivery_lease_until": nil,
		"dead_lettered_at":     nil,
		"dead_letter_reason":   "",
	}).Error
}

func (d *DB) MarkWebhookDeliverySkipped(id int) error {
	return d.conn.Model(&model.Webhook{}).Where("id = ?", id).Updates(map[string]any{
		"status":               "skipped",
		"failure_count":        0,
		"last_error":           "",
		"next_retry_at":        nil,
		"delivery_lease_until": nil,
		"dead_lettered_at":     nil,
		"dead_letter_reason":   "",
	}).Error
}

func (d *DB) MarkWebhookDeliveryFailed(id int, reason string, maxFailures int) (string, error) {
	var webhook model.Webhook
	if err := d.conn.First(&webhook, id).Error; err != nil {
		return "failed", err
	}

	nextFailureCount := webhook.FailureCount + 1
	updates := map[string]any{
		"failure_count":        nextFailureCount,
		"delivery_lease_until": nil,
	}

	status := "failed"
	if nextFailureCount >= maxFailures {
		now := time.Now()
		status = "dead_lettered"
		updates["status"] = status
		updates["last_error"] = reason
		updates["next_retry_at"] = nil
		updates["dead_lettered_at"] = &now
		updates["dead_letter_reason"] = reason
	} else {
		updates["status"] = status
		updates["last_error"] = reason
		updates["next_retry_at"] = nil
		updates["dead_lettered_at"] = nil
		updates["dead_letter_reason"] = ""
	}

	if err := d.conn.Model(&model.Webhook{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return "failed", err
	}

	return status, nil
}

func (d *DB) MarkWebhookRetryScheduled(id int, failureCount int, reason string, nextRetryAt time.Time) error {
	return d.conn.Model(&model.Webhook{}).Where("id = ?", id).Updates(map[string]any{
		"status":               "retrying",
		"failure_count":        failureCount,
		"last_error":           reason,
		"next_retry_at":        &nextRetryAt,
		"delivery_lease_until": nil,
		"dead_lettered_at":     nil,
		"dead_letter_reason":   "",
	}).Error
}

func (d *DB) ReleaseWebhookLease(id int) error {
	return d.conn.Model(&model.Webhook{}).
		Where("id = ? AND status = ?", id, "processing").
		Updates(map[string]any{
			"status":               "pending",
			"delivery_lease_until": nil,
		}).Error
}

func (d *DB) DeleteWebhook(id int) error {
	return d.conn.Transaction(func(tx *gorm.DB) error {
		webhook, err := lockWebhook(tx, id)
		if err != nil {
			return err
		}
		if webhook.Status == "processing" {
			return ErrWebhookProcessing
		}

		if err := tx.Where("webhook_id = ?", id).Delete(&model.DeliveryAttempt{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&webhook).Error; err != nil {
			return err
		}
		return nil
	})
}

func lockWebhook(tx *gorm.DB, id int) (model.Webhook, error) {
	var webhook model.Webhook
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&webhook, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Webhook{}, ErrNotFound
	}
	return webhook, err
}

func (d *DB) DeleteForwardingRule(source string) error {
	return d.conn.Where("source = ?", source).Delete(&model.ForwardingRule{}).Error
}
