package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/server"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

type mockLivePriceProvider struct {
	prices map[string]float64
}

func (m *mockLivePriceProvider) GetLatestPrice(symbol string) (float64, error) {
	if p, ok := m.prices[symbol]; ok {
		return p, nil
	}
	return 0, nil
}

func TestPositionsAndPortfolioSummaryEndpoints(t *testing.T) {
	cfg := &config.Config{
		InitialCapital: 50000.0,
		CoreTargetPct:  0.60,
		AlphaTargetPct: 0.40,
	}

	execEngine := trader.NewExecutionEngine(50000.0)
	priceProvider := &mockLivePriceProvider{
		prices: map[string]float64{
			"BTC/USDT": 66000.0,
		},
	}
	execEngine.SetPriceProvider(priceProvider)

	srv := server.NewServer(cfg, nil, nil, nil, nil, execEngine)
	r := srv.Router()

	t.Run("GET /api/v1/positions returns empty list initially", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/positions", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var trades []*db.Trade
		if err := json.Unmarshal(rec.Body.Bytes(), &trades); err != nil {
			t.Fatalf("failed to decode positions: %v", err)
		}
		if len(trades) != 0 {
			t.Fatalf("expected 0 positions, got %d", len(trades))
		}
	})

	t.Run("GET /api/v1/portfolio/summary returns config-driven equity values", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/portfolio/summary", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var summary map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
			t.Fatalf("failed to decode summary: %v", err)
		}

		if summary["initialEquity"] != 50000.0 {
			t.Errorf("expected initialEquity 50000.0, got %v", summary["initialEquity"])
		}
		if summary["totalEquity"] != 50000.0 {
			t.Errorf("expected totalEquity 50000.0, got %v", summary["totalEquity"])
		}
		if summary["targetCorePct"] != 0.60 {
			t.Errorf("expected targetCorePct 0.60, got %v", summary["targetCorePct"])
		}
		if summary["targetAlphaPct"] != 0.40 {
			t.Errorf("expected targetAlphaPct 0.40, got %v", summary["targetAlphaPct"])
		}
		if summary["cash"] != 50000.0 {
			t.Errorf("expected cash 50000.0, got %v", summary["cash"])
		}
	})

	t.Run("GET /api/v1/positions reflects executed trades with leverage and liquidation price", func(t *testing.T) {
		order := trader.OrderRequest{
			Symbol:       "BTC/USDT",
			Bucket:       "ALPHA",
			Side:         "BUY",
			Price:        65000.0,
			PositionSize: 0.1,
			StopLoss:     63000.0,
			TakeProfit:   70000.0,
			Leverage:     5,
		}
		trade, err := execEngine.ExecuteOrder(context.Background(), order)
		if err != nil {
			t.Fatalf("order execution failed: %v", err)
		}
		if trade.Leverage != 5 {
			t.Errorf("expected leverage 5, got %d", trade.Leverage)
		}
		if trade.LiquidationPrice <= 0 || trade.LiquidationPrice >= 65000.0 {
			t.Errorf("expected valid liquidation price below entry for LONG, got %f", trade.LiquidationPrice)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/positions", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		var trades []*db.Trade
		if err := json.Unmarshal(rec.Body.Bytes(), &trades); err != nil {
			t.Fatalf("failed to decode positions: %v", err)
		}
		if len(trades) != 1 {
			t.Fatalf("expected 1 position, got %d", len(trades))
		}
		if trades[0].Symbol != "BTC/USDT" {
			t.Errorf("expected symbol BTC/USDT, got %s", trades[0].Symbol)
		}
	})
}
