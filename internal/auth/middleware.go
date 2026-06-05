package auth

import (
	"net/http"
	"strings"
)

type KeyValidator interface {
	ValidateAPIKey(raw string) (scopes []string, ok bool)
}

type Middleware struct {
	validator KeyValidator
}

func NewMiddleware(v KeyValidator) *Middleware {
	return &Middleware{validator: v}
}

func (m *Middleware) RequireScope(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearerToken(r)
		if raw == "" {
			writeUnauthorized(w, "missing bearer token")
			return
		}
		scopes, ok := m.validator.ValidateAPIKey(raw)
		if !ok {
			writeUnauthorized(w, "invalid api key")
			return
		}
		if !hasScope(scopes, scope) {
			writeUnauthorized(w, "insufficient scope")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func AdminToken(expected string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Admin-Token")
		if token == "" {
			token = bearerToken(r)
		}
		if token == "" || token != expected {
			writeUnauthorized(w, "invalid admin token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want || s == "*" {
			return true
		}
	}
	return false
}

func writeUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"message":` + jsonString(msg) + `,"type":"authentication_error"}}`))
}

func jsonString(s string) string {
	b := make([]byte, 0, len(s)+2)
	b = append(b, '"')
	for _, c := range s {
		switch c {
		case '"', '\\':
			b = append(b, '\\', byte(c))
		case '\n':
			b = append(b, '\\', 'n')
		default:
			b = append(b, byte(c))
		}
	}
	b = append(b, '"')
	return string(b)
}
