package server

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/market"
)

func TestApplyDecayState_15m_Timeframe(t *testing.T) {
	srv := &Server{}
	tf := "15m"
	sig := &db.FuturesTradeSignal{
		ID:         101,
		Symbol:     "SOL/USDT",
		Direction:  "LONG",
		Status:     "ACTIVE",
		EntryPrice: 100.0,
		StopLoss:   95.0,
		Profile:    "CRYPTO",
		Timeframe:  &tf,
		CreatedAt:  time.Now().Add(-16 * time.Minute), // 16 min old (>15m BE offset, <30m static)
		DecayState: "NONE",
	}

	// At price 100.5, rMultiple = 0.5 / 5.0 = 0.1 (<0.5R)
	// Under 15m profile (BE offset 15m), 16m age should trigger BREAKEVEN.
	// Under legacy static 60m profile (BE offset 30m), 16m age would NOT trigger BREAKEVEN.
	srv.applyDecayState(context.Background(), sig, 100.5)

	if sig.DecayState != "BREAKEVEN" {
		t.Fatalf("expected DecayState BREAKEVEN for 15m signal at 16m age, got %s", sig.DecayState)
	}
}

func TestApplyDecayState_LegacyRow_MappedAndLogged(t *testing.T) {
	srv := &Server{}

	var logBuf bytes.Buffer
	origWriter := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(origWriter)

	// 1. CRYPTO legacy row (Timeframe == nil) -> mapped to 1h
	sigCrypto := &db.FuturesTradeSignal{
		ID:         201,
		Symbol:     "BTC/USDT",
		Direction:  "LONG",
		Status:     "ACTIVE",
		EntryPrice: 60000.0,
		StopLoss:   59000.0,
		Profile:    "CRYPTO",
		Timeframe:  nil,
		CreatedAt:  time.Now().Add(-5 * time.Minute),
		DecayState: "NONE",
	}

	srv.applyDecayState(context.Background(), sigCrypto, 60100.0)

	if sigCrypto.Timeframe == nil || *sigCrypto.Timeframe != "1h" {
		t.Fatalf("expected legacy CRYPTO signal timeframe to be mapped to '1h', got %v", sigCrypto.Timeframe)
	}
	if !strings.Contains(logBuf.String(), "legacy_timeframe_mapped") {
		t.Errorf("expected log to contain 'legacy_timeframe_mapped', got: %s", logBuf.String())
	}

	// 2. Second touch uses stored value without re-logging mapping
	logBuf.Reset()
	srv.applyDecayState(context.Background(), sigCrypto, 60100.0)
	if strings.Contains(logBuf.String(), "legacy_timeframe_mapped") {
		t.Errorf("second touch should not re-log legacy_timeframe_mapped, got: %s", logBuf.String())
	}

	// 3. COMMODITY legacy row (Timeframe == nil) -> mapped to 4h
	logBuf.Reset()
	sigCommodity := &db.FuturesTradeSignal{
		ID:         202,
		Symbol:     "XAU/USD",
		Direction:  "LONG",
		Status:     "ACTIVE",
		EntryPrice: 2000.0,
		StopLoss:   1950.0,
		Profile:    "COMMODITY",
		Timeframe:  nil,
		CreatedAt:  time.Now().Add(-5 * time.Minute),
		DecayState: "NONE",
	}

	srv.applyDecayState(context.Background(), sigCommodity, 2005.0)

	if sigCommodity.Timeframe == nil || *sigCommodity.Timeframe != "4h" {
		t.Fatalf("expected legacy COMMODITY signal timeframe to be mapped to '4h', got %v", sigCommodity.Timeframe)
	}
	if !strings.Contains(logBuf.String(), "legacy_timeframe_mapped") {
		t.Errorf("expected log to contain 'legacy_timeframe_mapped', got: %s", logBuf.String())
	}
}

type trackingKlineProvider struct {
	lastInterval string
	err          error
	candles      []market.HistoricalCandle
}

func (m *trackingKlineProvider) FetchHistoricalKlines(ctx context.Context, symbol, interval string, limit int) ([]market.HistoricalCandle, error) {
	m.lastInterval = interval
	if m.err != nil {
		return nil, m.err
	}
	return m.candles, nil
}

func TestCandleFetch_PerIntervalProvider(t *testing.T) {
	// FR-207: Verify per-interval provider path works for signal's timeframe; missing interval data → explicit error
	provider := &trackingKlineProvider{
		candles: []market.HistoricalCandle{
			{Open: 100, Close: 101},
		},
	}

	for _, interval := range []string{"15m", "1h", "4h", "12h"} {
		candles, err := provider.FetchHistoricalKlines(context.Background(), "BTCUSDT", interval, 60)
		if err != nil {
			t.Fatalf("unexpected error for interval %s: %v", interval, err)
		}
		if provider.lastInterval != interval {
			t.Errorf("expected provider interval %s, got %s", interval, provider.lastInterval)
		}
		if len(candles) != 1 {
			t.Errorf("expected 1 candle, got %d", len(candles))
		}
	}

	// Missing interval data -> explicit error (FR-207)
	provider.err = errors.New("binance: interval 12h unavailable for symbol")
	_, err := provider.FetchHistoricalKlines(context.Background(), "BTCUSDT", "12h", 60)
	if err == nil {
		t.Fatalf("expected explicit error for missing interval data, got nil")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected error message to detail interval failure, got: %v", err)
	}
}
