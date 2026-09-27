package ai

import (
	"strings"
	"testing"
	"time"
)

func TestHealthStats_Gateway(t *testing.T) {
	ResetStatsForTest()

	// Initial state
	s := GetGatewayStats()
	if s.Total != 0 || s.Success != 0 || s.Fail != 0 {
		t.Fatalf("expected 0 initial stats, got %+v", s)
	}

	// Record success
	RecordGateway(true, 100.0, "")
	s = GetGatewayStats()
	if s.Total != 1 || s.Success != 1 || s.Fail != 0 {
		t.Fatalf("expected 1 total, 1 success, got %+v", s)
	}
	if s.LastLatencyMs != 100.0 || s.EMA_LatencyMs != 100.0 {
		t.Fatalf("expected 100ms latency and EMA, got lat=%.1f, ema=%.1f", s.LastLatencyMs, s.EMA_LatencyMs)
	}
	if s.LastOK.IsZero() {
		t.Fatalf("expected non-zero LastOK")
	}

	// Record failure with long error string
	longErr := strings.Repeat("x", 300)
	RecordGateway(false, 200.0, longErr)
	s = GetGatewayStats()
	if s.Total != 2 || s.Success != 1 || s.Fail != 1 {
		t.Fatalf("expected 2 total, 1 success, 1 fail, got %+v", s)
	}
	if s.LastLatencyMs != 200.0 {
		t.Fatalf("expected 200ms last latency, got %.1f", s.LastLatencyMs)
	}
	// EMA with alpha=0.2: 0.2 * 200 + 0.8 * 100 = 120
	if s.EMA_LatencyMs != 120.0 {
		t.Fatalf("expected 120ms EMA, got %.1f", s.EMA_LatencyMs)
	}
	if len(s.LastError) != 200 {
		t.Fatalf("expected 200 max error len, got %d", len(s.LastError))
	}
}

func TestHealthStats_Jev(t *testing.T) {
	ResetStatsForTest()

	s := GetJevStats()
	if s.Total != 0 || s.Success != 0 || s.Fail != 0 {
		t.Fatalf("expected 0 initial stats, got %+v", s)
	}

	RecordJev(true, 50.0, "")
	s = GetJevStats()
	if s.Total != 1 || s.Success != 1 || s.Fail != 0 {
		t.Fatalf("expected 1 total, 1 success, got %+v", s)
	}
	if s.LastLatencyMs != 50.0 || s.EMA_LatencyMs != 50.0 {
		t.Fatalf("expected 50ms latency and EMA, got lat=%.1f, ema=%.1f", s.LastLatencyMs, s.EMA_LatencyMs)
	}

	time.Sleep(5 * time.Millisecond)
	RecordJev(false, 150.0, "typesafe timeout")
	s = GetJevStats()
	if s.Total != 2 || s.Success != 1 || s.Fail != 1 {
		t.Fatalf("expected 2 total, 1 success, 1 fail, got %+v", s)
	}
	// EMA: 0.2 * 150 + 0.8 * 50 = 70
	if s.EMA_LatencyMs != 70.0 {
		t.Fatalf("expected 70ms EMA, got %.1f", s.EMA_LatencyMs)
	}
	if s.LastError != "typesafe timeout" {
		t.Fatalf("expected 'typesafe timeout', got %q", s.LastError)
	}
}
