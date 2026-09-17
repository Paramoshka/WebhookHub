package handler

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"webhookhub/internal/model"
	"webhookhub/internal/storage"
	"webhookhub/web"
)

func TestProtectedTemplatesRenderCSRFToken(t *testing.T) {
	webhook := model.Webhook{ID: 1, Source: "stripe", Status: "pending"}
	rule := model.ForwardingRule{Source: "stripe", Target: "https://example.com/hook"}
	tests := []struct {
		name     string
		files    []string
		template string
		data     any
	}{
		{
			name:     "inspect",
			files:    []string{"base.html", "inspect.html", "inspect_body.html", "inspect_delivery.html"},
			template: "base",
			data: InspectWebhookData{Webhook: webhook, CSRFToken: "test-token",
				Payload:  inspectBody("payload", "Payload", []byte("{}")),
				Response: inspectBody("response", "Latest saved response", nil)},
		},
		{
			name:     "index",
			files:    []string{"base.html", "index.html"},
			template: "base",
			data:     PageData{CSRFToken: "test-token"},
		},
		{
			name:     "dashboard",
			files:    []string{"base.html", "dashboard.html"},
			template: "base",
			data:     WebhookPageData{CSRFToken: "test-token"},
		},
		{
			name:     "dlq",
			files:    []string{"base.html", "dlq.html"},
			template: "base",
			data:     DLQPageData{Webhooks: []model.Webhook{webhook}, CSRFToken: "test-token"},
		},
		{
			name:     "forwarding",
			files:    []string{"base.html", "forwarding.html"},
			template: "base",
			data:     ForwardingPageData{Rules: []model.ForwardingRule{rule}, CSRFToken: "test-token"},
		},
		{
			name:     "edit form",
			files:    []string{"edit_form.html"},
			template: "edit_form.html",
			data:     EditForwardingData{Rule: rule, CSRFToken: "test-token"},
		},
		{
			name:     "webhook partial",
			files:    []string{"logs.html", "partials.html"},
			template: "logs.html",
			data:     WebhookPageData{Webhooks: []model.Webhook{webhook}, CSRFToken: "test-token"},
		},
		{
			name:     "metrics",
			files:    []string{"metrics.html"},
			template: "metrics.html",
			data: DeliveryMetricsData{
				DeliveryMetrics: storage.DeliveryMetrics{RecentDeadLetters: []model.Webhook{webhook}},
				CSRFToken:       "test-token",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tmpl, err := template.ParseFS(web.Templates, test.files...)
			if err != nil {
				t.Fatal(err)
			}

			var output bytes.Buffer
			if err := tmpl.ExecuteTemplate(&output, test.template, test.data); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "test-token") {
				t.Fatal("rendered template does not contain CSRF token")
			}
		})
	}
}
