package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"webhookhub/internal/model"
)

func newDeliveryClaim(t *testing.T, db *DB) model.Webhook {
	t.Helper()
	if err := db.Save(&model.Webhook{Source: "delivery", Status: "pending", ReceivedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	return claimTestWebhook(t, db)
}

func TestDeliveryRejectsPreviousLeaseOwner(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	old := newDeliveryClaim(t, db)
	attemptID, err := db.CreateDeliveryAttempt(context.Background(), &old, "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate expiration in storage while the old worker still holds its claim.
	if err := db.conn.Model(&model.Webhook{}).Where("id = ?", old.ID).Update("delivery_lease_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	current := claimTestWebhook(t, db)
	if current.DeliveryLeaseVersion != old.DeliveryLeaseVersion+1 {
		t.Fatal("reclaim did not advance ownership")
	}
	if _, err := db.CreateDeliveryAttempt(context.Background(), &old, ""); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale create: %v", err)
	}
	for _, status := range []string{"success", "failed", "cancelled", "skipped"} {
		err := db.FinishDelivery(context.Background(), &old, attemptID, DeliveryResult{Status: status, Response: model.DeliveryResponse{Body: []byte("stale")}})
		if !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("stale %s: %v", status, err)
		}
	}
	interrupted, err := db.DeliveryAttemptByID(old.ID, attemptID)
	if err != nil || interrupted.Status != "interrupted" || len(interrupted.ResponseBody) != 0 {
		t.Fatalf("stale worker changed interrupted attempt: %+v %v", interrupted, err)
	}
	freshID, err := db.CreateDeliveryAttempt(context.Background(), &current, "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDelivery(context.Background(), &current, freshID, DeliveryResult{Status: "success", Response: model.DeliveryResponse{Body: []byte("fresh"), Captured: true}}); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDelivery(context.Background(), &old, attemptID, DeliveryResult{Status: "cancelled"}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("old cancellation: %v", err)
	}
	stored, err := db.FindByID(int(old.ID))
	if err != nil || stored.Status != "success" || string(stored.Response) != "fresh" || stored.DeliveryLeaseUntil != nil {
		t.Fatalf("new owner result corrupted: %+v %v", stored, err)
	}
	if err := db.ResetWebhookDeliveryState(int(old.ID)); err != nil {
		t.Fatal(err)
	}
	replay := claimTestWebhook(t, db)
	if replay.DeliveryLeaseVersion != current.DeliveryLeaseVersion+1 {
		t.Fatal("replay reset ownership version")
	}
}

func TestDeliveryResultsAreAtomic(t *testing.T) {
	db := openTestDB(t)
	for _, tt := range []struct {
		status, want string
		retry        bool
		failures     int
	}{
		{"success", "success", false, 0}, {"skipped", "skipped", false, 0},
		{"cancelled", "pending", false, 0}, {"failed", "retrying", true, 1}, {"failed", "dead_lettered", false, 1},
	} {
		t.Run(tt.want, func(t *testing.T) {
			truncateTestTables(t, db)
			claim := newDeliveryClaim(t, db)
			id, err := db.CreateDeliveryAttempt(context.Background(), &claim, "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.CreateDeliveryAttempt(context.Background(), &claim, ""); err == nil {
				t.Fatal("duplicate pending attempt accepted")
			}
			result := DeliveryResult{Status: tt.status, ErrorMessage: "test", Response: model.DeliveryResponse{Body: []byte("result"), Captured: true}}
			if tt.retry {
				next := time.Now().Add(time.Minute)
				result.NextRetryAt = &next
			}
			if err := db.FinishDelivery(context.Background(), &claim, id, result); err != nil {
				t.Fatal(err)
			}
			stored, err := db.FindByID(int(claim.ID))
			if err != nil || stored.Status != tt.want || stored.FailureCount != tt.failures || stored.DeliveryLeaseUntil != nil {
				t.Fatalf("wrong webhook: %+v %v", stored, err)
			}
			attempt, err := db.DeliveryAttemptByID(claim.ID, id)
			if err != nil || attempt.Status != tt.status || attempt.CompletedAt == nil {
				t.Fatalf("wrong attempt: %+v %v", attempt, err)
			}
			if (stored.NextRetryAt != nil) != tt.retry || (stored.DeadLetteredAt != nil) != (tt.want == "dead_lettered") {
				t.Fatal("wrong retry/dead-letter state")
			}
			if err := db.FinishDelivery(context.Background(), &claim, id, result); !errors.Is(err, ErrLeaseLost) {
				t.Fatalf("duplicate finish: %v", err)
			}
		})
	}
}

func TestDeliveryRejectsExpiredAndCancelledOperations(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	claim := newDeliveryClaim(t, db)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := db.CreateDeliveryAttempt(cancelled, &claim, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled create: %v", err)
	}
	id, err := db.CreateDeliveryAttempt(context.Background(), &claim, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDelivery(cancelled, &claim, id, DeliveryResult{Status: "success"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled finish: %v", err)
	}
	past := time.Now().Add(-time.Second)
	if err := db.conn.Model(&model.Webhook{}).Where("id = ?", claim.ID).Update("delivery_lease_until", past).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDelivery(context.Background(), &claim, id, DeliveryResult{Status: "success"}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expired finish: %v", err)
	}
	if _, err := db.CreateDeliveryAttempt(context.Background(), &claim, ""); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expired create: %v", err)
	}
}

func TestConcurrentWorkersClaimOnce(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	if err := db.Save(&model.Webhook{Source: "concurrent", Status: "pending", ReceivedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan []model.Webhook, 2)
	errs := make(chan error, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			now := time.Now()
			hooks, err := db.ClaimDeliverableWebhooks(context.Background(), 1, now, now.Add(time.Minute))
			results <- hooks
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for hooks := range results {
		count += len(hooks)
	}
	if count != 1 {
		t.Fatalf("workers claimed webhook %d times", count)
	}
}

func TestMigrateLegacyDeliveryLease(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	rollback := errors.New("rollback migration fixture")
	err := db.conn.Transaction(func(tx *gorm.DB) error {
		if err := tx.Migrator().DropColumn(&model.Webhook{}, "delivery_lease_version"); err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO webhooks (source,status) VALUES ('legacy','pending')").Error; err != nil {
			return err
		}
		if err := tx.AutoMigrate(&model.Webhook{}); err != nil {
			return err
		}
		var hook model.Webhook
		if err := tx.First(&hook).Error; err != nil {
			return err
		}
		if hook.DeliveryLeaseVersion != 0 || hook.Status != "pending" {
			return errors.New("legacy row changed")
		}
		data, err := json.Marshal(hook)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "LeaseVersion") {
			return errors.New("internal ownership leaked into JSON")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}

func TestDeliveryFinalizationDeadlineWhileWaitingForLock(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	claim := newDeliveryClaim(t, db)
	id, err := db.CreateDeliveryAttempt(context.Background(), &claim, "")
	if err != nil {
		t.Fatal(err)
	}
	tx := db.conn.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if _, err := lockWebhook(tx, int(claim.ID)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = db.FinishDelivery(ctx, &claim, id, DeliveryResult{Status: "success"})
	if err == nil || ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("finalization failed to honor deadline: %v", err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	attempt, err := db.DeliveryAttemptByID(claim.ID, id)
	if err != nil || attempt.Status != "pending" {
		t.Fatalf("timed-out transaction changed attempt: %+v %v", attempt, err)
	}
	hook, err := db.FindByID(int(claim.ID))
	if err != nil || hook.Status != "processing" {
		t.Fatalf("timed-out transaction changed webhook: %+v %v", hook, err)
	}
}
