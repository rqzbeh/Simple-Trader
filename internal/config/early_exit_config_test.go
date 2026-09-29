package config

import (
	"os"
	"testing"
)

func TestLoadEarlyExitConfig_MissingIsManaged(t *testing.T) {
	// spec-019 FR-603: absent keys are recorded as managed with zero fields.
	keys := []string{
		"EARLY_EXIT_ENABLED",
		"EARLY_EXIT_MIN_HOLD_MIN",
		"EARLY_EXIT_MAX_PER_DAY",
		"EARLY_EXIT_COOLDOWN_MIN",
		"EARLY_EXIT_CONF_FLOOR",
	}
	for _, k := range keys {
		os.Unsetenv(k)
	}

	cfg, err := LoadEarlyExitConfig()
	if err != nil {
		t.Fatalf("expected absent early-exit keys to succeed as managed, got err: %v", err)
	}
	for _, k := range []string{"ENABLED", "MIN_HOLD_MIN", "MAX_PER_DAY", "COOLDOWN_MIN", "CONF_FLOOR"} {
		if !cfg.Managed[k] {
			t.Errorf("expected Managed[%q] to be true, got false", k)
		}
	}
	if cfg.Enabled {
		t.Errorf("expected Enabled field to be zero (false), got true")
	}
	if !cfg.IsEnabled() {
		t.Errorf("expected IsEnabled() to be true under toggle convention")
	}
	if cfg.MinHoldMin != 0 || cfg.MaxPerDay != 0 || cfg.CooldownMin != 0 || cfg.ConfFloor != 0 {
		t.Errorf("expected all fields to be zero, got minHold=%d maxPerDay=%d cooldown=%d confFloor=%f",
			cfg.MinHoldMin, cfg.MaxPerDay, cfg.CooldownMin, cfg.ConfFloor)
	}
}

func TestLoadEarlyExitConfig_CustomValid(t *testing.T) {
	os.Setenv("EARLY_EXIT_ENABLED", "false")
	os.Setenv("EARLY_EXIT_MIN_HOLD_MIN", "45")
	os.Setenv("EARLY_EXIT_MAX_PER_DAY", "5")
	os.Setenv("EARLY_EXIT_COOLDOWN_MIN", "120")
	os.Setenv("EARLY_EXIT_CONF_FLOOR", "0.80")
	defer func() {
		os.Unsetenv("EARLY_EXIT_ENABLED")
		os.Unsetenv("EARLY_EXIT_MIN_HOLD_MIN")
		os.Unsetenv("EARLY_EXIT_MAX_PER_DAY")
		os.Unsetenv("EARLY_EXIT_COOLDOWN_MIN")
		os.Unsetenv("EARLY_EXIT_CONF_FLOOR")
	}()

	cfg, err := LoadEarlyExitConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Enabled {
		t.Errorf("expected Enabled=false, got true")
	}
	if cfg.MinHoldMin != 45 {
		t.Errorf("expected MinHoldMin=45, got %d", cfg.MinHoldMin)
	}
	if cfg.MaxPerDay != 5 {
		t.Errorf("expected MaxPerDay=5, got %d", cfg.MaxPerDay)
	}
	if cfg.CooldownMin != 120 {
		t.Errorf("expected CooldownMin=120, got %d", cfg.CooldownMin)
	}
	if cfg.ConfFloor != 0.80 {
		t.Errorf("expected ConfFloor=0.80, got %f", cfg.ConfFloor)
	}
}

func TestLoadEarlyExitConfig_KillSwitchParsing(t *testing.T) {
	defer os.Unsetenv("EARLY_EXIT_ENABLED")
	// remaining required keys present (spec-017: no struct defaults)
	t.Setenv("EARLY_EXIT_MIN_HOLD_MIN", "30")
	t.Setenv("EARLY_EXIT_MAX_PER_DAY", "3")
	t.Setenv("EARLY_EXIT_COOLDOWN_MIN", "60")
	t.Setenv("EARLY_EXIT_CONF_FLOOR", "0.75")

	valid := []struct {
		val      string
		expected bool
	}{
		{"true", true},
		{"1", true},
		{"TRUE", true},
		{"false", false},
		{"0", false},
		{"FALSE", false},
	}
	for _, tc := range valid {
		os.Setenv("EARLY_EXIT_ENABLED", tc.val)
		cfg, err := LoadEarlyExitConfig()
		if err != nil {
			t.Errorf("expected valid for %s, got error %v", tc.val, err)
		} else if cfg.Enabled != tc.expected {
			t.Errorf("for val=%s expected Enabled=%v, got %v", tc.val, tc.expected, cfg.Enabled)
		}
	}

	os.Setenv("EARLY_EXIT_ENABLED", "not_a_bool")
	if _, err := LoadEarlyExitConfig(); err == nil {
		t.Errorf("expected error for invalid bool string, got nil")
	}
}

func TestLoadEarlyExitConfig_InvalidValues(t *testing.T) {
	cleanup := func() {
		os.Unsetenv("EARLY_EXIT_ENABLED")
		os.Unsetenv("EARLY_EXIT_MIN_HOLD_MIN")
		os.Unsetenv("EARLY_EXIT_MAX_PER_DAY")
		os.Unsetenv("EARLY_EXIT_COOLDOWN_MIN")
		os.Unsetenv("EARLY_EXIT_CONF_FLOOR")
	}

	tests := []struct {
		name string
		key  string
		val  string
	}{
		{"conf_floor negative", "EARLY_EXIT_CONF_FLOOR", "-0.1"},
		{"conf_floor greater than 1", "EARLY_EXIT_CONF_FLOOR", "1.05"},
		{"conf_floor non-numeric", "EARLY_EXIT_CONF_FLOOR", "abc"},
		{"min_hold negative", "EARLY_EXIT_MIN_HOLD_MIN", "-1"},
		{"min_hold non-integer", "EARLY_EXIT_MIN_HOLD_MIN", "1.5"},
		{"cooldown negative", "EARLY_EXIT_COOLDOWN_MIN", "-10"},
		{"cooldown non-integer", "EARLY_EXIT_COOLDOWN_MIN", "xyz"},
		{"max_per_day zero", "EARLY_EXIT_MAX_PER_DAY", "0"},
		{"max_per_day negative", "EARLY_EXIT_MAX_PER_DAY", "-2"},
		{"max_per_day non-integer", "EARLY_EXIT_MAX_PER_DAY", "foo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanup()
			defer cleanup()
			os.Setenv(tc.key, tc.val)
			_, err := LoadEarlyExitConfig()
			if err == nil {
				t.Errorf("expected error for %s=%s, got nil (must not silently clamp)", tc.key, tc.val)
			}
		})
	}
}
