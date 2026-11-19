package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

type testDLQStore struct {
	filter     storage.WebhookFilter
	limit      int
	offset     int
	webhooks   []model.Webhook
	total      int
	summaryErr error
	countErr   error
}

func (s *testDLQStore) FilteredSummary(_ context.Context, filter storage.WebhookFilter, limit, offset int) ([]model.Webhook, error) {
	s.filter, s.limit, s.offset = filter, limit, offset
	return s.webhooks, s.summaryErr
}

func (s *testDLQStore) CountFiltered(_ context.Context, _ storage.WebhookFilter) (int, error) {
	return s.total, s.countErr
}

func TestDLQListsOnlyDeadLetteredEvents(t *testing.T) {
	store := &testDLQStore{
		webhooks: []model.Webhook{{ID: 42, Source: "stripe", Status: "dead_lettered", FailureCount: 3}},
		total:    41,
	}
	request := httptest.NewRequest(http.MethodGet, "/dlq?source=stripe&q=oops&page=2", nil)
	response := httptest.NewRecorder()

	DLQUI(store)(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if store.filter.Status != "dead_lettered" {
		t.Fatalf("DLQ must force dead_lettered status, got %q", store.filter.Status)
	}
	if store.filter.Source != "stripe" || store.filter.Query != "oops" {
		t.Fatalf("filters lost: %+v", store.filter)
	}
	if store.limit != 20 || store.offset != 20 {
		t.Fatalf("expected page size 20 at offset 20, got limit=%d offset=%d", store.limit, store.offset)
	}
	body := response.Body.String()
	if !strings.Contains(body, "#42") || !strings.Contains(body, "stripe") {
		t.Fatal("dead-lettered row not rendered")
	}
}

func TestDLQShowsBulkReplaySummary(t *testing.T) {
	store := &testDLQStore{webhooks: []model.Webhook{{ID: 42, Source: "stripe"}}}
	request := httptest.NewRequest(http.MethodGet, "/dlq?requeued=2&skipped=1", nil)
	response := httptest.NewRecorder()

	DLQUI(store)(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "Queued for delivery: 2. Skipped: 1 (deleted or no longer in DLQ).") {
		t.Fatal("bulk replay summary not rendered")
	}
}

func TestDLQPaginationBounds(t *testing.T) {
	for _, test := range []struct {
		name         string
		page         string
		total        int
		prevDisabled bool
		nextDisabled bool
	}{
		{name: "first page", page: "1", total: 41, prevDisabled: true, nextDisabled: false},
		{name: "last full page", page: "2", total: 40, prevDisabled: false, nextDisabled: true},
		{name: "middle page", page: "2", total: 41, prevDisabled: false, nextDisabled: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &testDLQStore{webhooks: []model.Webhook{{ID: 1}}, total: test.total}
			request := httptest.NewRequest(http.MethodGet, "/dlq?page="+test.page, nil)
			response := httptest.NewRecorder()

			DLQUI(store)(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", response.Code)
			}
			body := response.Body.String()
			if got := strings.Contains(body, `is-disabled">Prev`); got != test.prevDisabled {
				t.Fatalf("Prev disabled=%v, want %v", got, test.prevDisabled)
			}
			if got := strings.Contains(body, `is-disabled">Next`); got != test.nextDisabled {
				t.Fatalf("Next disabled=%v, want %v", got, test.nextDisabled)
			}
		})
	}
}

func TestDLQEmptyStateExplainsQueue(t *testing.T) {
	store := &testDLQStore{}
	request := httptest.NewRequest(http.MethodGet, "/dlq", nil)
	response := httptest.NewRecorder()

	DLQUI(store)(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Dead-letter queue is empty") {
		t.Fatalf("missing empty state: %d %s", response.Code, response.Body.String())
	}
}

func TestDLQMapsStorageErrors(t *testing.T) {
	for _, test := range []struct {
		name       string
		summaryErr error
		countErr   error
	}{
		{name: "list unavailable", summaryErr: errors.New("down")},
		{name: "count unavailable", countErr: errors.New("down")},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &testDLQStore{summaryErr: test.summaryErr, countErr: test.countErr}
			request := httptest.NewRequest(http.MethodGet, "/dlq", nil)
			response := httptest.NewRecorder()

			DLQUI(store)(response, request)

			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected 503, got %d", response.Code)
			}
		})
	}
}
