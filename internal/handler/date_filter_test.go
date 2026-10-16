package handler

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFilterDatesInNamedTimezone(t *testing.T) {
	for _, test := range []struct {
		zone, from, to, wantFrom, wantTo string
	}{
		{"Europe/Moscow", "2026-10-16T00:30", "2026-10-16T01:30", "2026-10-15T21:30:00Z", "2026-10-15T22:30:00Z"},
		{"Asia/Kathmandu", "2026-10-16T10:00", "", "2026-10-16T04:15:00Z", ""},
		{"America/New_York", "2026-03-08", "2026-03-08", "2026-03-08T05:00:00Z", "2026-03-09T04:00:00Z"},
		{"America/New_York", "2026-11-01", "2026-11-01", "2026-11-01T04:00:00Z", "2026-11-02T05:00:00Z"},
		{"Europe/Berlin", "2026-10-25T02:30:00+02:00", "2026-10-25T02:30:00+01:00", "2026-10-25T00:30:00Z", "2026-10-25T01:30:00Z"},
		{"UTC", "2026-12-31T23:59:59.123456789Z", "2026-12-31", "2026-12-31T23:59:59.123456789Z", "2027-01-01T00:00:00Z"},
		{"Australia/Lord_Howe", "2026-04-05T02:30", "", "2026-04-04T16:00:00Z", ""},
		{"America/New_York", "2040-01-01T10:30", "", "2040-01-01T15:30:00Z", ""},
		{"America/New_York", "2040-12-31T23:30", "", "2041-01-01T04:30:00Z", ""},
	} {
		t.Run(test.zone+"/"+test.from, func(t *testing.T) {
			values := url.Values{"tz": {test.zone}, "from": {test.from}, "to": {test.to}}
			from, to, err := webhookFilterDates(values)
			if err != nil {
				t.Fatal(err)
			}
			if dateTimeInstant(from) != test.wantFrom || dateTimeInstant(to) != test.wantTo {
				t.Fatalf("got %s / %s, want %s / %s", dateTimeInstant(from), dateTimeInstant(to), test.wantFrom, test.wantTo)
			}
			canonical := canonicalDateQuery(values, from, to)
			if canonical.Get("tz") != "" || canonical.Get("from") != test.wantFrom || canonical.Get("to") != test.wantTo {
				t.Fatalf("non-canonical query: %v", canonical)
			}
			roundFrom, roundTo, err := webhookFilterDates(canonical)
			if err != nil || dateTimeInstant(roundFrom) != test.wantFrom || dateTimeInstant(roundTo) != test.wantTo {
				t.Fatalf("shared URL changed the range: %v", err)
			}
		})
	}
}

func TestInvalidFilterDatesFailBeforeReadingStorage(t *testing.T) {
	for _, test := range []struct{ zone, value, message string }{
		{"Invalid/Timezone", "2026-10-16T10:30", "unknown timezone"},
		{"Local", "2026-10-16T10:30", "unknown timezone"},
		{"UTC", "invalid", "invalid date"},
		{"UTC", "2026-02-30", "invalid date"},
		{"America/New_York", "2026-03-08T02:30", "does not exist"},
		{"America/New_York", "2026-11-01T01:30", "occurs twice"},
		{"Australia/Lord_Howe", "2026-04-05T01:45", "occurs twice"},
		{"Australia/Lord_Howe", "2026-10-04T02:15", "does not exist"},
		{"Pacific/Apia", "2011-12-30T12:00", "does not exist"},
	} {
		for _, field := range []string{"from", "to"} {
			values := url.Values{"tz": {test.zone}, field: {test.value}}
			for _, handler := range []http.HandlerFunc{DashboardUI(nil), WebhookPartial(nil), DLQUI(nil), ListWebhooks(nil)} {
				w := httptest.NewRecorder()
				handler(w, httptest.NewRequest("GET", "/?"+values.Encode(), nil))
				if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), test.message) {
					t.Fatalf("%s %v: code=%d body=%s", field, test, w.Code, w.Body.String())
				}
			}
		}
	}
}

func TestTimezoneFilterNavigationPreservesPreciseInstants(t *testing.T) {
	values := url.Values{"tz": {"Asia/Kathmandu"}, "from": {"2026-10-16T10:00:12.123456789"}, "page": {"2"}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/partials/webhooks?"+values.Encode(), nil)
	r.Header.Set("HX-Request", "true")
	r.Header.Set("X-Update-History", "true")
	WebhookPartial(&testListStore{})(w, r)
	if w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}
	location, err := url.Parse(w.Header().Get("HX-Push-Url"))
	if err != nil || location.Query().Get("from") != "2026-10-16T04:15:12.123456789Z" || location.Query().Has("tz") {
		t.Fatalf("invalid canonical location: %s %v", location, err)
	}
	body := html.UnescapeString(w.Body.String())
	if !strings.Contains(body, buildDashboardURL(location.Query(), 1)) || !strings.Contains(body, url.QueryEscape(buildDashboardURL(location.Query(), 2))) {
		t.Fatal("pagination or Inspect changed the date filters")
	}
}

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
			got, err := parseWebhookDateTime(test.value, test.upper, time.UTC)
			if err != nil || got == nil || got.UTC().Format(time.RFC3339Nano) != test.want {
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
	for _, want := range []string{"From (<span data-timezone-label>UTC</span>)", "To (<span data-timezone-label>UTC</span>)", `name="from" value="2026-09-16T03:30"`, `name="to" value="2026-09-17T00:00"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in form", want)
		}
	}
	values.Set("from", "2026-09-16T03:30:00Z")
	values.Set("to", "2026-09-17T00:00:00Z")
	if !strings.Contains(body, buildWebhookListURL(values, 1)) {
		t.Fatal("initial filter lost the canonical instants")
	}
}
