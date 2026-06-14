package storage

import (
	"log"
	"os"
	"time"

	"webhookhub/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type DB struct {
	conn *gorm.DB
}

func InitDB() *DB {
	host := os.Getenv("POSTGRES_HOST")
	user := os.Getenv("POSTGRES_USER")
	pass := os.Getenv("POSTGRES_PASSWORD")
	dbname := os.Getenv("POSTGRES_DB")
	port := os.Getenv("POSTGRES_PORT")

	dsn := "host=" + host + " user=" + user + " password=" + pass + " dbname=" + dbname + " port=" + port + " sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}

	// Auto migrate
	err = db.AutoMigrate(&model.Webhook{}, &model.DeliveryAttempt{}, &model.ForwardingRule{}, &model.User{})
	if err != nil {
		log.Fatalf("auto-migrate failed: %v", err)
	}

	// Create admin if not exists
	var count int64
	db.Model(&model.User{}).Count(&count)

	if count == 0 {
		adminEmail := os.Getenv("ADMIN_EMAIL")
		if adminEmail == "" {
			adminEmail = "admin@example.com"
		}

		adminPass := os.Getenv("ADMIN_PASSWORD")
		if adminPass == "" {
			adminPass = "admin" // default, for dev only!
		}

		hashed, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
		if err != nil {
			log.Fatal("failed to hash admin password:", err)
		}

		admin := model.User{
			Email:    adminEmail,
			Password: string(hashed),
			IsAdmin:  true,
		}

		if err := db.Create(&admin).Error; err != nil {
			log.Fatal("failed to create admin user:", err)
		}

		log.Println("✅ Admin user created:", adminEmail)
	}

	return &DB{conn: db}
}

// Save webhook
func (d *DB) Save(h *model.Webhook) {
	if err := d.conn.Create(&h).Error; err != nil {
		log.Println("DB Save Error:", err)
	}
}

// Get all webhooks
func (d *DB) All() []model.Webhook {
	var list []model.Webhook
	d.conn.Order("id desc").Find(&list)
	return list
}

// Find webhook by ID
func (d *DB) FindByID(id string) (model.Webhook, bool) {
	var h model.Webhook
	result := d.conn.First(&h, id)
	return h, result.Error == nil
}

func (d *DB) UpdateResponseFromForward(id int, resp []byte) {
	d.conn.Model(&model.Webhook{}).Where("id = ?", id).Update("response", resp)
}

func (d *DB) ResetWebhookDeliveryState(id int) {
	if err := d.conn.Model(&model.Webhook{}).Where("id = ?", id).Updates(map[string]any{
		"status":             "pending",
		"response":           []byte(nil),
		"failure_count":      0,
		"dead_lettered_at":   nil,
		"dead_letter_reason": "",
	}).Error; err != nil {
		log.Println("DB ResetWebhookDeliveryState Error:", err)
	}
}

func (d *DB) MarkWebhookDeliverySuccess(id int) {
	if err := d.conn.Model(&model.Webhook{}).Where("id = ?", id).Updates(map[string]any{
		"status":             "success",
		"failure_count":      0,
		"dead_lettered_at":   nil,
		"dead_letter_reason": "",
	}).Error; err != nil {
		log.Println("DB MarkWebhookDeliverySuccess Error:", err)
	}
}

func (d *DB) MarkWebhookDeliverySkipped(id int) {
	if err := d.conn.Model(&model.Webhook{}).Where("id = ?", id).Updates(map[string]any{
		"status":             "skipped",
		"failure_count":      0,
		"dead_lettered_at":   nil,
		"dead_letter_reason": "",
	}).Error; err != nil {
		log.Println("DB MarkWebhookDeliverySkipped Error:", err)
	}
}

func (d *DB) MarkWebhookDeliveryFailed(id int, reason string, maxFailures int) string {
	var webhook model.Webhook
	if err := d.conn.First(&webhook, id).Error; err != nil {
		log.Println("DB MarkWebhookDeliveryFailed Load Error:", err)
		return "failed"
	}

	nextFailureCount := webhook.FailureCount + 1
	updates := map[string]any{
		"failure_count": nextFailureCount,
	}

	status := "failed"
	if nextFailureCount >= maxFailures {
		now := time.Now()
		status = "dead_lettered"
		updates["status"] = status
		updates["dead_lettered_at"] = &now
		updates["dead_letter_reason"] = reason
	} else {
		updates["status"] = status
		updates["dead_lettered_at"] = nil
		updates["dead_letter_reason"] = ""
	}

	if err := d.conn.Model(&model.Webhook{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		log.Println("DB MarkWebhookDeliveryFailed Update Error:", err)
		return "failed"
	}

	return status
}

// Delete webhook
func (d *DB) DeleteWebhook(id int) {
	if err := d.conn.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("webhook_id = ?", id).Delete(&model.DeliveryAttempt{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&model.Webhook{}, id).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		log.Println("DB DeleteWebhook Error:", err)
	}
}

// Delete forwarding rule
func (d *DB) DeleteForwardingRule(source string) {
	d.conn.Where("source = ?", source).Delete(&model.ForwardingRule{})
}
