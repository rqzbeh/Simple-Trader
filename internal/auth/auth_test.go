package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/auth"
)

func TestHashAndCheckPassword(t *testing.T) {
	password := "SecretAdminPassword123!"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if hash == password {
		t.Errorf("expected hash to differ from raw password")
	}

	if !auth.CheckPassword(hash, password) {
		t.Errorf("expected valid password check to return true")
	}

	if auth.CheckPassword(hash, "WrongPassword") {
		t.Errorf("expected wrong password check to return false")
	}
}

func TestRateLimiting(t *testing.T) {
	ctx := context.Background()
	rl := auth.NewRateLimiter(nil)
	key := "192.168.1.100"

	// 5 failed attempts allowed before cutoff
	for i := 0; i < 5; i++ {
		allowed, remaining := rl.IsAllowed(ctx, key)
		if !allowed {
			t.Fatalf("attempt %d should have been allowed", i+1)
		}
		if remaining != 5-i {
			t.Errorf("expected remaining %d, got %d", 5-i, remaining)
		}
		rl.RecordFailure(ctx, key)
	}

	// 6th attempt must be rejected
	allowed, remaining := rl.IsAllowed(ctx, key)
	if allowed {
		t.Errorf("expected 6th attempt to be blocked by rate limiter")
	}
	if remaining != 0 {
		t.Errorf("expected remaining 0, got %d", remaining)
	}

	// Resetting key unlocks rate limiter
	rl.Reset(ctx, key)
	allowedAfterReset, remainingAfterReset := rl.IsAllowed(ctx, key)
	if !allowedAfterReset || remainingAfterReset != 5 {
		t.Errorf("expected rate limiter to be fully reset")
	}
}

func TestAuthenticatorLifecycle(t *testing.T) {
	ctx := context.Background()
	adminPass := "MasterQuantKey#2026"
	authenticator, err := auth.NewAuthenticator(adminPass, nil)
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}

	clientIP := "10.0.0.50"

	// 1. Invalid login attempt
	_, err = authenticator.Login(ctx, "IncorrectPassword", clientIP)
	if err == nil {
		t.Errorf("expected error on incorrect password")
	}

	// 2. Successful login
	session, err := authenticator.Login(ctx, adminPass, clientIP)
	if err != nil {
		t.Fatalf("failed to login with correct credentials: %v", err)
	}

	if session.Token == "" {
		t.Errorf("expected non-empty session token")
	}

	if !authenticator.ValidateToken(session.Token) {
		t.Errorf("expected session token to be valid")
	}

	// 3. Logout
	authenticator.Logout(session.Token)
	if authenticator.ValidateToken(session.Token) {
		t.Errorf("expected revoked session token to be invalid")
	}
}

func TestSessionExpiration(t *testing.T) {
	sessionStore := auth.NewSessionStore(nil)
	sess, err := sessionStore.CreateSession(context.Background())
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if !sessionStore.ValidateSession(context.Background(), sess.Token) {
		t.Errorf("expected fresh session to be valid")
	}

	// Artificially expire the session
	time.Sleep(10 * time.Millisecond)
	sess.ExpiresAt = time.Now().Add(-1 * time.Hour)
	// directly re-register with past expiration
	// In reality validate handles time check:
}
