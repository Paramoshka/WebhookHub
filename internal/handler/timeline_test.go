package handler

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"webhookhub/internal/model"
)

func TestMaxAttemptDuration(t *testing.T) {
	for _, test := range []struct {
		name     string
		attempts []model.DeliveryAttempt
		want     int64
	}{
		{name: "empty floored at one", attempts: nil, want: 1},
		{name: "zero durations floored at one", attempts: []model.DeliveryAttempt{{DurationMS: 0}}, want: 1},
		{name: "longest wins", attempts: []model.DeliveryAttempt{{DurationMS: 120}, {DurationMS: 3400}, {DurationMS: 800}}, want: 3400},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := maxAttemptDuration(test.attempts); got != test.want {
				t.Fatalf("got %d, want %d", got, test.want)
			}
		})
	}
}

func TestDurationBarWidth(t *testing.T) {
	for _, test := range []struct {
		name              string
		durationMS, maxMS int64
		want              int
	}{
		{name: "zero duration", durationMS: 0, maxMS: 1000, want: 0},
		{name: "negative duration", durationMS: -5, maxMS: 1000, want: 0},
		{name: "half", durationMS: 500, maxMS: 1000, want: 50},
		{name: "longest is full", durationMS: 1000, maxMS: 1000, want: 100},
		{name: "above max clamps", durationMS: 2000, maxMS: 1000, want: 100},
		{name: "zero max never divides", durationMS: 10, maxMS: 0, want: 100},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := durationBarWidth(test.durationMS, test.maxMS); got != test.want {
				t.Fatalf("got %d, want %d", got, test.want)
			}
		})
	}
}

func TestHTTPBadgeClass(t *testing.T) {
	for _, test := range []struct {
		status int
		want   string
	}{
		{200, "http-2xx"}, {204, "http-2xx"}, {299, "http-2xx"},
		{301, "http-3xx"}, {308, "http-3xx"},
		{400, "http-4xx"}, {404, "http-4xx"}, {499, "http-4xx"},
		{500, "http-5xx"}, {503, "http-5xx"},
		{0, "http-other"}, {199, "http-other"}, {600, "http-other"},
	} {
		if got := httpBadgeClass(test.status); got != test.want {
			t.Fatalf("status %d: got %q, want %q", test.status, got, test.want)
		}
	}
}

func TestDeliveryAttemptsRenderAsTimeline(t *testing.T) {
	started := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	data := InspectWebhookData{
		Webhook: model.Webhook{ID: 42, Status: "failed"},
		Attempts: []model.DeliveryAttempt{
			{ID: 7, Target: "https://example.com/hook", Status: "failed", HTTPStatus: 500, DurationMS: 400, StartedAt: started, ErrorMessage: "target responded with status 500"},
			{ID: 6, Target: "https://example.com/hook", Status: "success", HTTPStatus: 200, DurationMS: 100, StartedAt: started.Add(-time.Minute)},
		},
		ReturnURL: "/dashboard?page=2",
		Response:  inspectBody("response", "Latest saved response", nil),
	}
	var out bytes.Buffer
	if err := inspectTemplates.ExecuteTemplate(&out, "inspect-delivery", data); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	if strings.Contains(body, "<table") {
		t.Fatal("attempt history must not be a wide scrollable table")
	}
	for _, want := range []string{
		`<ol class="attempt-timeline">`,
		`href="/webhooks/42/attempts/7?return_to=%2Fdashboard%3Fpage%3D2"`,
		`class="http-badge http-5xx">500</span>`,
		`class="http-badge http-2xx">200</span>`,
		`>Failed</span>`,
		`>Success</span>`,
		`width: 100%`,
		`width: 25%`,
		`target responded with status 500`,
		`→ https://example.com/hook`,
		`data-local-time datetime="2026-11-20T12:00:00Z"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("timeline missing %q", want)
		}
	}
}

func TestDeliveryTimelineEmptyState(t *testing.T) {
	data := InspectWebhookData{
		Webhook:  model.Webhook{ID: 42, Status: "pending"},
		Response: inspectBody("response", "Latest saved response", nil),
	}
	var out bytes.Buffer
	if err := inspectTemplates.ExecuteTemplate(&out, "inspect-delivery", data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No delivery attempts yet.") {
		t.Fatal("empty timeline lost its guidance")
	}
}

func TestInspectBodyOffersPayloadStudio(t *testing.T) {
	var jsonOut bytes.Buffer
	if err := inspectTemplates.ExecuteTemplate(&jsonOut, "inspect-body", inspectBody("payload", "Payload", []byte(`{"event":"created"}`))); err != nil {
		t.Fatal(err)
	}
	body := jsonOut.String()
	if !strings.Contains(body, `data-body-text="formatted" data-highlight="json"`) {
		t.Fatal("formatted JSON pre missing highlight marker")
	}
	if !strings.Contains(body, "data-wrap-toggle") {
		t.Fatal("wrap toggle missing")
	}
	if strings.Contains(body, `data-body-text="raw" data-highlight`) {
		t.Fatal("raw view must not be highlighted")
	}

	var binOut bytes.Buffer
	if err := inspectTemplates.ExecuteTemplate(&binOut, "inspect-body", inspectBody("payload", "Payload", []byte{0xff, 0x00})); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(binOut.String(), "data-wrap-toggle") || strings.Contains(binOut.String(), "data-highlight") {
		t.Fatal("binary preview must not offer studio controls")
	}

	var emptyOut bytes.Buffer
	if err := inspectTemplates.ExecuteTemplate(&emptyOut, "inspect-body", inspectBody("response", "Response", nil)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(emptyOut.String(), "data-wrap-toggle") {
		t.Fatal("empty body must not offer wrap toggle")
	}
}
