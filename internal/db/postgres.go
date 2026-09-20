package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RunMigrations applies SQL migration files to the database in order.
func (s *Store) RunMigrations(ctx context.Context, migrationsDir string) error {
	if s.Pool == nil {
		return fmt.Errorf("database pool is not initialized")
	}

	// Create schema migrations table if not exists
	_, err := s.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(128) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to ensure schema_migrations table: %w", err)
	}

	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations dir: %w", err)
	}

	var upFiles []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".up.sql") {
			upFiles = append(upFiles, f.Name())
		}
	}
	sort.Strings(upFiles)

	for _, fileName := range upFiles {
		version := strings.TrimSuffix(fileName, ".up.sql")

		var exists bool
		err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check migration status for %s: %w", version, err)
		}

		if exists {
			continue
		}

		filePath := filepath.Join(migrationsDir, fileName)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", filePath, err)
		}

		log.Printf("[MIGRATE] Applying migration %s...", version)
		_, err = s.Pool.Exec(ctx, string(content))
		if err != nil {
			return fmt.Errorf("migration %s failed: %w", version, err)
		}

		_, err = s.Pool.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version)
		if err != nil {
			return fmt.Errorf("failed to record migration %s: %w", version, err)
		}
		log.Printf("[MIGRATE] Successfully applied %s", version)
	}

	return nil
}
