package handler

import (
	"html"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFilterDatesUseUTC(t *testing.T) {
	original := time.Local
	t.Cleanup(func() { time.Local = original })
	for _, offset := range []int{7 * 3600, -4 * 3600} {
		time.Local = time.FixedZone("server", offset)
		for _, test := range []struct {
			value string
			upper bool
			want  string
		}{
			{"2026-09-16T10:30", false, "2026-09-16T10:30:00Z"},
			{"2026-09-16", false, "2026-09-16T00:00:00Z"},
			{"2026-09-16", true, "2026-09-17T00:00:00Z"},
			{"2026-09-16T10:30:00+07:00", false, "2026-09-16T03:30:00Z"},
			{"2026-09-16T23:30:00-04:00", true, "2026-09-17T03:30:00Z"},
			{"2026-09-16T10:30:12.123456789Z", false, "2026-09-16T10:30:12.123456789Z"},
			{"2026-12-31", true, "2027-01-01T00:00:00Z"},
		} {
			got := parseWebhookDateTime(test.value, test.upper)
			if got == nil || got.UTC().Format(time.RFC3339Nano) != test.want {
				t.Fatalf("offset=%d value=%s got=%v want=%s", offset, test.value, got, test.want)
			}
		}
	}
}

func TestDashboardDisplaysFilterDatesInUTC(t *testing.T) {
	values := url.Values{"from": {"2026-09-16T10:30:00+07:00"}, "to": {"2026-09-16"}}
	r := httptest.NewRequest("GET", "/dashboard?"+values.Encode(), nil)
	w := httptest.NewRecorder()
	DashboardUI(nil)(w, r)
	body := html.UnescapeString(w.Body.String())
	for _, want := range []string{"From (UTC)", "To (UTC)", `name="from" value="2026-09-16T03:30"`, `name="to" value="2026-09-17T00:00"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in form", want)
		}
	}
	// The first request still uses the precise, original URL values.
	if !strings.Contains(body, buildWebhookListURL(values, 1)) {
		t.Fatal("initial filter lost the original instants")
	}
}
