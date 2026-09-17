package handler

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"unicode/utf8"

	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

type webhookInspectStore interface {
	FindByID(int) (model.Webhook, error)
	DeliveryAttemptsByWebhook(uint) ([]model.DeliveryAttempt, error)
}

type InspectBody struct {
	ID        string
	Label     string
	Size      int
	Raw       string
	Formatted string
	JSON      bool
	Binary    bool
	Truncated bool
}

type InspectWebhookData struct {
	Webhook      model.Webhook
	Attempts     []model.DeliveryAttempt
	CSRFToken    string
	Headers      http.Header
	HeadersError bool
	Payload      InspectBody
	Response     InspectBody
	Attempt      *model.DeliveryAttempt
	ReturnURL    string
	InspectURL   string
}

type deliveryAttemptStore interface {
	DeliveryAttemptByID(uint, uint) (model.DeliveryAttempt, error)
}

func InspectDeliveryAttempt(db deliveryAttemptStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		webhookID, err := strconv.Atoi(r.PathValue("id"))
		attemptID, attemptErr := strconv.Atoi(r.PathValue("attemptID"))
		if err != nil || attemptErr != nil || webhookID <= 0 || attemptID <= 0 {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		attempt, err := db.DeliveryAttemptByID(uint(webhookID), uint(attemptID))
		if errors.Is(err, storage.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("load delivery attempt %d: %v", attemptID, err)
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		data := InspectWebhookData{
			Webhook: model.Webhook{ID: uint(webhookID)}, Attempt: &attempt, CSRFToken: CSRFToken(r),
			Response: inspectBody("response", "Response body", attempt.ResponseBody),
		}
		setInspectNavigation(&data, r)
		data.Response.Truncated = attempt.ResponseTruncated
		if attempt.ResponseHeaders != "" {
			if err := json.Unmarshal([]byte(attempt.ResponseHeaders), &data.Headers); err != nil {
				data.HeadersError = true
				log.Printf("decode delivery attempt %d headers: %v", attemptID, err)
			}
		}
		renderInspect(w, "base", data)
	}
}

func InspectWebhook(db webhookInspectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		webhook, ok := findInspectWebhook(w, r, db)
		if !ok {
			return
		}
		data, err := inspectDeliveryData(db, webhook, CSRFToken(r))
		if err != nil {
			log.Printf("load webhook %d delivery attempts: %v", webhook.ID, err)
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		data.Payload = inspectBody("payload", "Payload", webhook.Payload)
		setInspectNavigation(&data, r)
		if webhook.Headers != "" {
			if err := json.Unmarshal([]byte(webhook.Headers), &data.Headers); err != nil {
				data.HeadersError = true
				log.Printf("decode webhook %d headers: %v", webhook.ID, err)
			}
		}
		renderInspect(w, "base", data)
	}
}

func InspectDeliveryPartial(db webhookInspectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		webhook, ok := findInspectWebhook(w, r, db)
		if !ok {
			return
		}
		data, err := inspectDeliveryData(db, webhook, CSRFToken(r))
		if err != nil {
			log.Printf("load webhook %d delivery attempts: %v", webhook.ID, err)
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		setInspectNavigation(&data, r)
		renderInspect(w, "inspect-delivery", data)
	}
}

func setInspectNavigation(data *InspectWebhookData, r *http.Request) {
	data.ReturnURL = "/dashboard"
	target := r.URL.Query().Get("return_to")
	if isLocalRedirect(target) {
		parsed, err := url.Parse(target)
		if err == nil && parsed.Fragment == "" && (parsed.Path == "/dashboard" || parsed.Path == "/dlq") {
			data.ReturnURL = parsed.RequestURI()
		}
	}
	data.InspectURL = fmt.Sprintf("/webhooks/%d?return_to=%s", data.Webhook.ID, url.QueryEscape(data.ReturnURL))
}

func DownloadWebhookPayload(db webhookInspectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		webhook, ok := findInspectWebhook(w, r, db)
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="webhook-%d-payload.bin"`, webhook.ID))
		w.Header().Set("Content-Length", strconv.Itoa(len(webhook.Payload)))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if _, err := w.Write(webhook.Payload); err != nil {
			log.Printf("download webhook %d payload: %v", webhook.ID, err)
		}
	}
}

func findInspectWebhook(w http.ResponseWriter, r *http.Request, db webhookInspectStore) (model.Webhook, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return model.Webhook{}, false
	}
	webhook, err := db.FindByID(id)
	if errors.Is(err, storage.ErrNotFound) {
		http.NotFound(w, r)
		return model.Webhook{}, false
	}
	if err != nil {
		log.Printf("load webhook %d: %v", id, err)
		http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
		return model.Webhook{}, false
	}
	return webhook, true
}

func inspectDeliveryData(db webhookInspectStore, webhook model.Webhook, csrfToken string) (InspectWebhookData, error) {
	attempts, err := db.DeliveryAttemptsByWebhook(webhook.ID)
	if err != nil {
		return InspectWebhookData{}, err
	}
	data := InspectWebhookData{
		Webhook: webhook, Attempts: attempts, CSRFToken: csrfToken,
		Response: inspectBody("response", "Latest saved response", webhook.Response),
	}
	if len(webhook.Response) > 0 {
		for _, attempt := range attempts {
			if attempt.CompletedAt != nil && attempt.Status != "skipped" {
				data.Response.Truncated = attempt.ResponseTruncated
				break
			}
		}
	}
	return data, nil
}

func inspectBody(id, label string, body []byte) InspectBody {
	view := InspectBody{ID: id, Label: label, Size: len(body)}
	view.Binary = !utf8.Valid(body)
	for _, b := range body {
		if b < 32 && b != '\n' && b != '\r' && b != '\t' || b == 127 {
			view.Binary = true
			break
		}
	}
	if view.Binary {
		view.Raw = hex.Dump(body[:min(len(body), 256)])
		return view
	}
	view.Raw = string(body)
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, body, "", "  "); err == nil {
		view.JSON = true
		view.Formatted = formatted.String()
	}
	return view
}

func renderInspect(w http.ResponseWriter, name string, data InspectWebhookData) {
	tmpl := inspectTemplates
	if data.Attempt != nil {
		tmpl = attemptTemplates
	}
	var output bytes.Buffer
	if err := tmpl.ExecuteTemplate(&output, name, data); err != nil {
		log.Printf("render inspect template: %v", err)
		http.Error(w, "Template render failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(output.Bytes()); err != nil {
		log.Printf("write inspect page: %v", err)
	}
}
