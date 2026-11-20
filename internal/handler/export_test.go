package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

type testExportStore struct {
	filter   storage.WebhookFilter
	ctx      context.Context
	webhooks []model.Webhook
	err      error
	calls    int
}

func (s *testExportStore) StreamWebhooks(ctx context.Context, filter storage.WebhookFilter, yield func(model.Webhook) error) error {
	s.calls++
	s.ctx, s.filter = ctx, filter
	if s.err != nil {
		return s.err
	}
	for _, hook := range s.webhooks {
		if err := yield(hook); err != nil {
			return err
		}
	}
	return nil
}

func TestExportWebhooksStreamsNDJSON(t *testing.T) {
	store := &testExportStore{webhooks: []model.Webhook{
		{ID: 1, Source: "stripe", Status: "success", Payload: []byte(`{"event":"one"}`), ReceivedAt: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)},
		{ID: 2, Source: "stripe", Status: "failed", Payload: []byte{0xff, 0x00}, ReceivedAt: time.Date(2026, 11, 20, 11, 0, 0, 0, time.UTC)},
	}}
	request := httptest.NewRequest(http.MethodGet, "/api/webhooks/export?source=stripe", nil)
	response := httptest.NewRecorder()

	ExportWebhooks(store)(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if ct := response.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Fatalf("expected NDJSON content type, got %q", ct)
	}
	disposition := response.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, `attachment; filename="webhooks-`) || !strings.HasSuffix(disposition, `.ndjson"`) {
		t.Fatalf("expected attachment filename, got %q", disposition)
	}
	lines := strings.Split(strings.TrimSpace(response.Body.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d: %q", len(lines), response.Body.String())
	}
	var first, second model.Webhook
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatal(err)
	}
	if first.ID != 1 || first.Source != "stripe" || string(first.Payload) != `{"event":"one"}` {
		t.Fatalf("first line changed the event: %+v", first)
	}
	if second.ID != 2 || !bytes.Equal(second.Payload, []byte{0xff, 0x00}) {
		t.Fatalf("binary payload lost: %+v", second)
	}
	if store.filter.Source != "stripe" {
		t.Fatalf("filter not passed through: %+v", store.filter)
	}
	if store.ctx != request.Context() {
		t.Fatal("export must run under the request context")
	}
}

func TestExportWebhooksEmptyJournal(t *testing.T) {
	store := &testExportStore{}
	request := httptest.NewRequest(http.MethodGet, "/api/webhooks/export", nil)
	response := httptest.NewRecorder()

	ExportWebhooks(store)(response, request)

	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("empty export: code=%d body=%q", response.Code, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/x-ndjson" || !strings.HasPrefix(response.Header().Get("Content-Disposition"), "attachment;") {
		t.Fatal("empty export must still be a downloadable file")
	}
}

func TestExportWebhooksRejectsInvalidDates(t *testing.T) {
	store := &testExportStore{}
	request := httptest.NewRequest(http.MethodGet, "/api/webhooks/export?from=not-a-date", nil)
	response := httptest.NewRecorder()

	ExportWebhooks(store)(response, request)

	if response.Code != http.StatusBadRequest || store.calls != 0 {
		t.Fatalf("invalid dates: code=%d calls=%d", response.Code, store.calls)
	}
}

func TestExportWebhooksMapsStorageErrors(t *testing.T) {
	store := &testExportStore{err: errors.New("down")}
	request := httptest.NewRequest(http.MethodGet, "/api/webhooks/export", nil)
	response := httptest.NewRecorder()

	ExportWebhooks(store)(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}
	if response.Header().Get("Content-Disposition") != "" || response.Header().Get("Content-Type") == "application/x-ndjson" {
		t.Fatal("failed export must not look like an attachment")
	}
}

type abortingExportStore struct{}

func (abortingExportStore) StreamWebhooks(_ context.Context, _ storage.WebhookFilter, yield func(model.Webhook) error) error {
	if err := yield(model.Webhook{ID: 1, Source: "stripe", ReceivedAt: time.Now()}); err != nil {
		return err
	}
	return errors.New("stream broke")
}

func TestExportWebhooksKeepsStreamedRowsOnLateFailure(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/webhooks/export", nil)
	response := httptest.NewRecorder()

	ExportWebhooks(abortingExportStore{})(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("late failure must not rewrite the status, got %d", response.Code)
	}
	var hook model.Webhook
	if err := json.Unmarshal([]byte(strings.TrimSpace(response.Body.String())), &hook); err != nil || hook.ID != 1 {
		t.Fatalf("streamed row lost: %q %v", response.Body.String(), err)
	}
}

func TestExportWebhooksPassesAllFilters(t *testing.T) {
	store := &testExportStore{}
	request := httptest.NewRequest(http.MethodGet, "/api/webhooks/export?source=stripe&status=failed&q=oops&from=2026-11-01&to=2026-11-20&sort=received_asc&tz=Europe/Moscow&page=3", nil)
	response := httptest.NewRecorder()

	ExportWebhooks(store)(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	f := store.filter
	if f.Source != "stripe" || f.Status != "failed" || f.Query != "oops" || f.Sort != "received_asc" {
		t.Fatalf("filters lost: %+v", f)
	}
	if f.From == nil || f.To == nil {
		t.Fatalf("date bounds lost: %+v", f)
	}
	if got := f.From.UTC().Format(time.RFC3339); got != "2026-10-31T21:00:00Z" {
		t.Fatalf("from bound wrong: %s", got)
	}
	if got := f.To.UTC().Format(time.RFC3339); got != "2026-11-20T21:00:00Z" {
		t.Fatalf("to bound wrong: %s", got)
	}
}

func TestBuildExportURLPreservesCanonicalFilters(t *testing.T) {
	values := url.Values{
		"source": {"stripe"}, "status": {"failed"}, "q": {"a&b"},
		"from": {"2026-11-01T00:00:00Z"}, "to": {"2026-11-20T00:00:00Z"},
		"sort": {"received_asc"}, "page": {"3"}, "requeued": {"2"}, "tz": {"Europe/Moscow"},
	}
	got := buildExportURL(values)
	parsed, err := url.Parse(got)
	if err != nil || parsed.Path != "/api/webhooks/export" {
		t.Fatalf("unexpected export URL: %s %v", got, err)
	}
	q := parsed.Query()
	if q.Get("source") != "stripe" || q.Get("status") != "failed" || q.Get("q") != "a&b" || q.Get("sort") != "received_asc" ||
		q.Get("from") != "2026-11-01T00:00:00Z" || q.Get("to") != "2026-11-20T00:00:00Z" {
		t.Fatalf("filters lost: %s", got)
	}
	if q.Has("page") || q.Has("requeued") || q.Has("tz") {
		t.Fatalf("export URL carries non-filter keys: %s", got)
	}
	// tz is intentionally absent: canonicalDateQuery already rewrote from/to
	// as UTC RFC3339 instants, which the API honours without a timezone.
	if buildExportURL(url.Values{}) != "/api/webhooks/export" {
		t.Fatal("unfiltered export URL must not carry a trailing question mark")
	}
}

func TestLogsPartialLinksExportWithFilters(t *testing.T) {
	var out bytes.Buffer
	data := WebhookPageData{ExportURL: "/api/webhooks/export?source=stripe"}
	if err := logsTemplates.Execute(&out, data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `href="/api/webhooks/export?source=stripe"`) {
		t.Fatal("logs partial missing the export link")
	}
}
