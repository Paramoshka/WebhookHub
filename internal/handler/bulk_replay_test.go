package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"webhookhub/internal/storage"
)

type testBulkStore struct {
	ids []int
	ctx context.Context
	err error
}

func (s *testBulkStore) BulkReplayDeadLetters(ctx context.Context, ids []int) (storage.BulkReplayResult, error) {
	s.ctx = ctx
	s.ids = ids
	return storage.BulkReplayResult{Requeued: 1, Skipped: 1}, s.err
}

func TestBulkReplayHandler(t *testing.T) {
	for _, target := range []string{"/dlq?source=stripe&page=2&q=a%26b", "https://example.com/dlq", "//example.com/dlq", "/dashboard", "/dlq#fragment", "%"} {
		store := &testBulkStore{}
		form := url.Values{"ids": {"2", "1", "2"}, "redirect_to": {target}}
		r := httptest.NewRequest("POST", "/api/webhooks/replay/bulk", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		BulkReplayWebhooks(store)(w, r)
		if w.Code != 303 || !reflect.DeepEqual(store.ids, []int{2, 1}) || store.ctx != r.Context() {
			t.Fatalf("code=%d ids=%v", w.Code, store.ids)
		}
		location, err := url.Parse(w.Header().Get("Location"))
		if err != nil || location.Host != "" || location.Path != "/dlq" || location.Query().Get("requeued") != "1" || location.Query().Get("skipped") != "1" {
			t.Fatalf("redirect=%s err=%v", location, err)
		}
		if strings.HasPrefix(target, "/dlq?") && (location.Query().Get("source") != "stripe" || location.Query().Get("page") != "2" || location.Query().Get("q") != "a&b") {
			t.Fatal("return filters lost")
		}
	}
	store := &testBulkStore{err: errors.New("unavailable")}
	r := httptest.NewRequest("POST", "/api/webhooks/replay/bulk", strings.NewReader("ids=1"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	BulkReplayWebhooks(store)(w, r)
	if w.Code != 503 || w.Header().Get("Location") != "" {
		t.Fatal("database error was hidden")
	}
}

func TestBulkReplayRejectsInvalidSelection(t *testing.T) {
	tooMany := url.Values{}
	for id := 1; id <= 21; id++ {
		tooMany.Add("ids", strconv.Itoa(id))
	}
	for _, body := range []string{"", "ids=0", "ids=-1", "ids=no", "ids=1&ids=no", "ids=999999999999999999999999", tooMany.Encode()} {
		store := &testBulkStore{}
		r := httptest.NewRequest("POST", "/api/webhooks/replay/bulk", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		BulkReplayWebhooks(store)(w, r)
		if w.Code != 400 || store.ids != nil {
			t.Fatalf("body=%q code=%d ids=%v", body, w.Code, store.ids)
		}
	}
}

func TestBulkReplayRequiresCSRF(t *testing.T) {
	auth, err := NewAuth(strings.Repeat("a", 32), false, false)
	if err != nil {
		t.Fatal(err)
	}
	store := &testBulkStore{}
	w := httptest.NewRecorder()
	auth.RequireCSRF(BulkReplayWebhooks(store))(w, httptest.NewRequest(http.MethodPost, "/api/webhooks/replay/bulk", strings.NewReader("ids=1")))
	if w.Code != 403 || store.ids != nil {
		t.Fatal("unprotected bulk replay")
	}
}

func TestBulkReplaySummaryIsBoundedAndNotPreservedInLinks(t *testing.T) {
	for _, query := range []string{"requeued=1&skipped=2", "requeued=-1&skipped=0", "requeued=20&skipped=1", "requeued=99999999999999999999&skipped=1", "requeued=1&skipped=no"} {
		values, err := url.ParseQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		message := bulkReplayMessage(values)
		if (message != "") != (query == "requeued=1&skipped=2") {
			t.Fatalf("query=%s message=%q", query, message)
		}
		link := buildDLQPageURLWithFilters(values, 2)
		if strings.Contains(link, "requeued") || strings.Contains(link, "skipped") {
			t.Fatal("summary was preserved in navigation")
		}
	}
}
