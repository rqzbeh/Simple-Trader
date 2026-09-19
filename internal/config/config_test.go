package config_test

import (
	"os"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/config"
)

func TestLoadConfig_Defaults(t *testing.T) {
	os.Clearenv()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected no error loading defaults, got %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected default Port 8080, got %s", cfg.Port)
	}
	if cfg.DatabaseURL == "" {
		t.Errorf("expected default DatabaseURL to not be empty")
	}
	if cfg.RedisURL == "" {
		t.Errorf("expected default RedisURL to not be empty")
	}
	if cfg.AIBaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected default AIBaseURL https://api.openai.com/v1, got %s", cfg.AIBaseURL)
	}
	if cfg.AIModelID != "gpt-4o" {
		t.Errorf("expected default AIModelID gpt-4o, got %s", cfg.AIModelID)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("AI_MODEL_ID", "claude-3-5-sonnet")
	os.Setenv("AI_BASE_URL", "http://localhost:11434/v1")
	defer os.Clearenv()

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
