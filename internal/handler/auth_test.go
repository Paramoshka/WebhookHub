package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"webhookhub/internal/model"
	"webhookhub/internal/storage"

	"golang.org/x/crypto/bcrypt"
)

type testUserStore struct {
	user  model.User
	err   error
	email string
}

func (s *testUserStore) FindUserByEmail(_ context.Context, email string) (model.User, error) {
	s.email = email
	return s.user, s.err
}

// loginFormStep performs the GET /login step and returns the CSRF cookie and token
// exactly as a browser would receive them.
func loginFormStep(t *testing.T, auth *Auth) (*http.Cookie, string) {
	t.Helper()
	response := httptest.NewRecorder()
	auth.Login(nil)(response, httptest.NewRequest(http.MethodGet, "/login", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("login form: expected 200, got %d", response.Code)
	}
	var cookie *http.Cookie
	for _, c := range response.Result().Cookies() {
		if c.Name == "login_csrf" {
			cookie = c
		}
	}
	if cookie == nil || cookie.Value == "" {
		t.Fatal("login form did not set the login_csrf cookie")
	}
	_, rest, ok := strings.Cut(response.Body.String(), `name="csrf_token" value="`)
	if !ok {
		t.Fatal("login form has no csrf_token field")
	}
	token, _, ok := strings.Cut(rest, `"`)
	if !ok || token == "" {
		t.Fatal("login form csrf_token is empty")
	}
	return cookie, token
}

func loginPost(auth *Auth, store loginUserStore, cookie *http.Cookie, token, username, password string) *httptest.ResponseRecorder {
	form := url.Values{"username": {username}, "password": {password}, "csrf_token": {token}}
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	auth.Login(store)(response, request)
	return response
}

func sessionCookie(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range response.Result().Cookies() {
		if c.Name == "session" && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestLoginIssuesUsableSession(t *testing.T) {
	auth, err := NewAuth(strings.Repeat("a", 32), false, false)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	store := &testUserStore{user: model.User{Email: "admin@example.com", Password: string(hash), IsAdmin: true}}
	cookie, token := loginFormStep(t, auth)

	response := loginPost(auth, store, cookie, token, "admin@example.com", "correct-password")

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/" {
		t.Fatalf("expected redirect to /, got %d %q", response.Code, response.Header().Get("Location"))
	}
	if store.email != "admin@example.com" {
		t.Fatalf("expected lookup for admin@example.com, got %q", store.email)
	}
	session := sessionCookie(t, response)
	if session == nil {
		t.Fatal("successful login did not issue a session cookie")
	}
	if !session.HttpOnly || session.Path != "/" {
		t.Fatalf("session cookie missing HttpOnly or root path: %+v", session)
	}
	for _, c := range response.Result().Cookies() {
		if c.Name == "login_csrf" && (c.Value != "" || c.MaxAge >= 0) {
			t.Fatalf("login_csrf cookie was not cleared: %+v", c)
		}
	}

	// The issued session must authenticate a subsequent protected request.
	protected := auth.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		if CSRFToken(r) == "" {
			t.Error("session carries no CSRF token")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	request.AddCookie(session)
	protectedResponse := httptest.NewRecorder()
	protected(protectedResponse, request)
	if protectedResponse.Code != http.StatusNoContent {
		t.Fatalf("issued session was not accepted: %d", protectedResponse.Code)
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	auth, err := NewAuth(strings.Repeat("a", 32), false, false)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		store    *testUserStore
		password string
	}{
		{name: "unknown user", store: &testUserStore{err: storage.ErrNotFound}, password: "correct-password"},
		{name: "wrong password", store: &testUserStore{user: model.User{Email: "admin@example.com", Password: string(hash)}}, password: "wrong-password"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cookie, token := loginFormStep(t, auth)
			response := loginPost(auth, test.store, cookie, token, "admin@example.com", test.password)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", response.Code)
			}
			if sessionCookie(t, response) != nil {
				t.Fatal("failed login issued a session")
			}
		})
	}
}

func TestLoginMapsStorageErrors(t *testing.T) {
	auth, err := NewAuth(strings.Repeat("a", 32), false, false)
	if err != nil {
		t.Fatal(err)
	}
	store := &testUserStore{err: errors.New("down")}
	cookie, token := loginFormStep(t, auth)

	response := loginPost(auth, store, cookie, token, "admin@example.com", "correct-password")

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}
	if sessionCookie(t, response) != nil {
		t.Fatal("storage failure issued a session")
	}
}
