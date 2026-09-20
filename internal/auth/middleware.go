package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const (
	// SessionContextKey is the key for authenticated session stored in http context
	SessionContextKey contextKey = "auth_session"
)

// AuthMiddleware returns an HTTP middleware verifying session tokens for protected routes.
func AuthMiddleware(auth *Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ExtractToken(r)

			if token == "" || !auth.ValidateToken(token) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized: valid session token required"}`))
				return
			}

			// Token is valid; proceed with context
			ctx := context.WithValue(r.Context(), SessionContextKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ExtractToken retrieves the session token from Authorization header, Cookie, or URL query param.
func ExtractToken(r *http.Request) string {
	// 1. Check Authorization header: "Bearer <token>"
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}

	// 2. Check Cookie: simple_trader_session
	if cookie, err := r.Cookie("simple_trader_session"); err == nil && cookie.Value != "" {
		return cookie.Value
	}

	// 3. Check Query parameter: token (useful for SSE / EventSource)
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}

	return ""
}
