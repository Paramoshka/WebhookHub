package storage

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"webhookhub/internal/model"
)

func TestBulkReplaySkipsChangedEventsAndPreservesHistory(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	ctx := context.Background()
	when := time.Now().Add(-48 * time.Hour)
	statuses := []string{"dead_lettered", "dead_lettered", "success", "processing", "pending", "retrying", "skipped", "failed"}
	var ids []int
	for _, status := range statuses {
		hook := model.Webhook{Source: "bulk", Status: status, ReceivedAt: when, Payload: []byte{0, 0xff}, Headers: "headers", Response: []byte("response"), FailureCount: 3, LastError: "error", NextRetryAt: &when, DeliveryLeaseUntil: &when, DeadLetteredAt: &when, DeadLetterReason: "reason"}
		if err := db.Save(ctx, &hook); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, int(hook.ID))
	}
	attemptID, err := createAttemptFixture(db, &model.DeliveryAttempt{WebhookID: uint(ids[0]), Source: "bulk", Status: "failed", StartedAt: when, ResponseBody: []byte("old response"), ResponseCaptured: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := db.BulkReplayDeadLetters(ctx, append(ids, ids[0], 999))
	if err != nil || result.Requeued != 2 || result.Skipped != 7 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for i, id := range ids {
		hook, err := db.FindByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			if hook.Status != "pending" || hook.FailureCount != 0 || hook.LastError != "" || hook.NextRetryAt != nil || hook.DeliveryLeaseUntil != nil || hook.DeadLetteredAt != nil || hook.DeadLetterReason != "" || len(hook.Response) != 0 {
				t.Fatalf("incomplete reset: %+v", hook)
			}
		} else if hook.Status != statuses[i] || hook.FailureCount != 3 || string(hook.Response) != "response" {
			t.Fatalf("changed skipped event: %+v", hook)
		}
		if len(hook.Payload) != 2 || hook.Headers != "headers" {
			t.Fatal("request contents changed")
		}
	}
	attempt, err := db.DeliveryAttemptByID(ctx, uint(ids[0]), attemptID)
	if err != nil || string(attempt.ResponseBody) != "old response" || attempt.Status != "failed" {
		t.Fatalf("history lost: %+v %v", attempt, err)
	}

	for _, invalid := range [][]int{nil, {0}, {-1}, {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21}} {
		if _, err := db.BulkReplayDeadLetters(ctx, invalid); err == nil {
			t.Fatalf("accepted invalid IDs: %v", invalid)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := db.BulkReplayDeadLetters(cancelled, []int{ids[0]}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled bulk replay: %v", err)
	}
}

func TestBulkReplayRollsBackOnError(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	first := createRetentionHook(t, db, "dead_lettered", time.Now())
	second := createRetentionHook(t, db, "dead_lettered", time.Now())
	injected := errors.New("injected bulk update failure")
	if err := db.conn.Callback().Update().After("gorm:update").Register("test:bulk_update_failure", func(tx *gorm.DB) { tx.AddError(injected) }); err != nil {
		t.Fatal(err)
	}
	result, err := db.BulkReplayDeadLetters(context.Background(), []int{int(first.ID), int(second.ID)})
	if !errors.Is(err, injected) || result != (BulkReplayResult{}) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, id := range []uint{first.ID, second.ID} {
		hook, err := db.FindByID(context.Background(), int(id))
		if err != nil || hook.Status != "dead_lettered" {
			t.Fatalf("update did not roll back: %+v %v", hook, err)
		}
	}
}

func TestConcurrentBulkReplaysQueueEachEventOnce(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	first := createRetentionHook(t, db, "dead_lettered", time.Now())
	second := createRetentionHook(t, db, "dead_lettered", time.Now())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	results := make(chan BulkReplayResult, 2)
	for _, ids := range [][]int{{int(first.ID), int(second.ID)}, {int(second.ID), int(first.ID)}} {
		wg.Add(1)
		go func(ids []int) {
			defer wg.Done()
			result, err := db.BulkReplayDeadLetters(ctx, ids)
			if err != nil {
				t.Error(err)
			}
			results <- result
		}(ids)
	}
	wg.Wait()
	close(results)
	requeued, skipped := 0, 0
	for result := range results {
		requeued += result.Requeued
		skipped += result.Skipped
	}
	if requeued != 2 || skipped != 2 {
		t.Fatalf("requeued=%d skipped=%d", requeued, skipped)
	}
	now := time.Now()
	claimed, err := db.ClaimDeliverableWebhooks(ctx, 2, now, now.Add(time.Minute))
	if err != nil || len(claimed) != 2 {
		t.Fatalf("events were not queued: %+v %v", claimed, err)
	}
}

func TestBulkReplayRechecksConcurrentChanges(t *testing.T) {
	for _, operation := range []string{"replay", "retention", "claim"} {
		t.Run(operation, func(t *testing.T) {
			db := openTestDB(t)
			truncateTestTables(t, db)
			hook := createRetentionHook(t, db, "dead_lettered", time.Now().Add(-48*time.Hour))
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			started := make(chan struct{}, 1)
			resultCh := make(chan BulkReplayResult, 1)
			errCh := make(chan error, 1)
			err := db.conn.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if _, err := lockWebhook(tx, int(hook.ID)); err != nil {
					return err
				}
				if err := db.conn.Callback().Query().Before("gorm:query").Register("test:bulk_started", func(tx *gorm.DB) {
					select {
					case started <- struct{}{}:
					default:
					}
				}); err != nil {
					return err
				}
				go func() {
					result, err := db.BulkReplayDeadLetters(ctx, []int{int(hook.ID)})
					resultCh <- result
					errCh <- err
				}()
				select {
				case <-started:
				case <-ctx.Done():
					return ctx.Err()
				}
				locked := &DB{conn: tx}
				if operation == "retention" {
					_, err := locked.CleanupExpiredWebhooks(ctx, time.Now(), 20)
					return err
				}
				if err := locked.ResetWebhookDeliveryState(ctx, int(hook.ID)); err != nil {
					return err
				}
				if operation == "claim" {
					now := time.Now()
					_, err := locked.ClaimDeliverableWebhooks(ctx, 1, now, now.Add(time.Minute))
					return err
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := <-errCh; err != nil {
				t.Fatal(err)
			}
			result := <-resultCh
			if result.Requeued != 0 || result.Skipped != 1 {
				t.Fatalf("concurrent %s ignored: %+v", operation, result)
			}
		})
	}
}
