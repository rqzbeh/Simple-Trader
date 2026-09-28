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
