package storage

import (
	"os"
	"sort"
	"testing"
	"time"

	"webhookhub/internal/model"

	"golang.org/x/crypto/bcrypt"
)

func TestClaimDeliverableWebhooks(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)

	now := time.Now().UTC().Truncate(time.Second)
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)
	webhooks := []model.Webhook{
		{Source: "pending", Status: "pending", ReceivedAt: now.Add(-5 * time.Minute)},
		{Source: "due-retry", Status: "retrying", ReceivedAt: now, NextRetryAt: &past},
		{Source: "future-retry", Status: "retrying", ReceivedAt: now, NextRetryAt: &future},
		{Source: "expired-lease", Status: "processing", ReceivedAt: now, DeliveryLeaseUntil: &past},
		{Source: "active-lease", Status: "processing", ReceivedAt: now, DeliveryLeaseUntil: &future},
		{Source: "complete", Status: "success", ReceivedAt: now},
	}
	for i := range webhooks {
		if err := db.Save(&webhooks[i]); err != nil {
			t.Fatal(err)
		}
	}
	attemptID, err := db.CreateDeliveryAttempt(&model.DeliveryAttempt{
		WebhookID: webhooks[3].ID,
		Source:    webhooks[3].Source,
		Status:    "pending",
		StartedAt: past,
	})
	if err != nil {
		t.Fatal(err)
	}

	leaseUntil := now.Add(30 * time.Second)
	claimed, err := db.ClaimDeliverableWebhooks(10, now, leaseUntil)
	if err != nil {
		t.Fatal(err)
	}

	sources := make([]string, 0, len(claimed))
	for _, webhook := range claimed {
		sources = append(sources, webhook.Source)
		if webhook.Status != "processing" {
			t.Errorf("webhook %s: expected processing status, got %s", webhook.Source, webhook.Status)
		}
		if webhook.DeliveryLeaseUntil == nil || !webhook.DeliveryLeaseUntil.Equal(leaseUntil) {
			t.Errorf("webhook %s: expected lease %s, got %v", webhook.Source, leaseUntil, webhook.DeliveryLeaseUntil)
		}
	}
	sort.Strings(sources)
	want := []string{"due-retry", "expired-lease", "pending"}
	if len(sources) != len(want) {
		t.Fatalf("expected %v, got %v", want, sources)
	}
	for i := range want {
		if sources[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, sources)
		}
	}

	var interrupted model.DeliveryAttempt
	if err := db.conn.First(&interrupted, attemptID).Error; err != nil {
		t.Fatal(err)
	}
	if interrupted.Status != "interrupted" || interrupted.CompletedAt == nil {
		t.Fatalf("expected stale attempt to be interrupted, got %+v", interrupted)
	}
}

func TestEnsureAdminUpdatesBootstrapPassword(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)

	if err := db.EnsureAdmin("admin@example.com", "first-password"); err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureAdmin("new-admin@example.com", "rotated-password"); err != nil {
		t.Fatal(err)
	}

	user, err := db.FindUserByEmail("new-admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte("rotated-password")); err != nil {
		t.Fatal("configured admin password was not applied")
	}
}

func openTestDB(t *testing.T) *DB {
	t.Helper()
	required := []string{
		"TEST_POSTGRES_HOST",
		"TEST_POSTGRES_PORT",
		"TEST_POSTGRES_USER",
		"TEST_POSTGRES_PASSWORD",
		"TEST_POSTGRES_DB",
	}
	for _, name := range required {
		if os.Getenv(name) == "" {
			t.Skip("PostgreSQL integration test environment is not configured")
		}
	}

	db, err := Open(Config{
		Host:     os.Getenv("TEST_POSTGRES_HOST"),
		Port:     os.Getenv("TEST_POSTGRES_PORT"),
		User:     os.Getenv("TEST_POSTGRES_USER"),
		Password: os.Getenv("TEST_POSTGRES_PASSWORD"),
		Name:     os.Getenv("TEST_POSTGRES_DB"),
		SSLMode:  "disable",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	return db
}

func truncateTestTables(t *testing.T, db *DB) {
	t.Helper()
	if err := db.conn.Exec("TRUNCATE TABLE delivery_attempts, webhooks, forwarding_rules, users RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatal(err)
	}
}
