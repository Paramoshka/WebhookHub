package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	"webhookhub/internal/model"
)

func TestMigrateLegacyAttemptResponses(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	// Roll back the simulated old schema even if migration or assertions fail.
	rollback := errors.New("rollback migration test")
	err := db.conn.Transaction(func(tx *gorm.DB) error {
		for _, column := range []string{"response_body", "response_headers", "response_captured", "response_truncated"} {
			if err := tx.Migrator().DropColumn(&model.DeliveryAttempt{}, column); err != nil {
				return err
			}
		}
		if err := tx.Exec("INSERT INTO delivery_attempts (webhook_id, status) VALUES (42, 'success')").Error; err != nil {
			return err
		}
		if err := tx.AutoMigrate(&model.DeliveryAttempt{}); err != nil {
			return err
		}
		var attempt model.DeliveryAttempt
		if err := tx.First(&attempt).Error; err != nil {
			return err
		}
		if attempt.Status != "success" || attempt.ResponseCaptured || attempt.ResponseTruncated || len(attempt.ResponseBody) != 0 {
			return errors.New("legacy attempt changed during migration")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("migration: %v", err)
	}
}

func TestAttemptResponsesSurviveReplay(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	hook := model.Webhook{Source: "history", Status: "pending", ReceivedAt: time.Now()}
	if err := db.Save(&hook); err != nil {
		t.Fatal(err)
	}
	for _, code := range []int{500, 200} {
		if err := db.ResetWebhookDeliveryState(int(hook.ID)); err != nil {
			t.Fatal(err)
		}
		hook = claimTestWebhook(t, db)
		id, err := db.CreateDeliveryAttempt(context.Background(), &hook, "")
		if err != nil {
			t.Fatal(err)
		}
		response := model.DeliveryResponse{HTTPStatus: code, Body: []byte{byte(code)}, Headers: `{"X-Test":["saved"]}`, Captured: true, Truncated: code == 500}
		if err := db.FinishDelivery(context.Background(), &hook, id, DeliveryResult{Status: "success", Response: response, DurationMS: 12}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.ResetWebhookDeliveryState(int(hook.ID)); err != nil {
		t.Fatal(err)
	}
	current, err := db.FindByID(int(hook.ID))
	if err != nil || len(current.Response) != 0 {
		t.Fatalf("replay: %+v %v", current, err)
	}
	attempts, err := db.DeliveryAttemptsByWebhook(hook.ID)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("history: %+v %v", attempts, err)
	}
	for _, summary := range attempts {
		if len(summary.ResponseBody) != 0 || summary.ResponseHeaders != "" {
			t.Fatal("history list loads response contents")
		}
		attempt, err := db.DeliveryAttemptByID(hook.ID, summary.ID)
		if err != nil || !attempt.ResponseCaptured || len(attempt.ResponseBody) != 1 || attempt.ResponseHeaders == "" || attempt.ResponseTruncated != (attempt.HTTPStatus == 500) {
			t.Fatalf("response missing: %+v %v", attempt, err)
		}
	}
	if _, err := db.DeliveryAttemptByID(hook.ID+1, attempts[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-webhook lookup: %v", err)
	}
	if err := db.DeleteWebhook(int(hook.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DeliveryAttemptByID(hook.ID, attempts[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("orphaned response: %v", err)
	}
}

func TestFinishAttemptRollsBackOnWebhookUpdateFailure(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	hook := model.Webhook{Source: "rollback", Status: "pending", ReceivedAt: time.Now()}
	if err := db.Save(&hook); err != nil {
		t.Fatal(err)
	}
	hook = claimTestWebhook(t, db)
	id, err := db.CreateDeliveryAttempt(context.Background(), &hook, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.conn.Exec(`ALTER TABLE webhooks ADD CONSTRAINT test_response_failure CHECK (response IS NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.conn.Exec(`ALTER TABLE webhooks DROP CONSTRAINT test_response_failure`).Error; err != nil {
			t.Error(err)
		}
	})
	if err := db.FinishDelivery(context.Background(), &hook, id, DeliveryResult{Status: "success", Response: model.DeliveryResponse{HTTPStatus: 200, Body: []byte("ok"), Captured: true}, DurationMS: 1}); err == nil {
		t.Fatal("expected update failure")
	}
	attempt, err := db.DeliveryAttemptByID(hook.ID, id)
	if err != nil || attempt.Status != "pending" || attempt.ResponseCaptured {
		t.Fatalf("attempt update did not roll back: %+v %v", attempt, err)
	}
	stored, err := db.FindByID(int(hook.ID))
	if err != nil || stored.Status != "processing" || stored.DeliveryLeaseUntil == nil || stored.DeliveryLeaseVersion != hook.DeliveryLeaseVersion {
		t.Fatalf("webhook update did not roll back: %+v %v", stored, err)
	}
}
