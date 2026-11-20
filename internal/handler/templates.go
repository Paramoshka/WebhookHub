package handler

import (
	"html/template"
	"strings"

	"webhookhub/internal/model"
	"webhookhub/web"
)

// statusLabels maps stored status values to the labels shown in chips and
// filters, so a value like dead_lettered never reaches the page raw.
var statusLabels = map[string]string{
	"pending":       "Pending",
	"processing":    "Processing",
	"retrying":      "Retrying",
	"success":       "Success",
	"failed":        "Failed",
	"skipped":       "Skipped",
	"dead_lettered": "Dead-Lettered",
	"cancelled":     "Cancelled",
	"interrupted":   "Interrupted",
}

func statusLabel(status string) string {
	if label, ok := statusLabels[status]; ok {
		return label
	}
	return strings.ReplaceAll(status, "_", " ")
}

// maxAttemptDuration returns the longest attempt duration, floored at 1 ms so
// duration bars never divide by zero.
func maxAttemptDuration(attempts []model.DeliveryAttempt) int64 {
	var longest int64
	for _, attempt := range attempts {
		if attempt.DurationMS > longest {
			longest = attempt.DurationMS
		}
	}
	return max(longest, 1)
}

// durationBarWidth scales an attempt duration against the longest one, 0..100.
func durationBarWidth(durationMS, maxMS int64) int {
	if durationMS <= 0 {
		return 0
	}
	if maxMS < 1 {
		maxMS = 1
	}
	return int(min(durationMS, maxMS) * 100 / maxMS)
}

// httpBadgeClass groups HTTP status codes into color bands for attempt badges.
func httpBadgeClass(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "http-2xx"
	case status >= 300 && status < 400:
		return "http-3xx"
	case status >= 400 && status < 500:
		return "http-4xx"
	case status >= 500 && status < 600:
		return "http-5xx"
	default:
		return "http-other"
	}
}

var templateFuncs = template.FuncMap{
	"statusLabel":        statusLabel,
	"maxAttemptDuration": maxAttemptDuration,
	"durationBarWidth":   durationBarWidth,
	"httpBadgeClass":     httpBadgeClass,
}

// parseTemplatesFS parses a page set with the shared funcs. Like
// template.ParseFS, the root template takes the first file's name so Execute
// runs that file; Funcs must be registered before parsing.
func parseTemplatesFS(files ...string) (*template.Template, error) {
	return template.New(files[0]).Funcs(templateFuncs).ParseFS(web.Templates, files...)
}

func parseTemplates(files ...string) *template.Template {
	return template.Must(parseTemplatesFS(files...))
}

// Templates are parsed once at startup. Each page set is kept separate because
// pages define overlapping block names such as "title" and "content".
var (
	loginTemplates      = parseTemplates("login.html")
	indexTemplates      = parseTemplates("base.html", "index.html")
	dashboardTemplates  = parseTemplates("base.html", "dashboard.html")
	dlqTemplates        = parseTemplates("base.html", "dlq.html")
	forwardingTemplates = parseTemplates("base.html", "forwarding.html")
	editFormTemplates   = parseTemplates("edit_form.html")
	logsTemplates       = parseTemplates("logs.html", "partials.html")
	metricsTemplates    = parseTemplates("metrics.html")
	inspectTemplates    = parseTemplates("base.html", "inspect.html", "inspect_delivery.html", "inspect_body.html")
	attemptTemplates    = parseTemplates("base.html", "attempt.html", "inspect_delivery.html", "inspect_body.html")
)
