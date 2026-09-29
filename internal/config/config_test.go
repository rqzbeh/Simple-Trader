package config_test

import (
	"os"
	"strings"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/config"
)

// setSampleEnv installs every required key with its bootstrap sample value.
func setSampleEnv(t *testing.T) {
	t.Helper()
	for k, v := range config.SampleRequiredEnv() {
		t.Setenv(k, v)
	}
}

// TestLoadConfig_MissingListsKeys: no env = one error naming EVERY missing
// key (spec-017 FR-401 — no in-code defaults).
func TestLoadConfig_MissingListsKeys(t *testing.T) {
	os.Clearenv()
	_, err := config.Load()
	if err == nil {
		t.Fatalf("expected explicit error when required keys are missing")
	}
	msg := err.Error()
	if !strings.Contains(msg, "required env keys invalid") {
		t.Errorf("error must state the cause, got: %v", msg)
	}
	for _, k := range []string{"PORT", "DATABASE_URL", "JEV_BASE_URL", "EARLY_EXIT_ENABLED", "TIMEFRAME_SET_ALPHA"} {
		if !strings.Contains(msg, k) {
			t.Errorf("error must name missing key %s, got: %v", k, msg)
		}
	}
}

// TestLoadConfig_SampleBoots: SampleRequiredEnv → Load succeeds with sample values.
func TestLoadConfig_SampleBoots(t *testing.T) {
	setSampleEnv(t)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("sample env must boot: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.AIBaseURL != "https://api.openai.com/v1" {
		t.Errorf("AIBaseURL = %q", cfg.AIBaseURL)
	}
	if cfg.InitialCapital != 10000 {
		t.Errorf("InitialCapital = %v, want 10000", cfg.InitialCapital)
	}
}

// TestLoadConfig_EnvOverrides: sample + explicit overrides.
func TestLoadConfig_EnvOverrides(t *testing.T) {
	setSampleEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("AI_MODEL_ID", "claude-3-5-sonnet")
	t.Setenv("AI_BASE_URL", "http://localhost:11434/v1")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error loading env overrides, got %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("expected Port 9090, got %s", cfg.Port)
	}
	if cfg.AIModelID != "claude-3-5-sonnet" {
		t.Errorf("expected AIModelID claude-3-5-sonnet, got %s", cfg.AIModelID)
	}
	if cfg.AIBaseURL != "http://localhost:11434/v1" {
		t.Errorf("expected AIBaseURL http://localhost:11434/v1, got %s", cfg.AIBaseURL)
	}
}

// TestLoadConfig_SemanticsReject: present-but-unusable values fail loud
// (they used to be silently replaced by in-code defaults).
func TestLoadConfig_SemanticsReject(t *testing.T) {
	setSampleEnv(t)
	t.Setenv("IMPACT_FACTOR", "0")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "IMPACT_FACTOR") {
		t.Errorf("IMPACT_FACTOR=0 must fail loud, got %v", err)
	}
	setSampleEnv(t)
	t.Setenv("MAX_CONCURRENT_SIGNALS", "abc")
	if _, err := config.Load(); err == nil {
		t.Errorf("non-integer MAX_CONCURRENT_SIGNALS must fail")
	}
	setSampleEnv(t)
	t.Setenv("MIN_STOP_LOSS_PCT", "5")
	t.Setenv("MAX_STOP_LOSS_PCT", "1")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "MIN_STOP_LOSS_PCT") {
		t.Errorf("stop band inversion must fail loud, got %v", err)
	}
}

// TestLoadConfig_ProxyOptional: unset = direct (no error); invalid = startup error.
func TestLoadConfig_ProxyOptional(t *testing.T) {
	setSampleEnv(t)
	t.Setenv("UPSTREAM_PROXY_URL", "")
	if _, err := config.Load(); err != nil {
		t.Errorf("unset proxy must be valid (direct): %v", err)
	}
	t.Setenv("UPSTREAM_PROXY_URL", "socks5://127.0.0.1:1080")
	if _, err := config.Load(); err != nil {
		t.Errorf("valid socks5 proxy must load: %v", err)
	}
	t.Setenv("UPSTREAM_PROXY_URL", "ftp://nope")
	if _, err := config.Load(); err == nil {
		t.Errorf("invalid proxy scheme must fail loud at startup")
	}
}
