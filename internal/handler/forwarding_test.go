package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

type testForwardingPauseStore struct {
	source string
	paused bool
	calls  int
	err    error
}

func (s *testForwardingPauseStore) SetForwardingPaused(_ context.Context, source string, paused bool) error {
	s.source, s.paused = source, paused
	s.calls++
	return s.err
}

func TestSetForwardingPaused(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		paused bool
		err    error
		want   int
	}{
		{"pause", "stripe", true, nil, http.StatusSeeOther},
		{"resume", "stripe", false, nil, http.StatusSeeOther},
		{"missing source", "", true, nil, http.StatusBadRequest},
		{"invalid source", "bad/source", true, nil, http.StatusBadRequest},
		{"not found", "stripe", true, storage.ErrNotFound, http.StatusNotFound},
		{"database unavailable", "stripe", true, errors.New("down"), http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &testForwardingPauseStore{err: test.err}
			form := url.Values{"source": {test.source}}
			r := httptest.NewRequest(http.MethodPost, "/forwarding/pause", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			SetForwardingPaused(store, test.paused)(w, r)
			if w.Code != test.want {
				t.Fatalf("status=%d, want %d", w.Code, test.want)
			}
			if test.want == http.StatusBadRequest {
				if store.calls != 0 {
					t.Fatal("invalid source reached storage")
				}
				return
			}
			if store.calls != 1 || store.source != test.source || store.paused != test.paused {
				t.Fatalf("unexpected mutation: %+v", store)
			}
			if test.want == http.StatusSeeOther && w.Header().Get("Location") != "/forwarding" {
				t.Fatal("missing redirect to forwarding rules")
			}
		})
	}
}

func TestForwardingTemplateShowsPauseStateAndQueue(t *testing.T) {
	for _, paused := range []bool{false, true} {
		var output bytes.Buffer
		data := ForwardingPageData{
			Rules:       []model.ForwardingRule{{Source: "stripe", Paused: paused}},
			QueueCounts: map[string]int64{"stripe": 7},
			CSRFToken:   "test-token",
		}
		if err := forwardingTemplates.ExecuteTemplate(&output, "base", data); err != nil {
			t.Fatal(err)
		}
		path, label := "/forwarding/pause", "Pause"
		if paused {
			path, label = "/forwarding/resume", "Resume"
		}
		body := output.String()
		for _, want := range []string{`action="` + path + `"`, `>` + label + `</button>`,
			`name="csrf_token" value="test-token"`, `name="source" value="stripe"`, "7 queued"} {
			if !strings.Contains(body, want) {
				t.Fatalf("paused=%v: missing %q", paused, want)
			}
		}
	}
}

type testForwardingListStore struct {
	rules     []model.ForwardingRule
	counts    map[string]int64
	rulesErr  error
	countsErr error
}

func (s *testForwardingListStore) GetForwardingRules(context.Context) ([]model.ForwardingRule, error) {
	return s.rules, s.rulesErr
}

func (s *testForwardingListStore) QueuedWebhookCounts(context.Context) (map[string]int64, error) {
	return s.counts, s.countsErr
}

func TestForwardingUIListsRulesWithQueues(t *testing.T) {
	store := &testForwardingListStore{
		rules:  []model.ForwardingRule{{Source: "stripe", Target: "https://example.com/hook"}},
		counts: map[string]int64{"stripe": 7},
	}
	request := httptest.NewRequest(http.MethodGet, "/forwarding", nil)
	response := httptest.NewRecorder()

	ForwardingUI(store)(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{"stripe", "https://example.com/hook", "7 queued"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in forwarding page", want)
		}
	}
}

func TestForwardingUIMapsStorageErrors(t *testing.T) {
	for _, test := range []struct {
		name      string
		rulesErr  error
		countsErr error
	}{
		{name: "rules unavailable", rulesErr: errors.New("down")},
		{name: "counts unavailable", countsErr: errors.New("down")},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &testForwardingListStore{rulesErr: test.rulesErr, countsErr: test.countsErr}
			request := httptest.NewRequest(http.MethodGet, "/forwarding", nil)
			response := httptest.NewRecorder()

			ForwardingUI(store)(response, request)

			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected 503, got %d", response.Code)
			}
		})
	}
}

type testForwardingSaveStore struct {
	saved   *model.ForwardingRule
	saveErr error
}

func (s *testForwardingSaveStore) SaveForwardingRule(_ context.Context, rule model.ForwardingRule) error {
	if s.saveErr == nil {
		s.saved = &rule
	}
	return s.saveErr
}

func postForm(path string, values url.Values) *http.Request {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return request
}

func TestSaveForwardingRulePersistsForm(t *testing.T) {
	store := &testForwardingSaveStore{}
	response := httptest.NewRecorder()

	SaveForwardingRule(store)(response, postForm("/forwarding/save", url.Values{"source": {"stripe"}, "target": {"https://example.com/hook"}}))

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/forwarding" {
		t.Fatalf("expected redirect to /forwarding, got %d %q", response.Code, response.Header().Get("Location"))
	}
	if store.saved == nil {
		t.Fatal("rule was not saved")
	}
	if store.saved.Source != "stripe" || store.saved.Target != "https://example.com/hook" {
		t.Fatalf("unexpected saved rule: %+v", store.saved)
	}
	if store.saved.RetryMaxAttempts != 3 || store.saved.RetryBackoffSeconds != 2 {
		t.Fatalf("expected default retry policy 3/2, got %d/%d", store.saved.RetryMaxAttempts, store.saved.RetryBackoffSeconds)
	}
	if store.saved.VerifySecret != "" || store.saved.OutgoingSecret != "" {
		t.Fatalf("secrets must stay empty without form values: %+v", store.saved)
	}
}

func TestSaveForwardingRuleRejectsInvalidForm(t *testing.T) {
	store := &testForwardingSaveStore{}
	response := httptest.NewRecorder()

	SaveForwardingRule(store)(response, postForm("/forwarding/save", url.Values{"source": {"stripe"}}))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing target, got %d", response.Code)
	}
	if store.saved != nil {
		t.Fatal("invalid form reached storage")
	}
}

func TestSaveForwardingRuleMapsStorageErrors(t *testing.T) {
	store := &testForwardingSaveStore{saveErr: errors.New("down")}
	response := httptest.NewRecorder()

	SaveForwardingRule(store)(response, postForm("/forwarding/save", url.Values{"source": {"stripe"}, "target": {"https://example.com/hook"}}))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}
}

type testForwardingEditStore struct {
	rule   model.ForwardingRule
	err    error
	source string
}

func (s *testForwardingEditStore) GetForwardingRule(_ context.Context, source string) (model.ForwardingRule, error) {
	s.source = source
	return s.rule, s.err
}

func TestEditForwardingFormPrefillsRule(t *testing.T) {
	store := &testForwardingEditStore{rule: model.ForwardingRule{Source: "stripe", Target: "https://example.com/hook", VerifySecret: "whsec_old"}}
	request := httptest.NewRequest(http.MethodGet, "/forwarding/edit?source=stripe", nil)
	response := httptest.NewRecorder()

	EditForwardingForm(store)(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if store.source != "stripe" {
		t.Fatalf("expected lookup for stripe, got %q", store.source)
	}
	body := response.Body.String()
	for _, want := range []string{`action="/forwarding/update"`, `name="source" value="stripe"`, `value="https://example.com/hook"`, "Leave blank to keep current secret"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in edit form", want)
		}
	}
}

func TestEditForwardingFormMapsErrors(t *testing.T) {
	for _, test := range []struct {
		name  string
		query string
		err   error
		want  int
	}{
		{name: "missing source", query: "", err: nil, want: http.StatusBadRequest},
		{name: "not found", query: "?source=stripe", err: storage.ErrNotFound, want: http.StatusNotFound},
		{name: "database unavailable", query: "?source=stripe", err: errors.New("down"), want: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &testForwardingEditStore{err: test.err}
			request := httptest.NewRequest(http.MethodGet, "/forwarding/edit"+test.query, nil)
			response := httptest.NewRecorder()

			EditForwardingForm(store)(response, request)

			if response.Code != test.want {
				t.Fatalf("expected %d, got %d", test.want, response.Code)
			}
		})
	}
}

type testForwardingUpdateStore struct {
	testForwardingEditStore
	testForwardingSaveStore
}

func TestUpdateForwardingRuleKeepsSecretsAndPause(t *testing.T) {
	store := &testForwardingUpdateStore{
		testForwardingEditStore: testForwardingEditStore{rule: model.ForwardingRule{
			Source: "stripe", Target: "https://old.example.com/hook", Paused: true,
			VerifySecret: "whsec_old", VerifyHeader: "X-Sig", ToleranceSeconds: 600,
			OutgoingSecret: "out_old", RetryMaxAttempts: 5, RetryBackoffSeconds: 4,
		}},
	}
	response := httptest.NewRecorder()
	form := url.Values{
		"source": {"stripe"}, "target": {"https://new.example.com/hook"},
		"retry_max_attempts": {"5"}, "retry_backoff_seconds": {"4"},
	}

	UpdateForwardingRule(store)(response, postForm("/forwarding/update", form))

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/forwarding" {
		t.Fatalf("expected redirect to /forwarding, got %d %q", response.Code, response.Header().Get("Location"))
	}
	if store.saved == nil {
		t.Fatal("rule was not saved")
	}
	saved := *store.saved
	if saved.Target != "https://new.example.com/hook" {
		t.Fatalf("target not updated: %+v", saved)
	}
	if saved.VerifySecret != "whsec_old" || saved.VerifyHeader != "X-Sig" || saved.ToleranceSeconds != 600 {
		t.Fatalf("incoming verification lost on update: %+v", saved)
	}
	if saved.OutgoingSecret != "out_old" {
		t.Fatalf("outgoing secret lost on update: %+v", saved)
	}
	if !saved.Paused {
		t.Fatal("update must not resume a paused rule")
	}
	if saved.RetryMaxAttempts != 5 || saved.RetryBackoffSeconds != 4 {
		t.Fatalf("retry policy lost on update: %+v", saved)
	}
}

func TestUpdateForwardingRuleClearsVerificationOnRequest(t *testing.T) {
	store := &testForwardingUpdateStore{
		testForwardingEditStore: testForwardingEditStore{rule: model.ForwardingRule{
			Source: "stripe", Target: "https://example.com/hook", VerifySecret: "whsec_old", ToleranceSeconds: 600,
		}},
	}
	response := httptest.NewRecorder()
	form := url.Values{"source": {"stripe"}, "target": {"https://example.com/hook"}, "clear_verify_secret": {"1"}}

	UpdateForwardingRule(store)(response, postForm("/forwarding/update", form))

	if response.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", response.Code)
	}
	if store.saved == nil || store.saved.VerifySecret != "" || store.saved.VerifyHeader != "" || store.saved.ToleranceSeconds != 0 {
		t.Fatalf("clear_verify_secret did not reset verification: %+v", store.saved)
	}
}

func TestUpdateForwardingRuleMapsErrors(t *testing.T) {
	validForm := url.Values{"source": {"stripe"}, "target": {"https://example.com/hook"}}
	for _, test := range []struct {
		name    string
		form    url.Values
		getErr  error
		saveErr error
		want    int
	}{
		{name: "not found", form: validForm, getErr: storage.ErrNotFound, want: http.StatusNotFound},
		{name: "lookup unavailable", form: validForm, getErr: errors.New("down"), want: http.StatusServiceUnavailable},
		{name: "invalid target", form: url.Values{"source": {"stripe"}, "target": {"/relative"}}, want: http.StatusBadRequest},
		{name: "save unavailable", form: validForm, saveErr: errors.New("down"), want: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &testForwardingUpdateStore{
				testForwardingEditStore: testForwardingEditStore{err: test.getErr},
				testForwardingSaveStore: testForwardingSaveStore{saveErr: test.saveErr},
			}
			response := httptest.NewRecorder()

			UpdateForwardingRule(store)(response, postForm("/forwarding/update", test.form))

			if response.Code != test.want {
				t.Fatalf("expected %d, got %d", test.want, response.Code)
			}
			if test.want != http.StatusSeeOther && store.saved != nil {
				t.Fatal("failed update reached storage save")
			}
		})
	}
}

type testForwardingDeleteStore struct {
	deletedSource string
	calls         int
	err           error
}

func (s *testForwardingDeleteStore) DeleteForwardingRule(_ context.Context, source string) error {
	s.calls++
	s.deletedSource = source
	return s.err
}

func TestDeleteForwardingRule(t *testing.T) {
	for _, test := range []struct {
		name      string
		source    string
		err       error
		want      int
		wantCalls int
	}{
		{name: "deletes rule", source: "stripe", want: http.StatusSeeOther, wantCalls: 1},
		{name: "empty source is a no-op", source: "", want: http.StatusSeeOther, wantCalls: 0},
		{name: "database unavailable", source: "stripe", err: errors.New("down"), want: http.StatusServiceUnavailable, wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &testForwardingDeleteStore{err: test.err}
			form := url.Values{"source": {test.source}}
			response := httptest.NewRecorder()

			DeleteForwardingRule(store)(response, postForm("/forwarding/delete", form))

			if response.Code != test.want {
				t.Fatalf("expected %d, got %d", test.want, response.Code)
			}
			if store.calls != test.wantCalls {
				t.Fatalf("expected %d delete calls, got %d", test.wantCalls, store.calls)
			}
			if test.wantCalls == 1 && store.deletedSource != test.source {
				t.Fatalf("expected delete of %q, got %q", test.source, store.deletedSource)
			}
			if test.want == http.StatusSeeOther && response.Header().Get("Location") != "/forwarding" {
				t.Fatal("missing redirect to forwarding rules")
			}
		})
	}
}
