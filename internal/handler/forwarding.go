package handler

import (
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"webhookhub/internal/hmacsig"
	"webhookhub/internal/model"
	"webhookhub/internal/storage"
)

func ForwardingUI(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rules := db.GetForwardingRules()
		tmpl := template.Must(template.ParseFiles(
			"web/templates/base.html",
			"web/templates/forwarding.html",
		))
		if err := tmpl.ExecuteTemplate(w, "base", rules); err != nil {
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
		db.SaveForwardingRule(rule)

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

		rule, found := db.GetForwardingRule(source)
		if !found {
			http.Error(w, "Rule not found", http.StatusNotFound)
			return
		}

		tmpl := template.Must(template.ParseFiles("web/templates/edit_form.html"))
		if err := tmpl.Execute(w, rule); err != nil {
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
		existing, found := db.GetForwardingRule(source)
		if !found {
			http.Error(w, "Rule not found", http.StatusNotFound)
			return
		}

		rule, err := parseForwardingRuleForm(r, &existing)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		db.SaveForwardingRule(rule)

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
			db.DeleteForwardingRule(source)
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

	switch {
	case clearOutgoing:
		rule.OutgoingSecret = ""
	case outgoingSecret != "":
		rule.OutgoingSecret = outgoingSecret
	case existing == nil:
		rule.OutgoingSecret = ""
	}

	return rule, nil
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
