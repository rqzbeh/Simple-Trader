package config

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// KnownIntervals contains all standard intervals supported by the platform (FR-203).
var KnownIntervals = map[string]bool{
	"15m": true,
	"30m": true,
	"1h":  true,
	"2h":  true,
	"4h":  true,
	"6h":  true,
	"12h": true,
	"1d":  true,
}

// TimeframeConfig holds the bucket-scoped timeframe choices.
type TimeframeConfig struct {
	AlphaSet []string
	CoreSet  []string
}

var (
	timeframeMu  sync.RWMutex
	liveAlphaSet = []string{"15m", "1h", "4h"}
	liveCoreSet  = []string{"1h", "4h", "12h"}
)

// ParseTimeframeSet parses and validates a comma-separated list of timeframe options.
// It fails loud if empty or if any option is outside KnownIntervals or lacks a profile (FR-203).
func ParseTimeframeSet(key, raw string) ([]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("config: %s cannot be empty (no default allowed)", key)
	}

	parts := strings.Split(trimmed, ",")
	var result []string
	seen := make(map[string]bool)

	for _, p := range parts {
		opt := strings.TrimSpace(p)
		if opt == "" {
			continue
		}
		if !KnownIntervals[opt] {
			return nil, fmt.Errorf("config: %s option %q not in known intervals {15m,30m,1h,2h,4h,6h,12h,1d}", key, opt)
		}
		if _, ok := DefaultTimeframeProfiles[opt]; !ok {
			return nil, fmt.Errorf("config: %s option %q has no timeframe profile", key, opt)
		}
		if seen[opt] {
			return nil, fmt.Errorf("config: %s contains duplicate option %q", key, opt)
		}
		seen[opt] = true
		result = append(result, opt)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("config: %s cannot be empty (no default allowed)", key)
	}

	return result, nil
}

// LoadTimeframeConfig parses TIMEFRAME_SET_ALPHA and TIMEFRAME_SET_CORE from environment.
// Fails loud at boot on empty set or unknown option (FR-203, quickstart V1).
func LoadTimeframeConfig() (*TimeframeConfig, error) {
	rawAlpha := os.Getenv("TIMEFRAME_SET_ALPHA")
	alphaSet, err := ParseTimeframeSet("TIMEFRAME_SET_ALPHA", rawAlpha)
	if err != nil {
		return nil, err
	}

	rawCore := os.Getenv("TIMEFRAME_SET_CORE")
	coreSet, err := ParseTimeframeSet("TIMEFRAME_SET_CORE", rawCore)
	if err != nil {
		return nil, err
	}

	timeframeMu.Lock()
	liveAlphaSet = make([]string, len(alphaSet))
	copy(liveAlphaSet, alphaSet)
	liveCoreSet = make([]string, len(coreSet))
	copy(liveCoreSet, coreSet)
	timeframeMu.Unlock()

	return &TimeframeConfig{
		AlphaSet: alphaSet,
		CoreSet:  coreSet,
	}, nil
}

// GetBucketTimeframeSet returns the active timeframe set for a bucket (ALPHA or CORE).
func GetBucketTimeframeSet(bucket string) []string {
	timeframeMu.RLock()
	defer timeframeMu.RUnlock()
	b := strings.ToUpper(strings.TrimSpace(bucket))
	if b == "ALPHA" {
		res := make([]string, len(liveAlphaSet))
		copy(res, liveAlphaSet)
		return res
	}
	res := make([]string, len(liveCoreSet))
	copy(res, liveCoreSet)
	return res
}

// UpdateTimeframeSet updates the live set in-process for a key (TIMEFRAME_SET_ALPHA or TIMEFRAME_SET_CORE).
func UpdateTimeframeSet(key, raw string) error {
	k := strings.ToUpper(strings.TrimSpace(key))
	switch k {
	case "TIMEFRAME_SET_ALPHA", "ALPHA":
		set, err := ParseTimeframeSet("TIMEFRAME_SET_ALPHA", raw)
		if err != nil {
			return err
		}
		timeframeMu.Lock()
		liveAlphaSet = set
		timeframeMu.Unlock()
		return nil
	case "TIMEFRAME_SET_CORE", "CORE":
		set, err := ParseTimeframeSet("TIMEFRAME_SET_CORE", raw)
		if err != nil {
			return err
		}
		timeframeMu.Lock()
		liveCoreSet = set
		timeframeMu.Unlock()
		return nil
	default:
		return fmt.Errorf("config: unknown timeframe set key %q", key)
	}
}
