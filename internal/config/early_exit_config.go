package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// EarlyExitConfig holds configuration for the news-driven early exit loop (spec-014).
type EarlyExitConfig struct {
	Enabled     bool            `json:"early_exit_enabled"`
	MinHoldMin  int             `json:"early_exit_min_hold_min"`
	MaxPerDay   int             `json:"early_exit_max_per_day"`
	CooldownMin int             `json:"early_exit_cooldown_min"`
	ConfFloor   float64         `json:"early_exit_conf_floor"`
	Managed     map[string]bool `json:"managed,omitempty"`
}

// IsEnabled returns true if early exit is enabled.
// Absent env (managed) defaults to enabled; only explicit false disables (toggle convention, spec-019).
func (c EarlyExitConfig) IsEnabled() bool {
	if c.Managed != nil && c.Managed["ENABLED"] {
		return true
	}
	return c.Enabled
}

// LoadEarlyExitConfig loads and strictly validates early exit guard settings from environment variables.
// In spec-019, absent keys are recorded as Managed with their field left zero.
// Present values are strictly validated (invalid values produce explicit startup errors).
func LoadEarlyExitConfig() (EarlyExitConfig, error) {
	cfg := EarlyExitConfig{
		Managed: make(map[string]bool),
	}

	if raw, ok := os.LookupEnv("EARLY_EXIT_ENABLED"); !ok || strings.TrimSpace(raw) == "" {
		cfg.Managed["ENABLED"] = true
	} else {
		val := strings.TrimSpace(raw)
		b, err := strconv.ParseBool(val)
		if err != nil {
			return cfg, fmt.Errorf("config: EARLY_EXIT_ENABLED invalid bool: %q", val)
		}
		cfg.Enabled = b
	}

	if raw, ok := os.LookupEnv("EARLY_EXIT_MIN_HOLD_MIN"); !ok || strings.TrimSpace(raw) == "" {
		cfg.Managed["MIN_HOLD_MIN"] = true
	} else {
		val := strings.TrimSpace(raw)
		i, err := strconv.Atoi(val)
		if err != nil || i < 0 {
			return cfg, fmt.Errorf("config: EARLY_EXIT_MIN_HOLD_MIN invalid: must be integer >= 0, got %q", val)
		}
		cfg.MinHoldMin = i
	}

	if raw, ok := os.LookupEnv("EARLY_EXIT_MAX_PER_DAY"); !ok || strings.TrimSpace(raw) == "" {
		cfg.Managed["MAX_PER_DAY"] = true
	} else {
		val := strings.TrimSpace(raw)
		i, err := strconv.Atoi(val)
		if err != nil || i < 1 {
			return cfg, fmt.Errorf("config: EARLY_EXIT_MAX_PER_DAY invalid: must be integer >= 1, got %q", val)
		}
		cfg.MaxPerDay = i
	}

	if raw, ok := os.LookupEnv("EARLY_EXIT_COOLDOWN_MIN"); !ok || strings.TrimSpace(raw) == "" {
		cfg.Managed["COOLDOWN_MIN"] = true
	} else {
		val := strings.TrimSpace(raw)
		i, err := strconv.Atoi(val)
		if err != nil || i < 0 {
			return cfg, fmt.Errorf("config: EARLY_EXIT_COOLDOWN_MIN invalid: must be integer >= 0, got %q", val)
		}
		cfg.CooldownMin = i
	}

	if raw, ok := os.LookupEnv("EARLY_EXIT_CONF_FLOOR"); !ok || strings.TrimSpace(raw) == "" {
		cfg.Managed["CONF_FLOOR"] = true
	} else {
		val := strings.TrimSpace(raw)
		f, err := strconv.ParseFloat(val, 64)
		if err != nil || f < 0.0 || f > 1.0 {
			return cfg, fmt.Errorf("config: EARLY_EXIT_CONF_FLOOR invalid: must be float between 0.0 and 1.0, got %q", val)
		}
		cfg.ConfFloor = f
	}

	return cfg, nil
}
