package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"webhookhub/internal/model"
)

func TestStreamWebhooksFiltersAndOrders(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	ctx := context.Background()
	now := time.Now()
	hooks := []model.Webhook{
		{Source: "export", Payload: []byte("first"), ReceivedAt: now.Add(-2 * time.Minute)},
		{Source: "other", Payload: []byte("ignored"), ReceivedAt: now.Add(-time.Minute)},
		{Source: "export", Payload: []byte("second"), ReceivedAt: now},
	}
	for i := range hooks {
		hooks[i].Status = "success"
		if err := db.Save(ctx, &hooks[i]); err != nil {
			t.Fatal(err)
		}
	}

	var streamed []model.Webhook
	err := db.StreamWebhooks(ctx, WebhookFilter{Source: "export", Sort: "received_asc"}, func(hook model.Webhook) error {
		streamed = append(streamed, hook)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(streamed) != 2 || streamed[0].ID != hooks[0].ID || streamed[1].ID != hooks[2].ID {
		t.Fatalf("stream lost filter or order: %+v", streamed)
	}
	if string(streamed[0].Payload) != "first" || string(streamed[1].Payload) != "second" {
		t.Fatal("stream must include full payloads")
	}
}

func TestStreamWebhooksStopsOnYieldError(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := db.Save(ctx, &model.Webhook{Source: "export", Status: "success", ReceivedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}

	stop := errors.New("consumer stopped")
	calls := 0
	err := db.StreamWebhooks(ctx, WebhookFilter{}, func(model.Webhook) error {
		calls++
		return stop
	})
	if !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("yield error must stop the stream: err=%v calls=%d", err, calls)
	}
}

func TestStreamWebhooksHonorsContext(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := db.StreamWebhooks(ctx, WebhookFilter{}, func(model.Webhook) error {
		t.Fatal("cancelled stream yielded a row")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
