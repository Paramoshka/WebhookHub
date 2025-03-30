package handler

import (
	"html/template"
	"net/http"
	"os"
)

func LoginPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl := template.Must(template.ParseFiles("web/templates/login.html"))
		tmpl.Execute(w, nil)
	}
}

func Login() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid login form", http.StatusBadRequest)
			return
		}

		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == os.Getenv("ADMIN_USER") && password == os.Getenv("ADMIN_PASS") {
			http.SetCookie(w, &http.Cookie{
				Name:  "session",
				Value: "authenticated",
				Path:  "/",
			})
			http.Redirect(w, r, "/", http.StatusSeeOther)
		} else {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
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
		if err != nil || cookie.Value != "authenticated" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}
