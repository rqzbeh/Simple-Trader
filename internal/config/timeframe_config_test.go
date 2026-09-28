package config

import (
	"os"
	"strings"
	"testing"
)

func TestParseTimeframeSet_Empty(t *testing.T) {
	for _, raw := range []string{"", "   ", " , , "} {
		_, err := ParseTimeframeSet("TIMEFRAME_SET_ALPHA", raw)
		if err == nil {
			t.Fatalf("expected error for empty raw %q, got nil", raw)
		}
		if !strings.Contains(err.Error(), "TIMEFRAME_SET_ALPHA") {
			t.Errorf("expected error to mention key TIMEFRAME_SET_ALPHA, got %v", err)
		}
	}
}

func TestParseTimeframeSet_UnknownOption(t *testing.T) {
	testCases := []struct {
		key string
		raw string
	}{
		{"TIMEFRAME_SET_ALPHA", "15m,3h,4h"},
		{"TIMEFRAME_SET_CORE", "1h,5m,12h"},
		{"TIMEFRAME_SET_ALPHA", "invalid"},
	}

	for _, tc := range testCases {
		_, err := ParseTimeframeSet(tc.key, tc.raw)
		if err == nil {
			t.Fatalf("expected error for unknown option in %q, got nil", tc.raw)
		}
		if !strings.Contains(err.Error(), tc.key) {
			t.Errorf("expected error to mention key %s, got %v", tc.key, err)
		}
	}
}

func TestParseTimeframeSet_Valid(t *testing.T) {
	alpha, err := ParseTimeframeSet("TIMEFRAME_SET_ALPHA", "15m, 1h, 4h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedAlpha := []string{"15m", "1h", "4h"}
	if len(alpha) != len(expectedAlpha) {
		t.Fatalf("expected %d items, got %d", len(expectedAlpha), len(alpha))
	}
	for i, v := range expectedAlpha {
		if alpha[i] != v {
			t.Errorf("at index %d: expected %s, got %s", i, v, alpha[i])
		}
	}

	core, err := ParseTimeframeSet("TIMEFRAME_SET_CORE", "1h,4h,12h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedCore := []string{"1h", "4h", "12h"}
	if len(core) != len(expectedCore) {
		t.Fatalf("expected %d items, got %d", len(expectedCore), len(core))
	}
	for i, v := range expectedCore {
		if core[i] != v {
			t.Errorf("at index %d: expected %s, got %s", i, v, core[i])
		}
	}
}

func TestLoadTimeframeConfig_BootValidation(t *testing.T) {
	origAlpha := os.Getenv("TIMEFRAME_SET_ALPHA")
	origCore := os.Getenv("TIMEFRAME_SET_CORE")
	defer func() {
		os.Setenv("TIMEFRAME_SET_ALPHA", origAlpha)
		os.Setenv("TIMEFRAME_SET_CORE", origCore)
	}()

	// 1. Missing ALPHA -> startup error mentioning key
	os.Setenv("TIMEFRAME_SET_ALPHA", "")
	os.Setenv("TIMEFRAME_SET_CORE", "1h,4h,12h")
	cfg, err := LoadTimeframeConfig()
	if err == nil {
		t.Fatalf("expected error when TIMEFRAME_SET_ALPHA missing, got cfg: %+v", cfg)
	}
	if !strings.Contains(err.Error(), "TIMEFRAME_SET_ALPHA") {
		t.Errorf("expected error to mention TIMEFRAME_SET_ALPHA, got %v", err)
	}

	// 2. Missing CORE -> startup error mentioning key
	os.Setenv("TIMEFRAME_SET_ALPHA", "15m,1h,4h")
	os.Setenv("TIMEFRAME_SET_CORE", "")
	cfg, err = LoadTimeframeConfig()
	if err == nil {
		t.Fatalf("expected error when TIMEFRAME_SET_CORE missing, got cfg: %+v", cfg)
	}
	if !strings.Contains(err.Error(), "TIMEFRAME_SET_CORE") {
		t.Errorf("expected error to mention TIMEFRAME_SET_CORE, got %v", err)
	}

	// 3. Unknown option in ALPHA -> error
	os.Setenv("TIMEFRAME_SET_ALPHA", "15m,99m,4h")
	os.Setenv("TIMEFRAME_SET_CORE", "1h,4h,12h")
	_, err = LoadTimeframeConfig()
	if err == nil {
		t.Fatalf("expected error for unknown option, got nil")
	}

	// 4. Valid sets parse successfully
	os.Setenv("TIMEFRAME_SET_ALPHA", "15m,1h,4h")
	os.Setenv("TIMEFRAME_SET_CORE", "1h,4h,12h")
	cfg, err = LoadTimeframeConfig()
	if err != nil {
		t.Fatalf("expected valid config to load, got error: %v", err)
	}
	if len(cfg.AlphaSet) != 3 || len(cfg.CoreSet) != 3 {
		t.Errorf("unexpected set sizes: alpha=%v, core=%v", cfg.AlphaSet, cfg.CoreSet)
	}
}

func TestTimeframeProfiles_Seeded(t *testing.T) {
	expected := map[string]TimeframeProfile{
		"15m": {HorizonMin: 45, BEOffsetMin: 15, FlatOffsetMin: 30},
		"1h":  {HorizonMin: 120, BEOffsetMin: 30, FlatOffsetMin: 60},
		"4h":  {HorizonMin: 360, BEOffsetMin: 90, FlatOffsetMin: 180},
		"12h": {HorizonMin: 720, BEOffsetMin: 180, FlatOffsetMin: 360},
	}

	for k, exp := range expected {
		prof, ok := DefaultTimeframeProfiles[k]
		if !ok {
			t.Errorf("missing profile for %s", k)
			continue
		}
		if prof != exp {
			t.Errorf("profile %s mismatch: expected %+v, got %+v", k, exp, prof)
		}
	}
}
