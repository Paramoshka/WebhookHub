package storage

import (
	"bytes"
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
		if err := db.Save(&hooks[i]); err != nil {
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
			list, err := db.Filtered(filter, 10, 0)
			if err != nil {
				t.Fatal(err)
			}
			count, err := db.CountFiltered(filter)
			if err != nil {
				t.Fatal(err)
			}
			if len(list) != test.want || count != test.want {
				t.Fatalf("want %d matches, got list=%d count=%d", test.want, len(list), count)
			}
		})
	}
	for _, hook := range hooks {
		stored, err := db.FindByID(int(hook.ID))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(stored.Payload, hook.Payload) {
			t.Fatalf("payload changed for %s", hook.Source)
		}
	}
}

func TestFindByID(t *testing.T) {
	db := openTestDB(t)
	truncateTestTables(t, db)
	hook := model.Webhook{Source: "inspect", Status: "pending", ReceivedAt: time.Now()}
	if err := db.Save(&hook); err != nil {
		t.Fatal(err)
	}
	stored, err := db.FindByID(int(hook.ID))
	if err != nil || stored.ID != hook.ID {
		t.Fatalf("expected webhook %d, got %+v, err=%v", hook.ID, stored, err)
	}
	for _, id := range []int{0, -1, int(hook.ID) + 1} {
		if _, err := db.FindByID(id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("id %d: expected not found, got %v", id, err)
		}
	}
}
