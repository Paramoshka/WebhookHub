package handler

import (
	"html/template"
	"strings"

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

var templateFuncs = template.FuncMap{"statusLabel": statusLabel}

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
