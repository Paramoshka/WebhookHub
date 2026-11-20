package handler

import (
	"bytes"
	"strings"
	"testing"

	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

func TestPagesDeclareLanguage(t *testing.T) {
	var base bytes.Buffer
	if err := dashboardTemplates.ExecuteTemplate(&base, "base", WebhookPageData{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(base.String(), `<html lang="en">`) {
		t.Fatal("base page missing lang attribute")
	}
	var login bytes.Buffer
	if err := loginTemplates.Execute(&login, LoginPageData{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(login.String(), `<html lang="en">`) {
		t.Fatal("login page missing lang attribute")
	}
}

func TestStatusChipsUseReadableLabels(t *testing.T) {
	for _, test := range []struct{ status, label string }{
		{"pending", "Pending"},
		{"processing", "Processing"},
		{"retrying", "Retrying"},
		{"success", "Success"},
		{"failed", "Failed"},
		{"skipped", "Skipped"},
		{"dead_lettered", "Dead-Lettered"},
	} {
		t.Run(test.status, func(t *testing.T) {
			var out bytes.Buffer
			data := WebhookPageData{Webhooks: []model.Webhook{{ID: 1, Status: test.status}}}
			if err := logsTemplates.Execute(&out, data); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), ">"+test.label+"</span>") {
				t.Fatalf("status chip not humanized: %s", out.String())
			}
		})
	}

	var out bytes.Buffer
	data := InspectWebhookData{
		Webhook:  model.Webhook{ID: 1, Status: "failed"},
		Attempts: []model.DeliveryAttempt{{ID: 2, Status: "cancelled"}, {ID: 3, Status: "interrupted"}},
		Payload:  inspectBody("payload", "Payload", []byte("{}")),
		Response: inspectBody("response", "Latest saved response", nil),
	}
	if err := inspectTemplates.ExecuteTemplate(&out, "base", data); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Cancelled", "Interrupted"} {
		if !strings.Contains(out.String(), ">"+label+"</span>") {
			t.Fatalf("attempt status chip %q missing", label)
		}
	}

	var attempt bytes.Buffer
	attemptData := InspectWebhookData{
		Webhook:  model.Webhook{ID: 1},
		Attempt:  &model.DeliveryAttempt{ID: 2, Status: "cancelled"},
		Response: inspectBody("response", "Response", nil),
	}
	if err := attemptTemplates.ExecuteTemplate(&attempt, "base", attemptData); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(attempt.String(), ">Cancelled</span>") {
		t.Fatal("attempt page chip not humanized")
	}
}

func TestReplayActionHasOneName(t *testing.T) {
	var logs bytes.Buffer
	if err := logsTemplates.Execute(&logs, WebhookPageData{Webhooks: []model.Webhook{{ID: 1, Status: "failed"}}, CSRFToken: "token"}); err != nil {
		t.Fatal(err)
	}
	var metrics bytes.Buffer
	metricsData := DeliveryMetricsData{DeliveryMetrics: storage.DeliveryMetrics{RecentDeadLetters: []model.Webhook{{ID: 1}}}, CSRFToken: "token"}
	if err := metricsTemplates.Execute(&metrics, metricsData); err != nil {
		t.Fatal(err)
	}
	var dlq bytes.Buffer
	if err := dlqTemplates.ExecuteTemplate(&dlq, "base", DLQPageData{Webhooks: []model.Webhook{{ID: 1}}, CSRFToken: "token"}); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"logs": logs.String(), "metrics": metrics.String(), "dlq": dlq.String()} {
		if !strings.Contains(body, ">Replay</button>") {
			t.Fatalf("%s: single-event action is not called Replay", name)
		}
		if strings.Contains(body, ">Retry</button>") || strings.Contains(body, ">Requeue</button>") {
			t.Fatalf("%s: inconsistent action name for the replay endpoint", name)
		}
	}
	if !strings.Contains(dlq.String(), ">Requeue selected</button>") {
		t.Fatal("bulk action lost its distinct name")
	}
}

func TestDLQDisabledPaginationIsNotFocusable(t *testing.T) {
	var out bytes.Buffer
	data := DLQPageData{
		Webhooks:    []model.Webhook{{ID: 1}},
		CurrentPage: 1,
		DisablePrev: true,
		DisableNext: true,
		PrevURL:     "/dlq?page=0",
		NextURL:     "/dlq?page=2",
	}
	if err := dlqTemplates.ExecuteTemplate(&out, "base", data); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	if strings.Contains(body, `href="/dlq?page=0"`) || strings.Contains(body, `href="/dlq?page=2"`) {
		t.Fatal("disabled pagination directions remain focusable links")
	}
	if strings.Count(body, `aria-disabled="true"`) != 2 {
		t.Fatal("disabled directions must be spans with aria-disabled")
	}
}

func TestDashboardFilterOffersSkipped(t *testing.T) {
	var out bytes.Buffer
	if err := dashboardTemplates.ExecuteTemplate(&out, "base", WebhookPageData{Status: "skipped"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `<option value="skipped" selected>Skipped</option>`) {
		t.Fatal("skipped status is missing from the filter")
	}
}

func TestLogsEmptyStateExplainsHowToReceive(t *testing.T) {
	var out bytes.Buffer
	if err := logsTemplates.Execute(&out, WebhookPageData{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "/hook/:source") {
		t.Fatal("empty logs state does not teach the intake path")
	}
}

func TestRuleFormsHaveVisibleLabels(t *testing.T) {
	var fwd bytes.Buffer
	if err := forwardingTemplates.ExecuteTemplate(&fwd, "base", ForwardingPageData{CSRFToken: "token"}); err != nil {
		t.Fatal(err)
	}
	body := fwd.String()
	for _, label := range []string{"Source", "Target URL", "Signing secret", "Signature header", "Tolerance, seconds", "Outgoing secret", "Max attempts", "Initial backoff, seconds"} {
		if !strings.Contains(body, `class="field-label">`+label+`</span>`) {
			t.Fatalf("forwarding form missing visible label %q", label)
		}
	}
	var dlq bytes.Buffer
	if err := dlqTemplates.ExecuteTemplate(&dlq, "base", DLQPageData{CSRFToken: "token"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dlq.String(), `class="field-label">Source</span>`) {
		t.Fatal("DLQ filter missing visible label")
	}
	var edit bytes.Buffer
	if err := editFormTemplates.Execute(&edit, EditForwardingData{Rule: model.ForwardingRule{Source: "stripe"}, CSRFToken: "token"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(edit.String(), `<div class="field-label">`) {
		t.Fatal("label must not contain a div (invalid nesting)")
	}
}

func TestLoginSupportsPasswordManagers(t *testing.T) {
	var out bytes.Buffer
	if err := loginTemplates.Execute(&out, LoginPageData{}); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	if !strings.Contains(body, `autocomplete="username"`) || !strings.Contains(body, `autocomplete="current-password"`) {
		t.Fatal("login fields missing autocomplete hints")
	}
}

func TestCopyIsSentenceCaseWithEllipsis(t *testing.T) {
	var fwd bytes.Buffer
	if err := forwardingTemplates.ExecuteTemplate(&fwd, "base", ForwardingPageData{CSRFToken: "token"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fwd.String(), ">Add rule</button>") {
		t.Fatal("add-rule button not sentence case")
	}
	var dlq bytes.Buffer
	if err := dlqTemplates.ExecuteTemplate(&dlq, "base", DLQPageData{CSRFToken: "token"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dlq.String(), ">Apply filter</button>") {
		t.Fatal("apply-filter button not sentence case")
	}
	var edit bytes.Buffer
	if err := editFormTemplates.Execute(&edit, EditForwardingData{Rule: model.ForwardingRule{Source: "stripe"}, CSRFToken: "token"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(edit.String(), ">Save changes</button>") {
		t.Fatal("save-changes button not sentence case")
	}
	var dash bytes.Buffer
	if err := dashboardTemplates.ExecuteTemplate(&dash, "base", WebhookPageData{}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Loading delivery metrics…", "Loading logs…"} {
		if !strings.Contains(dash.String(), want) {
			t.Fatalf("missing %q", want)
		}
	}
}
