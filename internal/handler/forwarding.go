package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"webhookhub/internal/forwarder"
	"webhookhub/internal/hmacsig"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

type ForwardingPageData struct {
	Rules     []model.ForwardingRule
	CSRFToken string
}

type EditForwardingData struct {
	Rule      model.ForwardingRule
	CSRFToken string
}

func ForwardingUI(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rules, err := db.GetForwardingRules()
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := forwardingTemplates.ExecuteTemplate(w, "base", ForwardingPageData{Rules: rules, CSRFToken: CSRFToken(r)}); err != nil {
			http.Error(w, "Template render failed", http.StatusInternalServerError)
		}
	}
}

func SaveForwardingRule(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}

		rule, err := parseForwardingRuleForm(r, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := db.SaveForwardingRule(rule); err != nil {
			http.Error(w, "Failed to save forwarding rule", http.StatusServiceUnavailable)
			return
		}

		http.Redirect(w, r, "/forwarding", http.StatusSeeOther)
	}
}

func EditForwardingForm(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := r.URL.Query().Get("source")
		if source == "" {
			http.Error(w, "Missing source", http.StatusBadRequest)
			return
		}

		rule, err := db.GetForwardingRule(r.Context(), source)
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "Rule not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}

		if err := editFormTemplates.Execute(w, EditForwardingData{Rule: rule, CSRFToken: CSRFToken(r)}); err != nil {
			http.Error(w, "Template render failed", http.StatusInternalServerError)
		}
	}
}

func UpdateForwardingRule(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}

		source := strings.TrimSpace(r.FormValue("source"))
		existing, err := db.GetForwardingRule(r.Context(), source)
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "Rule not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}

		rule, err := parseForwardingRuleForm(r, &existing)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := db.SaveForwardingRule(rule); err != nil {
			http.Error(w, "Failed to update forwarding rule", http.StatusServiceUnavailable)
			return
		}

		http.Redirect(w, r, "/forwarding", http.StatusSeeOther)
	}
}

func DeleteForwardingRule(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}

		source := r.FormValue("source")
		if source != "" {
			if err := db.DeleteForwardingRule(source); err != nil {
				http.Error(w, "Failed to delete forwarding rule", http.StatusServiceUnavailable)
				return
			}
		}

		http.Redirect(w, r, "/forwarding", http.StatusSeeOther)
	}
}

func parseForwardingRuleForm(r *http.Request, existing *model.ForwardingRule) (model.ForwardingRule, error) {
	source := strings.TrimSpace(r.FormValue("source"))
	target := strings.TrimSpace(r.FormValue("target"))
	if source == "" || target == "" {
		return model.ForwardingRule{}, errors.New("source and target are required")
	}
	if !validSource(source) {
		return model.ForwardingRule{}, errors.New("source must be 1-64 letters, numbers, dots, underscores, or dashes")
	}
	if err := validateTargetURL(target); err != nil {
		return model.ForwardingRule{}, err
	}

	rule := model.ForwardingRule{
		Source: source,
		Target: target,
	}
	if existing != nil {
		rule = *existing
		rule.Source = source
		rule.Target = target
	}

	verifySecret := strings.TrimSpace(r.FormValue("verify_secret"))
	verifyHeader := strings.TrimSpace(r.FormValue("verify_header"))
	toleranceRaw := strings.TrimSpace(r.FormValue("tolerance"))
	clearVerify := r.FormValue("clear_verify_secret") != ""

	switch {
	case clearVerify:
		rule.VerifySecret = ""
		rule.VerifyHeader = ""
		rule.ToleranceSeconds = 0
	case verifySecret != "":
		tolerance, err := parseTolerance(toleranceRaw, hmacsig.DefaultToleranceSeconds)
		if err != nil {
			return model.ForwardingRule{}, err
		}
		rule.VerifySecret = verifySecret
		rule.VerifyHeader = normalizeVerifyHeader(verifyHeader, hmacsig.DefaultIncomingHeader)
		rule.ToleranceSeconds = tolerance
	case existing != nil && existing.VerifySecret != "":
		tolerance, err := parseTolerance(toleranceRaw, existing.ToleranceSeconds)
		if err != nil {
			return model.ForwardingRule{}, err
		}
		rule.VerifySecret = existing.VerifySecret
		rule.VerifyHeader = normalizeVerifyHeader(verifyHeader, normalizeVerifyHeader(existing.VerifyHeader, hmacsig.DefaultIncomingHeader))
		rule.ToleranceSeconds = tolerance
	default:
		rule.VerifySecret = ""
		rule.VerifyHeader = ""
		rule.ToleranceSeconds = 0
	}

	outgoingSecret := strings.TrimSpace(r.FormValue("outgoing_secret"))
	clearOutgoing := r.FormValue("clear_outgoing_secret") != ""
	retryMaxAttempts, err := parsePositiveIntWithDefault(strings.TrimSpace(r.FormValue("retry_max_attempts")), forwarder.DefaultMaxAttempts, "retry max attempts must be a positive integer")
	if err != nil {
		return model.ForwardingRule{}, err
	}
	retryBackoffSeconds, err := parsePositiveIntWithDefault(strings.TrimSpace(r.FormValue("retry_backoff_seconds")), forwarder.DefaultBackoffSeconds, "retry backoff seconds must be a positive integer")
	if err != nil {
		return model.ForwardingRule{}, err
	}

	switch {
	case clearOutgoing:
		rule.OutgoingSecret = ""
	case outgoingSecret != "":
		rule.OutgoingSecret = outgoingSecret
	case existing == nil:
		rule.OutgoingSecret = ""
	}

	rule.RetryMaxAttempts = retryMaxAttempts
	rule.RetryBackoffSeconds = retryBackoffSeconds

	return rule, nil
}

func validateTargetURL(target string) error {
	if len(target) > 2048 {
		return errors.New("target URL is too long")
	}
	parsed, err := url.ParseRequestURI(target)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("target must be an absolute http or https URL")
	}
	if parsed.User != nil {
		return errors.New("target URL must not contain credentials")
	}
	return nil
}

func parseTolerance(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, errors.New("tolerance must be a non-negative integer")
	}
	return value, nil
}

func normalizeVerifyHeader(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func parsePositiveIntWithDefault(raw string, fallback int, message string) (int, error) {
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, errors.New(message)
	}
	return value, nil
}
