package config

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultEnvPath returns the path to the environment file.
// It checks the ENV_FILE environment variable, defaulting to "/app/.env".
func DefaultEnvPath() string {
	if p := os.Getenv("ENV_FILE"); p != "" {
		return p
	}
	return "/app/.env"
}

// GetEnvFilePath returns DefaultEnvPath().
func GetEnvFilePath() string {
	return DefaultEnvPath()
}

// UpsertEnv reads an environment file, replaces existing keys or appends new keys
// specified in kv while preserving comments, blank lines, and other keys.
// It performs an atomic write using a temporary file with permissions 0600,
// and falls back to direct in-place write if the target is a Docker bind-mount point (EBUSY).
func UpsertEnv(path string, kv map[string]string) error {
	if path == "" {
		path = DefaultEnvPath()
	}

	var existingLines []string
	seen := make(map[string]bool)

	// If file exists, read existing lines
	fileBytes, err := os.ReadFile(path)
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(fileBytes))
		for scanner.Scan() {
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			// Preserve empty lines and comments
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				existingLines = append(existingLines, line)
				continue
			}

			// Parse KEY=VALUE
			eqIdx := strings.Index(line, "=")
			if eqIdx == -1 {
				existingLines = append(existingLines, line)
				continue
			}

			key := strings.TrimSpace(line[:eqIdx])
			if newVal, ok := kv[key]; ok {
				existingLines = append(existingLines, fmt.Sprintf("%s=%s", key, newVal))
				seen[key] = true
			} else {
				existingLines = append(existingLines, line)
			}
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("envfile: error scanning %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("envfile: error reading %s: %w", path, err)
	}

	// Append any keys from kv that were not in the file
	var toAppend []string
	for k, v := range kv {
		if !seen[k] {
			toAppend = append(toAppend, fmt.Sprintf("%s=%s", k, v))
		}
	}
	sort.Strings(toAppend)
	existingLines = append(existingLines, toAppend...)

	// Construct output buffer
	var buf bytes.Buffer
	for _, l := range existingLines {
		buf.WriteString(l)
		buf.WriteByte('\n')
	}
	content := buf.Bytes()

	// Ensure destination directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("envfile: error creating directory %s: %w", dir, err)
	}

	// Create temp file in same directory for atomic rename
	tmpFile, err := os.CreateTemp(dir, ".env.tmp.*")
	if err != nil {
		// Fallback to system temp directory if dir is not writable for temp files
		tmpFile, err = os.CreateTemp("", ".env.tmp.*")
		if err != nil {
			return fmt.Errorf("envfile: error creating temp file: %w", err)
		}
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		return fmt.Errorf("envfile: error writing temp file: %w", err)
	}
	_ = tmpFile.Chmod(0600)
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("envfile: error closing temp file: %w", err)
	}

	// Attempt atomic rename
	if err := os.Rename(tmpName, path); err != nil {
		// Target might be a Docker bind mount (EBUSY/EXDEV).
		// Fall back to direct write to update the mounted file in place.
		dst, err2 := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0600)
		if err2 != nil {
			return fmt.Errorf("envfile: rename failed (%v) and direct write failed: %w", err, err2)
		}
		if _, err2 := dst.Write(content); err2 != nil {
			dst.Close()
			return fmt.Errorf("envfile: error writing destination file %s: %w", path, err2)
		}
		if err2 := dst.Close(); err2 != nil {
			return fmt.Errorf("envfile: error closing destination file %s: %w", path, err2)
		}
	}

	_ = os.Chmod(path, 0600)
	return nil
}

// ApplyEnvFile hydrates the process environment from the .env file (the
// single source of truth, spec-017). Standard dotenv semantics: an EXISTING
// process value wins (compose-built DATABASE_URL/PORT/REDIS_URL must survive);
// every other key comes from the file. Path: ENV_FILE, else ".env" in the
// working directory. Missing file = no-op.
func ApplyEnvFile() {
	path := os.Getenv("ENV_FILE")
	if path == "" {
		path = ".env"
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue // process env wins
		}
		val = strings.TrimSpace(val)
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		_ = os.Setenv(key, val)
	}
}
