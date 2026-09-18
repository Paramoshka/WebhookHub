package handler

import (
	"html"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

type testListStore struct{ offset int }

func (s *testListStore) Filtered(_ storage.WebhookFilter, _, offset int) ([]model.Webhook, error) {
	s.offset = offset
	return []model.Webhook{{ID: 42, Source: "test"}}, nil
}
func (*testListStore) CountFiltered(storage.WebhookFilter) (int, error) { return 40, nil }

func TestDashboardRestoresPageAndFilters(t *testing.T) {
	values := url.Values{"page": {"3"}, "source": {"test"}, "q": {"a&b + Привет"}}
	r := httptest.NewRequest("GET", "/dashboard?"+values.Encode(), nil)
	w := httptest.NewRecorder()
	DashboardUI(nil)(w, r)
	body := html.UnescapeString(w.Body.String())
	if w.Code != 200 || !strings.Contains(body, buildWebhookListURL(values, 3)) || !strings.Contains(body, `value="a&b + Привет"`) {
		t.Fatalf("page state not restored: %d %s", w.Code, body)
	}
}

func TestWebhookListHistoryOnlyForNavigation(t *testing.T) {
	values := url.Values{"page": {"3"}, "q": {"a&b + Привет"}}
	for _, navigate := range []bool{false, true} {
		store := &testListStore{}
		r := httptest.NewRequest("GET", buildWebhookListURL(values, 3), nil)
		r.Header.Set("HX-Request", "true")
		if navigate {
			r.Header.Set("X-Update-History", "true")
		}
		w := httptest.NewRecorder()
		WebhookPartial(store)(w, r)
		want := ""
		if navigate {
			want = buildDashboardURL(values, 3)
		}
		if w.Code != 200 || store.offset != 20 || w.Header().Get("HX-Push-Url") != want {
			t.Fatalf("navigation: code=%d offset=%d history=%q", w.Code, store.offset, w.Header().Get("HX-Push-Url"))
		}
		body := html.UnescapeString(w.Body.String())
		if !strings.Contains(body, `href="`+buildDashboardURL(values, 2)+`"`) || !strings.Contains(body, "return_to="+url.QueryEscape(buildDashboardURL(values, 3))) {
			t.Fatal("links do not preserve canonical page")
		}
	}
}

func TestInspectReturnNavigation(t *testing.T) {
	for _, test := range []struct{ target, want string }{
		{"/dashboard?page=3&q=a%26b", "/dashboard?page=3&q=a%26b"},
		{"/dlq?source=test&page=2", "/dlq?source=test&page=2"},
		{"https://example.com/dashboard", "/dashboard"},
		{"//example.com/dashboard", "/dashboard"},
		{"/login", "/dashboard"},
		{"/dashboard#fragment", "/dashboard"},
	} {
		r := httptest.NewRequest("GET", "/webhooks/42?return_to="+url.QueryEscape(test.target), nil)
		data := InspectWebhookData{Webhook: model.Webhook{ID: 42}}
		setInspectNavigation(&data, r)
		if data.ReturnURL != test.want {
			t.Fatalf("%q -> %q", test.target, data.ReturnURL)
		}
		parsed, err := url.Parse(data.InspectURL)
		if err != nil || parsed.Path != "/webhooks/42" || parsed.Query().Get("return_to") != test.want {
			t.Fatalf("inspect return link: %s %v", data.InspectURL, err)
		}
	}
}
