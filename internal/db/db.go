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
	host := "postgres"
	if config.ConnConfig != nil && config.ConnConfig.Host != "" {
		host = config.ConnConfig.Host
	}

	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 30 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second

	// Bound every layer of connection setup. Without these an unreachable or
	// blackholed Postgres silently hangs startup and every later query instead
	// of failing fast with a clear error.
	if config.ConnConfig != nil {
		config.ConnConfig.ConnectTimeout = 5 * time.Second
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres pool: %w", err)
	}

	// Verify connectivity for real: NewWithConfig never dials, so a pool that
	// looks constructed can still be unreachable.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to reach postgres at %s: %w", host, err)
	}

	return &Store{Pool: pool}, nil
}

// Close closes the underlying pool.
func (s *Store) Close() {
	if s.Pool != nil {
		s.Pool.Close()
	}
}
