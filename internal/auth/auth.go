package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultBcryptCost is cost 12 as requested in institutional security requirements.
	DefaultBcryptCost = 12
	// SessionDuration is the active lifetime of an administrative session.
	SessionDuration = 24 * time.Hour
)

var (
	ErrInvalidCredentials = errors.New("invalid administrative credentials")
	ErrSessionExpired     = errors.New("session token expired or invalid")
	ErrRateLimited        = errors.New("too many failed login attempts, please retry later")
)

// HashPassword hashes a plain text password with salted bcrypt at cost 12.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), DefaultBcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(bytes), nil
}

// CheckPassword securely verifies a password against a bcrypt hash.
func CheckPassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// ConstantTimeCompare compares two string tokens in constant time to prevent timing attacks.
func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// GenerateSessionToken creates a cryptographically secure 32-byte hex-encoded session token.
func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random session token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Session represents an authenticated administrator session.
type Session struct {
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SessionStore handles session persistence with Redis and in-memory fallback.
type SessionStore struct {
	redisClient *cache.Client
	mu          sync.RWMutex
	inMemory    map[string]Session
}

// NewSessionStore initializes the session store.
func NewSessionStore(redisClient *cache.Client) *SessionStore {
	return &SessionStore{
		redisClient: redisClient,
		inMemory:    make(map[string]Session),
	}
}

// CreateSession generates, registers, and returns a new authenticated session.
func (s *SessionStore) CreateSession(ctx context.Context) (*Session, error) {
	token, err := GenerateSessionToken()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	sess := Session{
		Token:     token,
		CreatedAt: now,
		ExpiresAt: now.Add(SessionDuration),
	}

	// Persist to in-memory store
	s.mu.Lock()
	s.inMemory[token] = sess
	s.mu.Unlock()

	// Persist to Redis if available
	if s.redisClient != nil && s.redisClient.Underlying() != nil {
		redisKey := fmt.Sprintf("session:%s", token)
		_ = s.redisClient.Underlying().Set(ctx, redisKey, now.Format(time.RFC3339), SessionDuration).Err()
	}

	return &sess, nil
}

// ValidateSession verifies if a session token is active and unexpired.
func (s *SessionStore) ValidateSession(ctx context.Context, token string) bool {
	if token == "" {
		return false
	}

	// 1. Check Redis if available
	if s.redisClient != nil && s.redisClient.Underlying() != nil {
		redisKey := fmt.Sprintf("session:%s", token)
		val, err := s.redisClient.Underlying().Get(ctx, redisKey).Result()
		if err == nil && val != "" {
			return true
		}
	}

	// 2. Fallback to in-memory store
	s.mu.RLock()
	sess, exists := s.inMemory[token]
	s.mu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().After(sess.ExpiresAt) {
		s.RevokeSession(ctx, token)
		return false
	}

	return true
}

// RevokeSession invalidates and removes a session token.
func (s *SessionStore) RevokeSession(ctx context.Context, token string) {
	s.mu.Lock()
	delete(s.inMemory, token)
	s.mu.Unlock()

	if s.redisClient != nil && s.redisClient.Underlying() != nil {
		redisKey := fmt.Sprintf("session:%s", token)
		_ = s.redisClient.Underlying().Del(ctx, redisKey).Err()
	}
}

// Authenticator wraps credential checking, rate-limiting, and session management.
type Authenticator struct {
	adminPasswordHash string
	configured        bool
	sessions          *SessionStore
	limiter           *RateLimiter
}

// NewAuthenticator creates an Authenticator instance.
func NewAuthenticator(adminPassword string, redisClient *cache.Client) (*Authenticator, error) {
	if adminPassword == "" {
		return &Authenticator{
			sessions: NewSessionStore(redisClient),
			limiter:  NewRateLimiter(redisClient),
		}, nil
	}

	hash, err := HashPassword(adminPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to hash admin password: %w", err)
	}

	return &Authenticator{
		adminPasswordHash: hash,
		configured:        true,
		sessions:          NewSessionStore(redisClient),
		limiter:           NewRateLimiter(redisClient),
	}, nil
}

// IsConfigured returns true if an admin password was explicitly configured.
func (a *Authenticator) IsConfigured() bool {
	return a != nil && a.configured
}

// Login validates password, enforces sliding window rate limit, and issues session token.
func (a *Authenticator) Login(ctx context.Context, password, clientIP string) (*Session, error) {
	if !a.IsConfigured() {
		return nil, errors.New("admin password is not configured on this server")
	}

	allowed, remaining := a.limiter.IsAllowed(ctx, clientIP)
	if !allowed {
		return nil, ErrRateLimited
	}

	// Match against bcrypt hash only; raw password is never retained.
	if !CheckPassword(a.adminPasswordHash, password) {
		a.limiter.RecordFailure(ctx, clientIP)
		return nil, fmt.Errorf("%w (remaining attempts before lockout: %d)", ErrInvalidCredentials, remaining-1)
	}

	// Success: reset failures and generate session
	a.limiter.Reset(ctx, clientIP)
	return a.sessions.CreateSession(ctx)
}

// ValidateToken verifies if the given token is an active valid session.
func (a *Authenticator) ValidateToken(token string) bool {
	return a.sessions.ValidateSession(context.Background(), token)
}

// Logout revokes the session token.
func (a *Authenticator) Logout(token string) {
	a.sessions.RevokeSession(context.Background(), token)
}

// Limiter returns the rate limiter.
func (a *Authenticator) Limiter() *RateLimiter {
	return a.limiter
}

// Sessions returns the session store.
func (a *Authenticator) Sessions() *SessionStore {
	return a.sessions
}
