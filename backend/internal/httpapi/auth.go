package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"forking-paths/internal/store"
)

const sessionCookie = "forking_session"

func (s *Server) authRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/register", s.register)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/forgot", s.forgot)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/auth/me", s.me)
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Recovery string `json:"recovery"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, &store.ValidationError{Msg: "请求格式不对"})
		return
	}
	user, token, err := s.store.Register(r.Context(), body.Username, body.Password, body.Recovery)
	if err != nil {
		writeErr(w, err)
		return
	}
	setSession(w, token)
	writeJSON(w, http.StatusOK, map[string]string{"username": user.Username})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, &store.ValidationError{Msg: "请求格式不对"})
		return
	}
	user, token, err := s.store.Login(r.Context(), body.Username, body.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	setSession(w, token)
	writeJSON(w, http.StatusOK, map[string]string{"username": user.Username})
}

func (s *Server) forgot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Recovery string `json:"recovery"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, &store.ValidationError{Msg: "请求格式不对"})
		return
	}
	user, token, err := s.store.ResetPassword(r.Context(), body.Username, body.Recovery, body.Password)
	if err != nil {
		writeErr(w, err)
		return
	}
	setSession(w, token)
	writeJSON(w, http.StatusOK, map[string]string{"username": user.Username})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	_ = s.store.Logout(r.Context(), readSession(r))
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, err := s.store.UserByToken(r.Context(), readSession(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	on, err := s.store.GardenPublic(store.WithUser(r.Context(), user.ID))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": user.Username, "public": on})
}

func (s *Server) withUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicAuth(r) {
			next.ServeHTTP(w, r)
			return
		}
		user, err := s.store.UserByToken(r.Context(), readSession(r))
		if err != nil {
			writeErr(w, store.ErrUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(store.WithUser(r.Context(), user.ID)))
	})
}

func publicAuth(r *http.Request) bool {
	if r.URL.Path == "/api/health" || r.URL.Path == "/api/sync/catch" {
		return true
	}
	if r.Method != http.MethodPost && !(r.Method == http.MethodGet && r.URL.Path == "/api/auth/me") {
		return false
	}
	switch r.URL.Path {
	case "/api/auth/register", "/api/auth/login", "/api/auth/forgot", "/api/auth/logout", "/api/auth/me":
		return true
	default:
		return false
	}
}

func readSession(r *http.Request) string {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(c.Value)
}

func setSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
