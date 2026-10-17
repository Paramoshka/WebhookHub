package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"webhookhub/internal/model"
)

func TestForwardingPausePersistsAcrossConnectionsAndRuleEdits(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	ctx := context.Background()
	rule := model.ForwardingRule{Source: "paused", Target: "https://example.com/original"}
	if err := db.SaveForwardingRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	rule, err := db.GetForwardingRule(ctx, rule.Source)
	if err != nil || rule.Paused {
		t.Fatalf("new rule should be active: %+v, %v", rule, err)
	}
	for range 2 {
		if err := db.SetForwardingPaused(ctx, rule.Source, true); err != nil {
			t.Fatal(err)
		}
	}

	// Saving an older edit or adding the same source must not undo a pause.
	rule.Target = "https://example.com/updated"
	if err := db.SaveForwardingRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	reopened := openTestDB(t)
	stored, err := reopened.GetForwardingRule(ctx, rule.Source)
	if err != nil || !stored.Paused || stored.Target != rule.Target {
		t.Fatalf("pause or edit lost after reopening: %+v, %v", stored, err)
	}
	for range 2 {
		if err := reopened.SetForwardingPaused(ctx, rule.Source, false); err != nil {
			t.Fatal(err)
		}
	}
	stored, err = db.GetForwardingRule(ctx, rule.Source)
	if err != nil || stored.Paused {
		t.Fatalf("resume was not persisted: %+v, %v", stored, err)
	}
	if err := db.SetForwardingPaused(ctx, "missing", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing rule: %v", err)
	}
}

func TestMigrateForwardingPauseKeepsExistingRulesActive(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	ctx := context.Background()
	rule := model.ForwardingRule{Source: "legacy", Target: "https://example.com"}
	if err := db.SaveForwardingRule(ctx, rule); err != nil {
		t.Fatal(err)
	}
	if err := db.conn.Migrator().DropColumn(&model.ForwardingRule{}, "paused"); err != nil {
		t.Fatal(err)
	}
	migrated := openTestDB(t)
	stored, err := migrated.GetForwardingRule(ctx, rule.Source)
	if err != nil || stored.Paused || stored.Target != rule.Target {
		t.Fatalf("migration changed an existing rule: %+v, %v", stored, err)
	}
	if err := migrated.SetForwardingPaused(ctx, rule.Source, true); err != nil {
		t.Fatal(err)
	}
}

func TestPausedSourcesKeepTheirQueueUntilResumed(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	ctx := context.Background()
	now := time.Now()
	past, future := now.Add(-time.Minute), now.Add(time.Hour)
	if err := db.SaveForwardingRule(ctx, model.ForwardingRule{Source: "paused", Target: "https://example.com", Paused: true}); err != nil {
		t.Fatal(err)
	}
	hooks := []model.Webhook{
		{Source: "paused", Status: "pending", ReceivedAt: past},
		{Source: "paused", Status: "retrying", ReceivedAt: past, NextRetryAt: &past, FailureCount: 2},
		{Source: "paused", Status: "retrying", ReceivedAt: past, NextRetryAt: &future},
		{Source: "paused", Status: "processing", ReceivedAt: past, DeliveryLeaseUntil: &past},
		{Source: "paused", Status: "processing", ReceivedAt: past, DeliveryLeaseUntil: &future},
		{Source: "paused", Status: "success", ReceivedAt: past},
		{Source: "paused", Status: "dead_lettered", ReceivedAt: past},
		{Source: "other", Status: "pending", ReceivedAt: now},
	}
	for i := range hooks {
		if err := db.Save(ctx, &hooks[i]); err != nil {
			t.Fatal(err)
		}
	}
	counts, err := db.QueuedWebhookCounts(ctx)
	if err != nil || counts["paused"] != 4 || counts["other"] != 1 {
		t.Fatalf("queue counts: %v, %v", counts, err)
	}
	claimed, err := db.ClaimDeliverableWebhooks(ctx, 1, now, future)
	if err != nil || len(claimed) != 1 || claimed[0].ID != hooks[7].ID {
		t.Fatalf("paused source blocked another source: %+v, %v", claimed, err)
	}
	for _, hook := range hooks[:7] {
		stored, err := db.FindByID(ctx, int(hook.ID))
		if err != nil || stored.Status != hook.Status || stored.FailureCount != hook.FailureCount || stored.DeliveryLeaseVersion != 0 {
			t.Fatalf("paused event changed: %+v, %v", stored, err)
		}
		attempts, err := db.DeliveryAttemptsByWebhook(ctx, hook.ID)
		if err != nil || len(attempts) != 0 {
			t.Fatalf("pause created attempts: %+v, %v", attempts, err)
		}
	}
	if err := db.SetForwardingPaused(ctx, "paused", false); err != nil {
		t.Fatal(err)
	}
	claimed, err = db.ClaimDeliverableWebhooks(ctx, 10, now, future)
	if err != nil || len(claimed) != 3 {
		t.Fatalf("resumed queue: %+v, %v", claimed, err)
	}
	want := map[uint]bool{hooks[0].ID: true, hooks[1].ID: true, hooks[3].ID: true}
	for _, hook := range claimed {
		if !want[hook.ID] {
			t.Fatalf("unexpected resumed event: %+v", hook)
		}
		delete(want, hook.ID)
	}
	if len(want) != 0 {
		t.Fatalf("events missing after resume: %v", want)
	}
}
