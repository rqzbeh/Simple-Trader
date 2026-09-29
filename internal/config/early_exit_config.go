package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// EarlyExitConfig holds configuration for the news-driven early exit loop (spec-014).
type EarlyExitConfig struct {
	Enabled     bool    `json:"early_exit_enabled"`
	MinHoldMin  int     `json:"early_exit_min_hold_min"`
	MaxPerDay   int     `json:"early_exit_max_per_day"`
	CooldownMin int     `json:"early_exit_cooldown_min"`
	ConfFloor   float64 `json:"early_exit_conf_floor"`
}

// LoadEarlyExitConfig loads and strictly validates early exit guard settings from environment variables.
// Invalid values produce explicit startup errors (FR-104 / FR-007, no silent defaults or clamping).
func LoadEarlyExitConfig() (EarlyExitConfig, error) {
	// spec-017 FR-401: no in-code defaults — every key must be present in env.
	var cfg EarlyExitConfig
	for _, k := range []string{"EARLY_EXIT_ENABLED", "EARLY_EXIT_MIN_HOLD_MIN", "EARLY_EXIT_MAX_PER_DAY", "EARLY_EXIT_COOLDOWN_MIN", "EARLY_EXIT_CONF_FLOOR"} {
		if _, ok := os.LookupEnv(k); !ok {
			return cfg, fmt.Errorf("config: required env key missing (no in-code default): %s", k)
		}
	}

	if val := strings.TrimSpace(os.Getenv("EARLY_EXIT_ENABLED")); val != "" {
		b, err := strconv.ParseBool(val)
		if err != nil {
			return cfg, fmt.Errorf("config: EARLY_EXIT_ENABLED invalid bool: %q", val)
		}
		cfg.Enabled = b
	}

	if val := strings.TrimSpace(os.Getenv("EARLY_EXIT_MIN_HOLD_MIN")); val != "" {
		i, err := strconv.Atoi(val)
		if err != nil || i < 0 {
			return cfg, fmt.Errorf("config: EARLY_EXIT_MIN_HOLD_MIN invalid: must be integer >= 0, got %q", val)
		}
		cfg.MinHoldMin = i
	}

	if val := strings.TrimSpace(os.Getenv("EARLY_EXIT_MAX_PER_DAY")); val != "" {
		i, err := strconv.Atoi(val)
		if err != nil || i < 1 {
			return cfg, fmt.Errorf("config: EARLY_EXIT_MAX_PER_DAY invalid: must be integer >= 1, got %q", val)
		}
		cfg.MaxPerDay = i
	}

	if val := strings.TrimSpace(os.Getenv("EARLY_EXIT_COOLDOWN_MIN")); val != "" {
		i, err := strconv.Atoi(val)
		if err != nil || i < 0 {
			return cfg, fmt.Errorf("config: EARLY_EXIT_COOLDOWN_MIN invalid: must be integer >= 0, got %q", val)
		}
		cfg.CooldownMin = i
	}

	if val := strings.TrimSpace(os.Getenv("EARLY_EXIT_CONF_FLOOR")); val != "" {
		f, err := strconv.ParseFloat(val, 64)
		if err != nil || f < 0.0 || f > 1.0 {
			return cfg, fmt.Errorf("config: EARLY_EXIT_CONF_FLOOR invalid: must be float between 0.0 and 1.0, got %q", val)
		}
		cfg.ConfFloor = f
	}

	return cfg, nil
}
