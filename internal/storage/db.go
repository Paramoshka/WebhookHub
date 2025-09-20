package storage

import (
	"log"
	"os"

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
	err = db.AutoMigrate(&model.Webhook{}, &model.ForwardingRule{}, &model.User{})
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

// Update webhook status
func (d *DB) UpdateStatus(id int, status string) {
	d.conn.Model(&model.Webhook{}).Where("id = ?", id).Update("status", status)
}

// Delete webhook
func (d *DB) DeleteWebhook(id int) {
	d.conn.Delete(&model.Webhook{}, id)
}

// Delete forwarding rule
func (d *DB) DeleteForwardingRule(source string) {
	d.conn.Where("source = ?", source).Delete(&model.ForwardingRule{})
}
