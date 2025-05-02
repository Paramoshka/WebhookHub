package handler

import (
	"html/template"
	"net/http"
	"os"
	"webhookhub/internal/storage"

	"github.com/gorilla/securecookie"
	"golang.org/x/crypto/bcrypt"
)

var s = securecookie.New(
	[]byte(os.Getenv("SESSION_KEY")), // hash key
	nil,                              // block key
)

func Login(db *storage.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			tmpl := template.Must(template.ParseFiles("web/templates/login.html"))
			tmpl.Execute(w, nil)
			return
		}

		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			email := r.FormValue("username")
			password := r.FormValue("password")

			user, ok := db.FindUserByEmail(email)
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			encoded, _ := s.Encode("session", map[string]string{
				"user": email,
			})

			http.SetCookie(w, &http.Cookie{
				Name:     "session",
				Value:    encoded,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})

			http.Redirect(w, r, "/", http.StatusSeeOther)
		}
	}
}

func Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:   "session",
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		session := make(map[string]string)
		if err := s.Decode("session", cookie.Value, &session); err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		if session["user"] == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		next(w, r)
	}
}
