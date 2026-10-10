package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	"webhookhub/internal/model"
)

func TestStorageHonorsCancelledContext(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	operations := map[string]func() error{
		"save":        func() error { return db.Save(ctx, &model.Webhook{Source: "cancelled"}) },
		"find":        func() error { _, err := db.FindByID(ctx, 1); return err },
		"replay":      func() error { return db.ResetWebhookDeliveryState(ctx, 1) },
		"delete":      func() error { return db.DeleteWebhook(ctx, 1) },
		"save rule":   func() error { return db.SaveForwardingRule(ctx, model.ForwardingRule{Source: "cancelled"}) },
		"list rules":  func() error { _, err := db.GetForwardingRules(ctx); return err },
		"find rule":   func() error { _, err := db.GetForwardingRule(ctx, "cancelled"); return err },
		"delete rule": func() error { return db.DeleteForwardingRule(ctx, "cancelled") },
		"find user":   func() error { _, err := db.FindUserByEmail(ctx, "cancelled@example.com"); return err },
		"filter":      func() error { _, err := db.Filtered(ctx, WebhookFilter{}, 10, 0); return err },
		"count":       func() error { _, err := db.CountFiltered(ctx, WebhookFilter{}); return err },
		"attempts":    func() error { _, err := db.DeliveryAttemptsByWebhook(ctx, 1); return err },
		"attempt":     func() error { _, err := db.DeliveryAttemptByID(ctx, 1, 1); return err },
		"metrics":     func() error { _, err := db.DeliveryMetrics(ctx); return err },
		"retention":   func() error { _, err := db.CleanupExpiredWebhooks(ctx, time.Now(), 1); return err },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			if err := operation(); !errors.Is(err, context.Canceled) {
				t.Fatalf("expected cancelled operation, got %v", err)
			}
		})
	}
}

func TestReplayDeadlineWhileWaitingForLock(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	hook := model.Webhook{Source: "locked", Status: "success", ReceivedAt: time.Now()}
	if err := db.Save(context.Background(), &hook); err != nil {
		t.Fatal(err)
	}
	err := db.conn.Transaction(func(tx *gorm.DB) error {
		if _, err := lockWebhook(tx, int(hook.ID)); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if err := db.ResetWebhookDeliveryState(ctx, int(hook.ID)); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected lock wait deadline, got %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := db.FindByID(context.Background(), int(hook.ID))
	if err != nil || stored.Status != "success" {
		t.Fatalf("cancelled replay changed webhook: %+v, err=%v", stored, err)
	}
}

func TestRetentionWorkerCancelsActiveCleanup(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	started := make(chan struct{}, 1)
	if err := db.conn.Callback().Query().Before("gorm:query").Register("test:retention_started", func(tx *gorm.DB) {
		select {
		case started <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := db.conn.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("LOCK TABLE webhooks IN ACCESS EXCLUSIVE MODE").Error; err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		workers := StartRetentionWorker(ctx, db, 1, time.Hour, 1)
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("cleanup did not start")
		}
		cancel()
		done := make(chan struct{})
		go func() { workers.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("retention did not stop while its query was blocked")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
