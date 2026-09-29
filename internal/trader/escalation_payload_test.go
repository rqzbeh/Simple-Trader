package trader

import (
	"strings"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
)

// TestEscalationPayload_Passthrough: trade path hands the full entry request
// through unchanged (2026-09-29 defect sent an empty dummy body instead).
func TestEscalationPayload_Passthrough(t *testing.T) {
	in := ai.DecisionRequest{
		Symbol:        "ETH/USDT",
		Bucket:        "ALPHA",
		NewsHeadlines: []string{"US SEC follows CFTC in staff guidance for crypto"},
	}
	in.IndicatorSnap.RSI = 61.5

	out, err := EscalationPayload(in)
	if err != nil {
		t.Fatalf("EscalationPayload failed: %v", err)
	}
	if out.Symbol != "ETH/USDT" || out.Bucket != "ALPHA" {
		t.Errorf("payload not passed through: %+v", out)
	}
	if out.IndicatorSnap.RSI != 61.5 {
		t.Errorf("indicator snapshot lost: %+v", out.IndicatorSnap)
	}
	if len(out.NewsHeadlines) != 1 {
		t.Errorf("headlines lost: %v", out.NewsHeadlines)
	}
}

// TestEscalationPayload_FromStateObject: shadow-side Route hands a StateObject;
// escalation must reconstruct the same numeric evidence, never an empty body.
func TestEscalationPayload_FromStateObject(t *testing.T) {
	in := StateObject{
		Symbol: "BTC/USDT",
		Indicators: map[string]float64{
			"rsi": 55, "macd": 12, "macd_signal": 11, "macd_histogram": 1,
			"bb_upper": 100, "bb_middle": 99, "bb_lower": 98,
			"confluence": 0.72, "obi": 0.3,
		},
	}
	out, err := EscalationPayload(in)
	if err != nil {
		t.Fatalf("EscalationPayload(StateObject) failed: %v", err)
	}
	if out.Symbol != "BTC/USDT" {
		t.Errorf("symbol = %q, want BTC/USDT", out.Symbol)
	}
	snap := out.IndicatorSnap
	if snap.RSI != 55 || snap.MACD != 12 || snap.Signal != 11 || snap.Histogram != 1 ||
		snap.UpperBand != 100 || snap.MiddleBand != 99 || snap.LowerBand != 98 ||
		snap.ConfluenceScore != 0.72 || snap.OBI != 0.3 {
		t.Errorf("state-to-request mapping wrong: %+v", snap)
	}
}

// TestEscalationPayload_RejectsUnknown: unknown payload = explicit error —
// zero-fallback law forbids judging an empty context.
func TestEscalationPayload_RejectsUnknown(t *testing.T) {
	_, err := EscalationPayload("escalated")
	if err == nil {
		t.Fatalf("expected explicit error for unsupported payload type")
	}
	if !strings.Contains(err.Error(), "unsupported escalation payload") {
		t.Errorf("error must name the cause, got: %v", err)
	}
}
