package market

import (
	"testing"
	"time"
)

func Test3HourCandleAggregation(t *testing.T) {
	cfg := AggregatorConfig{
		TimeframeSeconds: 10800, // 3 hours
		MaxHistoryBars:   10,
	}
	agg := NewCandleAggregator(cfg)

	// Base time at 00:00:00 UTC
	baseTime := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)

	// Ingest ticks within the first 3-hour window (00:00 - 03:00)
	agg.IngestTick("BTC/USD", 65000.0, 1.5, baseTime.Add(10*time.Minute))
	agg.IngestTick("BTC/USD", 65500.0, 2.0, baseTime.Add(30*time.Minute))
	agg.IngestTick("BTC/USD", 64800.0, 0.5, baseTime.Add(90*time.Minute))
	agg.IngestTick("BTC/USD", 65200.0, 1.0, baseTime.Add(150*time.Minute))

	cur, ok := agg.GetCurrentBar("BTC/USD")
	if !ok {
		t.Fatalf("expected active current bar")
	}

	if cur.Open != 65000.0 {
		t.Fatalf("expected Open 65000, got %.2f", cur.Open)
	}
	if cur.High != 65500.0 {
		t.Fatalf("expected High 65500, got %.2f", cur.High)
	}
	if cur.Low != 64800.0 {
		t.Fatalf("expected Low 64800, got %.2f", cur.Low)
	}
	if cur.Close != 65200.0 {
		t.Fatalf("expected Close 65200, got %.2f", cur.Close)
	}
	if cur.Volume != 5.0 {
		t.Fatalf("expected Volume 5.0, got %.2f", cur.Volume)
	}
	if cur.Ticks != 4 {
		t.Fatalf("expected 4 ticks, got %d", cur.Ticks)
	}

	// Ingest tick at 03:05:00 (crosses into second 3-hour window)
	completedBar, isComplete := agg.IngestTick("BTC/USD", 65300.0, 1.2, baseTime.Add(3*time.Hour+5*time.Minute))
	if !isComplete || completedBar == nil {
		t.Fatalf("expected 3-hour bar to complete upon entering new window")
	}

	if !completedBar.Complete {
		t.Fatalf("expected completed bar flag to be true")
	}
	if completedBar.Close != 65200.0 {
		t.Fatalf("expected completed bar Close to be 65200, got %.2f", completedBar.Close)
	}

	// Verify historical bars
	history := agg.GetHistory("BTC/USD")
	if len(history) != 1 {
		t.Fatalf("expected 1 completed bar in history, got %d", len(history))
	}
	if history[0].Close != 65200.0 {
		t.Fatalf("expected historical bar Close 65200, got %.2f", history[0].Close)
	}
}
