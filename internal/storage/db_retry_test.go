package storage

import (
	"errors"
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

func TestWebhookMutationsRejectProcessingWebhook(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)

	leaseUntil := time.Now().Add(time.Minute)
	webhook := model.Webhook{
		Source:             "processing",
		Status:             "processing",
		ReceivedAt:         time.Now(),
		DeliveryLeaseUntil: &leaseUntil,
	}
	if err := db.Save(&webhook); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDeliveryAttempt(&model.DeliveryAttempt{
		WebhookID: webhook.ID,
		Source:    webhook.Source,
		Status:    "pending",
		StartedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	if err := db.ResetWebhookDeliveryState(int(webhook.ID)); !errors.Is(err, ErrWebhookProcessing) {
		t.Fatalf("expected reset conflict, got %v", err)
	}
	if err := db.DeleteWebhook(int(webhook.ID)); !errors.Is(err, ErrWebhookProcessing) {
		t.Fatalf("expected delete conflict, got %v", err)
	}

	var stored model.Webhook
	if err := db.conn.First(&stored, webhook.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "processing" || stored.DeliveryLeaseUntil == nil {
		t.Fatalf("processing webhook was changed: %+v", stored)
	}
	attempts, err := db.DeliveryAttemptsByWebhook(webhook.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatalf("expected delivery attempt to remain, got %d", len(attempts))
	}
}

func TestWebhookMutationsAllowInactiveWebhook(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)

	nextRetryAt := time.Now().Add(time.Minute)
	webhook := model.Webhook{
		Source:       "retrying",
		Status:       "retrying",
		ReceivedAt:   time.Now(),
		FailureCount: 2,
		LastError:    "temporary failure",
		NextRetryAt:  &nextRetryAt,
	}
	if err := db.Save(&webhook); err != nil {
		t.Fatal(err)
	}

	if err := db.ResetWebhookDeliveryState(int(webhook.ID)); err != nil {
		t.Fatal(err)
	}
	var reset model.Webhook
	if err := db.conn.First(&reset, webhook.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reset.Status != "pending" || reset.FailureCount != 0 || reset.NextRetryAt != nil {
		t.Fatalf("unexpected reset state: %+v", reset)
	}

	if err := db.DeleteWebhook(int(webhook.ID)); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteWebhook(int(webhook.ID)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected deleted webhook to be missing, got %v", err)
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
