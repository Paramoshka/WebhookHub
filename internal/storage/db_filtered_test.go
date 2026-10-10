package storage

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"webhookhub/internal/model"
)

func TestPayloadSearch(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	hooks := []model.Webhook{
		{Source: "json", Payload: []byte(`{"event":"Payment", "message":"Привет мир"}`)},
		{Source: "empty", Payload: []byte{}},
		{Source: "binary", Payload: []byte{0xff, 0xfe}},
		{Source: "nul", Payload: []byte("payment\x00")},
		{Source: "payment-source", Payload: []byte{0xff}},
	}
	for i := range hooks {
		hooks[i].Status = "success"
		hooks[i].ReceivedAt = time.Now()
		if err := db.Save(context.Background(), &hooks[i]); err != nil {
			t.Fatal(err)
		}
	}
	// Existing records must be searchable after reinitializing the function.
	if err := initPayloadSearch(db.conn); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		query string
		want  int
	}{
		{query: "payment", want: 2},
		{query: "PAYMENT", want: 2},
		{query: "Привет", want: 1},
		{query: "ПРИВЕТ", want: 1},
		{query: "missing", want: 0},
	} {
		t.Run(test.query, func(t *testing.T) {
			filter := WebhookFilter{Query: test.query}
			list, err := db.Filtered(context.Background(), filter, 10, 0)
			if err != nil {
				t.Fatal(err)
			}
			count, err := db.CountFiltered(context.Background(), filter)
			if err != nil {
				t.Fatal(err)
			}
			if len(list) != test.want || count != test.want {
				t.Fatalf("want %d matches, got list=%d count=%d", test.want, len(list), count)
			}
		})
	}
	for _, hook := range hooks {
		stored, err := db.FindByID(context.Background(), int(hook.ID))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(stored.Payload, hook.Payload) {
			t.Fatalf("payload changed for %s", hook.Source)
		}
	}
}

func TestPayloadSearchEscapesWildcards(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	hooks := []model.Webhook{
		{Source: "percent", Payload: []byte(`{"discount":"50%"}`)},
		{Source: "underscore", Payload: []byte(`field_name=value`)},
		{Source: "backslash", Payload: []byte(`C:\temp\file`)},
		{Source: "plain", Payload: []byte(`50 percent off`)},
	}
	for i := range hooks {
		hooks[i].Status = "success"
		hooks[i].ReceivedAt = time.Now()
		if err := db.Save(context.Background(), &hooks[i]); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		query string
		want  int
	}{
		{query: `50%`, want: 1},
		{query: `%`, want: 1},
		{query: `_`, want: 1},
		{query: `\`, want: 1},
		{query: `field_name`, want: 1},
	} {
		t.Run(test.query, func(t *testing.T) {
			filter := WebhookFilter{Query: test.query}
			list, err := db.Filtered(context.Background(), filter, 10, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(list) != test.want {
				t.Fatalf("want %d matches for %q, got %d", test.want, test.query, len(list))
			}
		})
	}
}

func TestFindByID(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	hook := model.Webhook{Source: "inspect", Status: "pending", ReceivedAt: time.Now()}
	if err := db.Save(context.Background(), &hook); err != nil {
		t.Fatal(err)
	}
	stored, err := db.FindByID(context.Background(), int(hook.ID))
	if err != nil || stored.ID != hook.ID {
		t.Fatalf("expected webhook %d, got %+v, err=%v", hook.ID, stored, err)
	}
	for _, id := range []int{0, -1, int(hook.ID) + 1} {
		if _, err := db.FindByID(context.Background(), id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("id %d: expected not found, got %v", id, err)
		}
	}
}

func TestFilterDateBoundaries(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	from := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 1)
	for _, timestamp := range []time.Time{from.Add(-time.Microsecond), from, to.Add(-time.Microsecond), to} {
		if err := db.Save(context.Background(), &model.Webhook{Source: "utc", Status: "success", ReceivedAt: timestamp}); err != nil {
			t.Fatal(err)
		}
	}
	filter := WebhookFilter{Source: "utc", From: &from, To: &to, Sort: "received_asc"}
	hooks, err := db.Filtered(context.Background(), filter, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	count, err := db.CountFiltered(context.Background(), filter)
	if err != nil || count != 2 || len(hooks) != 2 {
		t.Fatalf("range [from,to): count=%d rows=%d err=%v", count, len(hooks), err)
	}
	if !hooks[0].ReceivedAt.Equal(from) || !hooks[1].ReceivedAt.Equal(to.Add(-time.Microsecond)) {
		t.Fatal("incorrect date boundaries")
	}
}

func TestFilteredSummaryPreservesSearchAndPagination(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	ctx := context.Background()
	now := time.Now()
	hooks := []model.Webhook{
		{Source: "summary", Payload: []byte("needle in payload"), Headers: "headers"},
		{Source: "summary", Payload: []byte("payload"), Headers: "needle in headers"},
		{Source: "summary", Payload: []byte("payload"), Headers: "headers", LastError: "needle in error"},
	}
	for i := range hooks {
		hooks[i].Status, hooks[i].ReceivedAt, hooks[i].Response = "dead_lettered", now, []byte("response")
		if err := db.Save(ctx, &hooks[i]); err != nil {
			t.Fatal(err)
		}
	}
	filter := WebhookFilter{Source: "summary", Status: "dead_lettered", Query: "needle", Sort: "received_asc"}
	for offset := 0; offset < 3; offset++ {
		list, err := db.FilteredSummary(ctx, filter, 1, offset)
		if err != nil || len(list) != 1 || list[0].ID != hooks[offset].ID {
			t.Fatalf("offset=%d list=%+v err=%v", offset, list, err)
		}
		if len(list[0].Payload) != 0 || len(list[0].Response) != 0 || list[0].Headers != "" || list[0].Source != "summary" || list[0].Status != "dead_lettered" {
			t.Fatalf("unexpected summary: %+v", list[0])
		}
	}
	filter.Sort = "received_desc"
	list, err := db.FilteredSummary(ctx, filter, 2, 0)
	if err != nil || len(list) != 2 || list[0].ID != hooks[2].ID || list[1].ID != hooks[1].ID {
		t.Fatalf("unstable descending sort: %+v, %v", list, err)
	}
	full, err := db.Filtered(ctx, filter, 3, 0)
	if err != nil || len(full) != 3 || len(full[0].Payload) == 0 || full[0].Headers == "" || string(full[0].Response) != "response" {
		t.Fatalf("full records lost content: %+v, %v", full, err)
	}
}
