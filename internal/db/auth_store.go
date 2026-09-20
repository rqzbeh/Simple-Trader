package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// AdminUser represents an authorized administrator record.
type AdminUser struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	Role         string     `json:"role"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

// EncryptedSecret represents an encrypted key-value record.
type EncryptedSecret struct {
	KeyName          string    `json:"key_name"`
	EncryptedPayload string    `json:"encrypted_payload"`
	Nonce            string    `json:"nonce"`
	UpdatedAt        time.Time `json:"updated_at"`
}

var (
	ErrUserNotFound   = errors.New("admin user not found")
	ErrSecretNotFound = errors.New("secret not found")
)

// GetAdminUserByUsername retrieves an admin user by their unique username.
func (s *Store) GetAdminUserByUsername(ctx context.Context, username string) (*AdminUser, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}

	var u AdminUser
	row := s.Pool.QueryRow(ctx, `
		SELECT id, username, password_hash, role, created_at, last_login_at
		FROM admin_users
		WHERE username = $1
	`, username)

	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.LastLoginAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query admin user: %w", err)
	}

	return &u, nil
}

// UpsertAdminUser inserts or updates an admin user with a bcrypt password hash.
func (s *Store) UpsertAdminUser(ctx context.Context, username, passwordHash, role string) (*AdminUser, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}

	var u AdminUser
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO admin_users (username, password_hash, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (username) DO UPDATE
		SET password_hash = EXCLUDED.password_hash,
		    role = EXCLUDED.role
		RETURNING id, username, password_hash, role, created_at, last_login_at
	`, username, passwordHash, role)

	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.LastLoginAt)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert admin user: %w", err)
	}

	return &u, nil
}

// UpdateLastLogin updates the last login timestamp for an admin user.
func (s *Store) UpdateLastLogin(ctx context.Context, userID string) error {
	if s.Pool == nil {
		return errors.New("database pool not initialized")
	}

	_, err := s.Pool.Exec(ctx, `
		UPDATE admin_users
		SET last_login_at = NOW()
		WHERE id = $1
	`, userID)
	return err
}

// GetSecret retrieves an encrypted system secret by key name.
func (s *Store) GetSecret(ctx context.Context, keyName string) (*EncryptedSecret, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}

	var sec EncryptedSecret
	row := s.Pool.QueryRow(ctx, `
		SELECT key_name, encrypted_payload, nonce, updated_at
		FROM encrypted_system_secrets
		WHERE key_name = $1
	`, keyName)

	err := row.Scan(&sec.KeyName, &sec.EncryptedPayload, &sec.Nonce, &sec.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSecretNotFound
		}
		return nil, fmt.Errorf("failed to get encrypted secret: %w", err)
	}

	return &sec, nil
}

// SetSecret stores or updates an encrypted system secret.
func (s *Store) SetSecret(ctx context.Context, keyName, encryptedPayload, nonce string) error {
	if s.Pool == nil {
		return errors.New("database pool not initialized")
	}

	_, err := s.Pool.Exec(ctx, `
		INSERT INTO encrypted_system_secrets (key_name, encrypted_payload, nonce, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (key_name) DO UPDATE
		SET encrypted_payload = EXCLUDED.encrypted_payload,
		    nonce = EXCLUDED.nonce,
		    updated_at = NOW()
	`, keyName, encryptedPayload, nonce)
	if err != nil {
		return fmt.Errorf("failed to set encrypted secret: %w", err)
	}

	return nil
}
