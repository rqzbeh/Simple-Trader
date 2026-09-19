package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ValidateURL checks whether a postgres connection URL string is well-formed.
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url format: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return errors.New("scheme must be postgres or postgresql")
	}
	if u.Host == "" {
		return errors.New("host cannot be empty")
	}
	return nil
}

// Store encapsulates database operations on PostgreSQL using pgxpool.
type Store struct {
	Pool *pgxpool.Pool
}

// NewStore initializes a new pgx connection pool.
func NewStore(ctx context.Context, dbURL string) (*Store, error) {
	if err := ValidateURL(dbURL); err != nil {
		return nil, err
	}

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pgx config: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	return &Store{Pool: pool}, nil
}

// Close closes the underlying pool.
func (s *Store) Close() {
	if s.Pool != nil {
		s.Pool.Close()
	}
}
