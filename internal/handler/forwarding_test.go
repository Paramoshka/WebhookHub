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
