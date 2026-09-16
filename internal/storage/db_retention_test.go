package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"webhookhub/internal/model"

	"gorm.io/gorm"
)

func TestCleanupExpiredWebhooks(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	now := time.Now()
	for _, status := range []string{"success", "failed", "skipped", "dead_lettered", "pending", "processing", "retrying"} {
		createRetentionHook(t, db, status, now.Add(-48*time.Hour))
	}
	createRetentionHook(t, db, "success", now)
	deleted, err := db.CleanupExpiredWebhooks(now.Add(-24*time.Hour), 2)
	if err != nil || deleted != 4 {
		t.Fatalf("expected 4 deletions across batches, got %d, err=%v", deleted, err)
	}
	var remaining, attempts int64
	if err := db.conn.Model(&model.Webhook{}).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.conn.Model(&model.DeliveryAttempt{}).Count(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 4 || attempts != 4 {
		t.Fatalf("expected 4 hooks and attempts, got %d and %d", remaining, attempts)
	}
}

func TestCleanupRollsBackAttemptsOnDeleteFailure(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	hook := createRetentionHook(t, db, "success", time.Now().Add(-48*time.Hour))
	injected := errors.New("injected webhook delete failure")
	if err := db.conn.Callback().Delete().Before("gorm:delete").Register("test:fail_webhook_delete", func(tx *gorm.DB) {
		if tx.Statement.Table == "webhooks" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	deleted, err := db.CleanupExpiredWebhooks(time.Now(), 2)
	if !errors.Is(err, injected) || deleted != 0 {
		t.Fatalf("expected rollback, got deleted=%d err=%v", deleted, err)
	}
	if _, err := db.FindByID(int(hook.ID)); err != nil {
		t.Fatal(err)
	}
	attempts, err := db.DeliveryAttemptsByWebhook(hook.ID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempt was not restored: count=%d err=%v", len(attempts), err)
	}
}

func TestCleanupSkipsReplayLockAndQueuedWebhook(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db = &DB{conn: db.conn.WithContext(ctx)}
	truncateTestTables(t, db)
	hook := createRetentionHook(t, db, "success", time.Now().Add(-48*time.Hour))
	// Hold the same row lock Replay takes while cleanup runs on another connection.
	err := db.conn.Transaction(func(tx *gorm.DB) error {
		if _, err := lockWebhook(tx, int(hook.ID)); err != nil {
			return err
		}
		deleted, err := db.CleanupExpiredWebhooks(time.Now(), 2)
		if err != nil || deleted != 0 {
			t.Errorf("locked webhook deleted: count=%d err=%v", deleted, err)
		}
		return (&DB{conn: tx}).ResetWebhookDeliveryState(int(hook.ID))
	})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := db.CleanupExpiredWebhooks(time.Now(), 2)
	if err != nil || deleted != 0 {
		t.Fatalf("queued webhook deleted: count=%d err=%v", deleted, err)
	}
	now := time.Now()
	hooks, err := db.ClaimDeliverableWebhooks(1, now, now.Add(time.Minute))
	if err != nil || len(hooks) != 1 || hooks[0].ID != hook.ID {
		t.Fatalf("replayed webhook cannot be claimed: hooks=%v err=%v", hooks, err)
	}
}

func TestCleanupKeepsLockUntilDeletion(t *testing.T) {
	db := openTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db = &DB{conn: db.conn.WithContext(ctx)}
	truncateTestTables(t, db)
	hook := createRetentionHook(t, db, "success", time.Now().Add(-48*time.Hour))
	checked := false
	if err := db.conn.Callback().Delete().Before("gorm:delete").Register("test:check_retention_lock", func(tx *gorm.DB) {
		if tx.Statement.Table != "delivery_attempts" {
			return
		}
		checked = true
		// NOWAIT verifies the cleanup transaction already owns the webhook row.
		err := db.conn.Transaction(func(other *gorm.DB) error {
			return other.Raw("SELECT id FROM webhooks WHERE id = ? FOR UPDATE NOWAIT", hook.ID).Scan(new(uint)).Error
		})
		var sqlError interface{ SQLState() string }
		if !errors.As(err, &sqlError) || sqlError.SQLState() != "55P03" {
			t.Errorf("expected row lock conflict, got %v", err)
		}
		now := time.Now()
		hooks, err := db.ClaimDeliverableWebhooks(1, now, now.Add(time.Minute))
		if err != nil || len(hooks) != 0 {
			t.Errorf("worker claimed cleanup candidate: hooks=%v err=%v", hooks, err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	deleted, err := db.CleanupExpiredWebhooks(time.Now(), 2)
	if err != nil || deleted != 1 || !checked {
		t.Fatalf("expected checked deletion, got count=%d checked=%v err=%v", deleted, checked, err)
	}
	if err := db.ResetWebhookDeliveryState(int(hook.ID)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected replay after deletion to fail with not found, got %v", err)
	}
}

func createRetentionHook(t *testing.T, db *DB, status string, receivedAt time.Time) model.Webhook {
	t.Helper()
	hook := model.Webhook{Source: "retention", Status: status, ReceivedAt: receivedAt}
	if err := db.Save(&hook); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDeliveryAttempt(&model.DeliveryAttempt{
		WebhookID: hook.ID, Source: hook.Source, Status: "success", StartedAt: receivedAt,
		ResponseBody: []byte("saved response"), ResponseHeaders: `{"Content-Type":["text/plain"]}`, ResponseCaptured: true,
	}); err != nil {
		t.Fatal(err)
	}
	return hook
}
