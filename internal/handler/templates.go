package handler

import (
	"html/template"

	"webhookhub/web"
)

// Templates are parsed once at startup. Each page set is kept separate because
// pages define overlapping block names such as "title" and "content".
var (
	loginTemplates      = template.Must(template.ParseFS(web.Templates, "login.html"))
	indexTemplates      = template.Must(template.ParseFS(web.Templates, "base.html", "index.html"))
	dashboardTemplates  = template.Must(template.ParseFS(web.Templates, "base.html", "dashboard.html"))
	dlqTemplates        = template.Must(template.ParseFS(web.Templates, "base.html", "dlq.html"))
	forwardingTemplates = template.Must(template.ParseFS(web.Templates, "base.html", "forwarding.html"))
	editFormTemplates   = template.Must(template.ParseFS(web.Templates, "edit_form.html"))
	logsTemplates       = template.Must(template.ParseFS(web.Templates, "logs.html", "partials.html"))
	metricsTemplates    = template.Must(template.ParseFS(web.Templates, "metrics.html"))
	inspectTemplates    = template.Must(template.ParseFS(web.Templates, "base.html", "inspect.html", "inspect_delivery.html", "inspect_body.html"))
	attemptTemplates    = template.Must(template.ParseFS(web.Templates, "base.html", "attempt.html", "inspect_delivery.html", "inspect_body.html"))
)
