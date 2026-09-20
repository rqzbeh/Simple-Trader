package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/rqzbeh/simple-trader/internal/auth"
)

// LoginRequest defines payload for admin authentication.
type LoginRequest struct {
	Password string `json:"password"`
}

// LoginResponse returns session metadata upon successful authentication.
type LoginResponse struct {
	Status    string    `json:"status"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SessionStatusResponse returns the current authentication state.
type SessionStatusResponse struct {
	Authenticated bool   `json:"authenticated"`
	TokenMasked   string `json:"token_masked,omitempty"`
}

// LoginHandler processes password verification and issues session tokens.
func (s *Server) LoginHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.authenticator == nil {
		http.Error(w, `{"error":"authentication service unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		http.Error(w, `{"error":"password is required"}`, http.StatusBadRequest)
		return
	}

	clientIP := r.RemoteAddr
	session, err := s.authenticator.Login(r.Context(), req.Password, clientIP)
	if err != nil {
		if errors.Is(err, auth.ErrRateLimited) {
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{
				"error": err.Error(),
			})
			return
		}

		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "simple_trader_session",
		Value:    session.Token,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.IsProduction,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(LoginResponse{
		Status:    "authenticated",
		Token:     session.Token,
		ExpiresAt: session.ExpiresAt,
	})
}

// LogoutHandler terminates an active session and clears the cookie.
func (s *Server) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token := auth.ExtractToken(r)
	if token != "" && s.authenticator != nil {
		s.authenticator.Logout(token)
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "simple_trader_session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.IsProduction,
	})

	json.NewEncoder(w).Encode(map[string]string{
		"status": "logged_out",
	})
}

// SessionHandler checks if the requester holds an active valid session.
func (s *Server) SessionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token := auth.ExtractToken(r)
	if token == "" || s.authenticator == nil || !s.authenticator.ValidateToken(token) {
		json.NewEncoder(w).Encode(SessionStatusResponse{
			Authenticated: false,
		})
		return
	}

	masked := token
	if len(token) > 8 {
		masked = token[:4] + "..." + token[len(token)-4:]
	}

	json.NewEncoder(w).Encode(SessionStatusResponse{
		Authenticated: true,
		TokenMasked:   masked,
	})
}
