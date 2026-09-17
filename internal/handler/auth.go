package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"webhookhub/internal/storage"

	"github.com/gorilla/securecookie"
	"golang.org/x/crypto/bcrypt"
)

const sessionMaxAgeSeconds = 12 * 60 * 60
const loginCSRFMaxAgeSeconds = 10 * 60

type authContextKey string

const csrfContextKey authContextKey = "csrf_token"

// dummyPasswordHash is compared against when the email is unknown so that
// response timing does not reveal whether an account exists.
var dummyPasswordHash = []byte("$2a$10$l6AgED.zKbzavi0YpUsLou4kEK/KTfH4Bm/CX5mwI/cuTLX9P5LbW")

type Auth struct {
	cookies           *securecookie.SecureCookie
	loginCookies      *securecookie.SecureCookie
	secure            bool
	trustProxyHeaders bool
	loginLimiter      *loginLimiter
}

type sessionData struct {
	User      string
	CSRFToken string
}

type LoginPageData struct {
	CSRFToken string
}

func NewAuth(sessionKey string, secure, trustProxyHeaders bool) (*Auth, error) {
	if len(sessionKey) < 32 {
		return nil, errors.New("SESSION_KEY must contain at least 32 characters")
	}

	cookies := securecookie.New([]byte(sessionKey), nil)
	cookies.MaxAge(sessionMaxAgeSeconds)
	loginCookies := securecookie.New([]byte(sessionKey), nil)
	loginCookies.MaxAge(loginCSRFMaxAgeSeconds)

	return &Auth{
		cookies:           cookies,
		loginCookies:      loginCookies,
		secure:            secure,
		trustProxyHeaders: trustProxyHeaders,
		loginLimiter:      newLoginLimiter(),
	}, nil
}

func (a *Auth) Login(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			csrfToken, err := randomToken()
			if err != nil {
				http.Error(w, "Failed to create login form", http.StatusInternalServerError)
				return
			}
			encodedToken, err := a.loginCookies.Encode("login_csrf", csrfToken)
			if err != nil {
				http.Error(w, "Failed to create login form", http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     "login_csrf",
				Value:    encodedToken,
				Path:     "/login",
				MaxAge:   loginCSRFMaxAgeSeconds,
				HttpOnly: true,
				Secure:   a.secure,
				SameSite: http.SameSiteStrictMode,
			})

			if err := loginTemplates.Execute(w, LoginPageData{CSRFToken: csrfToken}); err != nil {
				http.Error(w, "Template render failed", http.StatusInternalServerError)
			}
			return
		}

		clientKey := clientIP(r, a.trustProxyHeaders)
		if !a.loginLimiter.allow(clientKey) {
			w.Header().Set("Retry-After", strconv.Itoa(int(loginWindow.Seconds())))
			http.Error(w, "Too many failed login attempts", http.StatusTooManyRequests)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		if !a.validLoginCSRF(r) {
			http.Error(w, "Invalid CSRF token", http.StatusForbidden)
			return
		}

		email := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		user, err := db.FindUserByEmail(email)
		switch {
		case errors.Is(err, storage.ErrNotFound):
			_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
			a.loginLimiter.fail(clientKey)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		case err != nil:
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
			a.loginLimiter.fail(clientKey)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		a.loginLimiter.reset(clientKey)

		csrfToken, err := randomToken()
		if err != nil {
			http.Error(w, "Failed to create session", http.StatusInternalServerError)
			return
		}
		encoded, err := a.cookies.Encode("session", sessionData{
			User:      email,
			CSRFToken: csrfToken,
		})
		if err != nil {
			http.Error(w, "Failed to create session", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    encoded,
			Path:     "/",
			MaxAge:   sessionMaxAgeSeconds,
			HttpOnly: true,
			Secure:   a.secure,
			SameSite: http.SameSiteStrictMode,
		})
		http.SetCookie(w, &http.Cookie{
			Name:     "login_csrf",
			Value:    "",
			Path:     "/login",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   a.secure,
			SameSite: http.SameSiteStrictMode,
		})

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (a *Auth) validLoginCSRF(r *http.Request) bool {
	cookie, err := r.Cookie("login_csrf")
	if err != nil {
		return false
	}

	var expected string
	if err := a.loginCookies.Decode("login_csrf", cookie.Value, &expected); err != nil {
		return false
	}
	provided := strings.TrimSpace(r.FormValue("csrf_token"))
	return expected != "" && provided != "" && subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}

func (a *Auth) Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   a.secure,
			SameSite: http.SameSiteStrictMode,
		})
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func (a *Auth) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			redirectToLogin(w, r)
			return
		}

		var session sessionData
		if err := a.cookies.Decode("session", cookie.Value, &session); err != nil || session.User == "" || session.CSRFToken == "" {
			redirectToLogin(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), csrfContextKey, session.CSRFToken)
		next(w, r.WithContext(ctx))
	}
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/login")
		http.Error(w, "Session expired. Please sign in again.", http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (a *Auth) RequireCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expected := CSRFToken(r)
		provided := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
		if provided == "" {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Invalid form", http.StatusBadRequest)
				return
			}
			provided = strings.TrimSpace(r.FormValue("csrf_token"))
		}

		if expected == "" || provided == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
			http.Error(w, "Invalid CSRF token", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

func CSRFToken(r *http.Request) string {
	token, _ := r.Context().Value(csrfContextKey).(string)
	return token
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
