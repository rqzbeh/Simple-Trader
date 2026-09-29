package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertEnvNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	kv := map[string]string{
		"FOO": "bar",
		"BAZ": "123",
	}

	if err := UpsertEnv(path, kv); err != nil {
		t.Fatalf("UpsertEnv failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "BAZ=123\n") || !strings.Contains(content, "FOO=bar\n") {
		t.Fatalf("unexpected content: %q", content)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 permissions, got %v", info.Mode().Perm())
	}
}

func TestUpsertEnvUpdateAndPreserve(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	initial := `# Configuration Header
PORT=8080
# Database connection
DATABASE_URL=postgres://localhost/db
ROUTING_CONFIDENCE_THRESHOLD=0.75

# Tail comment
`
	if err := os.WriteFile(path, []byte(initial), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	updates := map[string]string{
		"ROUTING_CONFIDENCE_THRESHOLD": "0.80",
		"NEW_KEY":                     "new_value",
	}

	if err := UpsertEnv(path, updates); err != nil {
		t.Fatalf("UpsertEnv failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	content := string(data)

	// Check preserved comments and other keys
	if !strings.Contains(content, "# Configuration Header") {
		t.Errorf("lost header comment")
	}
	if !strings.Contains(content, "PORT=8080") {
		t.Errorf("lost PORT=8080")
	}
	if !strings.Contains(content, "# Database connection") {
		t.Errorf("lost database comment")
	}
	if !strings.Contains(content, "DATABASE_URL=postgres://localhost/db") {
		t.Errorf("lost DATABASE_URL")
	}

	// Check updated key
	if !strings.Contains(content, "ROUTING_CONFIDENCE_THRESHOLD=0.80") {
		t.Errorf("expected ROUTING_CONFIDENCE_THRESHOLD=0.80, got:\n%s", content)
	}
	if strings.Contains(content, "ROUTING_CONFIDENCE_THRESHOLD=0.75") {
		t.Errorf("old threshold value still present")
	}

	// Check appended key
	if !strings.Contains(content, "NEW_KEY=new_value") {
		t.Errorf("expected NEW_KEY=new_value")
	}
}

// TestApplyEnvFile verifies the startup .env hydration: keys from ENV_FILE
// reach the process env (settings persistence across restarts, spec-015
// convergence 2026-09-29 — compose interpolates only a subset of keys).
func TestApplyEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "" +
		"# comment line\n" +
		"\n" +
		"EARLY_EXIT_ENABLED=false\n" +
		"TIMEFRAME_SET_ALPHA=15m,4h\n" +
		"UPSTREAM_PROXY_URL=socks5://127.0.0.1:1080\n" +
		"QUOTED_KEY=\"quoted value\"\n" +
		"BROKEN_LINE_NO_EQUALS\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp env: %v", err)
	}

	t.Setenv("ENV_FILE", path)
	t.Setenv("EARLY_EXIT_ENABLED", "true") // file must win
	t.Setenv("TIMEFRAME_SET_ALPHA", "")
	ApplyEnvFile()

	if v := os.Getenv("EARLY_EXIT_ENABLED"); v != "false" {
		t.Errorf("EARLY_EXIT_ENABLED = %q, want file value false (file is source of truth)", v)
	}
	if v := os.Getenv("TIMEFRAME_SET_ALPHA"); v != "15m,4h" {
		t.Errorf("TIMEFRAME_SET_ALPHA = %q, want 15m,4h from file", v)
	}
	if v := os.Getenv("UPSTREAM_PROXY_URL"); v != "socks5://127.0.0.1:1080" {
		t.Errorf("UPSTREAM_PROXY_URL = %q, want file value", v)
	}
	if v := os.Getenv("QUOTED_KEY"); v != "quoted value" {
		t.Errorf("QUOTED_KEY = %q, want unquoted file value", v)
	}
	if v := os.Getenv("BROKEN_LINE_NO_EQUALS"); v != "" {
		t.Errorf("line without = must be skipped, got %q", v)
	}
}

// TestApplyEnvFile_NoFileNoop: no ENV_FILE / missing file = no crash, env untouched.
func TestApplyEnvFile_NoFileNoop(t *testing.T) {
	t.Setenv("ENV_FILE", "")
	ApplyEnvFile()
	t.Setenv("ENV_FILE", filepath.Join(t.TempDir(), "does-not-exist.env"))
	ApplyEnvFile()
}
