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

// AllKnownIntervals lists all standard intervals in deterministic sorted order (spec-019).
var AllKnownIntervals = []string{"15m", "30m", "1h", "2h", "4h", "6h", "12h", "1d"}

// TimeframeConfig holds the bucket-scoped timeframe choices.
type TimeframeConfig struct {
	AlphaSet []string
	CoreSet  []string
}

var (
	timeframeMu sync.RWMutex
	// spec-017 FR-402: no in-code default sets — populated by
	// LoadTimeframeConfig() at boot (env TIMEFRAME_SET_ALPHA/CORE).
	// spec-019: nil/empty live set indicates unrestricted mode.
	liveAlphaSet []string
	liveCoreSet  []string
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
// An absent or empty env for a set leaves it unrestricted (nil/empty live slice, spec-019).
// An invalid non-empty value still fails loud at boot (FR-203/FR-601).
func LoadTimeframeConfig() (*TimeframeConfig, error) {
	var alphaSet []string
	if rawAlpha := strings.TrimSpace(os.Getenv("TIMEFRAME_SET_ALPHA")); rawAlpha != "" {
		var err error
		alphaSet, err = ParseTimeframeSet("TIMEFRAME_SET_ALPHA", rawAlpha)
		if err != nil {
			return nil, err
		}
	}

	var coreSet []string
	if rawCore := strings.TrimSpace(os.Getenv("TIMEFRAME_SET_CORE")); rawCore != "" {
		var err error
		coreSet, err = ParseTimeframeSet("TIMEFRAME_SET_CORE", rawCore)
		if err != nil {
			return nil, err
		}
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

// TimeframeUnrestricted returns true if both bucket sets are unrestricted (empty/nil).
func TimeframeUnrestricted() bool {
	timeframeMu.RLock()
	defer timeframeMu.RUnlock()
	return len(liveAlphaSet) == 0 && len(liveCoreSet) == 0
}

// TimeframeBucketUnrestricted returns true if the specified bucket set is unrestricted.
func TimeframeBucketUnrestricted(bucket string) bool {
	timeframeMu.RLock()
	defer timeframeMu.RUnlock()
	b := strings.ToUpper(strings.TrimSpace(bucket))
	if b == "ALPHA" {
		return len(liveAlphaSet) == 0
	}
	return len(liveCoreSet) == 0
}

// GetBucketTimeframeSet returns the active timeframe set for a bucket (ALPHA or CORE).
// When the live set is empty (unrestricted), it returns all KnownIntervals keys in deterministic order.
func GetBucketTimeframeSet(bucket string) []string {
	timeframeMu.RLock()
	defer timeframeMu.RUnlock()
	b := strings.ToUpper(strings.TrimSpace(bucket))
	var live []string
	if b == "ALPHA" {
		live = liveAlphaSet
	} else {
		live = liveCoreSet
	}
	if len(live) == 0 {
		res := make([]string, len(AllKnownIntervals))
		copy(res, AllKnownIntervals)
		return res
	}
	res := make([]string, len(live))
	copy(res, live)
	return res
}

// UpdateTimeframeSet updates the live set in-process for a key (TIMEFRAME_SET_ALPHA or TIMEFRAME_SET_CORE).
func UpdateTimeframeSet(key, raw string) error {
	k := strings.ToUpper(strings.TrimSpace(key))
	switch k {
	case "TIMEFRAME_SET_ALPHA", "ALPHA":
		if strings.TrimSpace(raw) == "" {
			timeframeMu.Lock()
			liveAlphaSet = nil
			timeframeMu.Unlock()
			return nil
		}
		set, err := ParseTimeframeSet("TIMEFRAME_SET_ALPHA", raw)
		if err != nil {
			return err
		}
		timeframeMu.Lock()
		liveAlphaSet = set
		timeframeMu.Unlock()
		return nil
	case "TIMEFRAME_SET_CORE", "CORE":
		if strings.TrimSpace(raw) == "" {
			timeframeMu.Lock()
			liveCoreSet = nil
			timeframeMu.Unlock()
			return nil
		}
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
