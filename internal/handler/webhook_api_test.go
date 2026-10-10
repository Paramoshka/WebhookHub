package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

type testAPIStore struct {
	hooks         []model.Webhook
	err           error
	filter        storage.WebhookFilter
	limit, offset int
	ctx           context.Context
	called        bool
}

func (s *testAPIStore) Filtered(ctx context.Context, filter storage.WebhookFilter, limit, offset int) ([]model.Webhook, error) {
	s.ctx, s.filter, s.limit, s.offset, s.called = ctx, filter, limit, offset, true
	return s.hooks, s.err
}

func TestWebhookAPIPagination(t *testing.T) {
	for _, test := range []struct {
		query                         string
		rows, wantRows, limit, offset int
		next                          bool
	}{
		{"", 21, 20, 21, 0, true},
		{"page=3&limit=2", 3, 2, 3, 4, true},
		{"page=3&limit=2", 2, 2, 3, 4, false},
		{"page=4&limit=2", 0, 0, 3, 6, false},
		{"limit=100", 101, 100, 101, 0, true},
	} {
		t.Run(test.query+"/"+strconv.Itoa(test.rows), func(t *testing.T) {
			store := &testAPIStore{}
			for i := 0; i < test.rows; i++ {
				store.hooks = append(store.hooks, model.Webhook{ID: uint(i + 1), Source: "binary", Payload: []byte{0, 0xff}, Response: []byte{0xfe}, Headers: `{"X-Test":["value"]}`})
			}
			r := httptest.NewRequest(http.MethodGet, "/api/webhooks?"+test.query, nil)
			w := httptest.NewRecorder()
			ListWebhooks(store)(w, r)
			var hooks []model.Webhook
			if err := json.Unmarshal(w.Body.Bytes(), &hooks); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || len(hooks) != test.wantRows || store.limit != test.limit || store.offset != test.offset || store.ctx != r.Context() {
				t.Fatalf("code=%d rows=%d limit=%d offset=%d", w.Code, len(hooks), store.limit, store.offset)
			}
			if len(hooks) > 0 && (!bytes.Equal(hooks[0].Payload, []byte{0, 0xff}) || !bytes.Equal(hooks[0].Response, []byte{0xfe}) || hooks[0].Headers != store.hooks[0].Headers) {
				t.Fatal("event fields changed")
			}
			if test.wantRows == 0 && strings.TrimSpace(w.Body.String()) != "[]" {
				t.Fatal("empty page must be an array")
			}
			link := w.Header().Get("Link")
			if (link != "") != test.next {
				t.Fatalf("unexpected next page: %q", link)
			}
			if test.next {
				if !strings.HasSuffix(link, `>; rel="next"`) {
					t.Fatal(link)
				}
				next, err := url.Parse(strings.TrimSuffix(strings.TrimPrefix(link, "<"), `>; rel="next"`))
				page, limit, _ := parseWebhookPagination(r.URL.Query())
				if err != nil || next.Path != "/api/webhooks" || next.Query().Get("page") != strconv.Itoa(page+1) || next.Query().Get("limit") != strconv.Itoa(limit) {
					t.Fatal(link)
				}
			}
		})
	}
}

func TestWebhookAPIRejectsInvalidPagination(t *testing.T) {
	for _, query := range []string{"page=0", "page=-1", "page=no", "page=" + strconv.Itoa(math.MaxInt), "page=" + strconv.Itoa(math.MaxInt/2+1) + "&limit=2", "limit=0", "limit=-1", "limit=101", "limit=no", "page=999999999999999999999999"} {
		store := &testAPIStore{}
		w := httptest.NewRecorder()
		ListWebhooks(store)(w, httptest.NewRequest("GET", "/api/webhooks?"+query, nil))
		if w.Code != 400 || store.called {
			t.Fatalf("%s: code=%d queried=%v", query, w.Code, store.called)
		}
	}
}

func TestWebhookAPIFiltersAndDatabaseError(t *testing.T) {
	query := url.Values{"source": {"a&b"}, "status": {"failed"}, "q": {"привет + %"}, "from": {"2026-10-01"}, "to": {"2026-10-02"}, "sort": {"received_asc"}, "limit": {"1"}}
	store := &testAPIStore{hooks: []model.Webhook{{ID: 1}, {ID: 2}}}
	w := httptest.NewRecorder()
	ListWebhooks(store)(w, httptest.NewRequest("GET", "/api/webhooks?"+query.Encode(), nil))
	if store.filter.Source != "a&b" || store.filter.Status != "failed" || store.filter.Query != "привет + %" || store.filter.Sort != "received_asc" || store.filter.From == nil || store.filter.To == nil || !store.filter.To.Equal(time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("filter lost: %+v", store.filter)
	}
	next, err := url.Parse(strings.TrimSuffix(strings.TrimPrefix(w.Header().Get("Link"), "<"), `>; rel="next"`))
	if err != nil {
		t.Fatal(err)
	}
	for key, values := range query {
		if next.Query().Get(key) != values[0] {
			t.Fatalf("next link lost %s", key)
		}
	}
	store.err = errors.New("unavailable")
	w = httptest.NewRecorder()
	ListWebhooks(store)(w, httptest.NewRequest("GET", "/api/webhooks", nil))
	if w.Code != 503 || w.Header().Get("Link") != "" {
		t.Fatalf("database error: %d", w.Code)
	}
}
