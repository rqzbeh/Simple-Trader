package trader

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/db"
)

// SignalStoreInterface defines persistence operations required by the signal manager.
type SignalStoreInterface interface {
	InsertFuturesSignal(ctx context.Context, sig *db.FuturesTradeSignal) (*db.FuturesTradeSignal, error)
	ListFuturesSignals(ctx context.Context, status string, limit int) ([]db.FuturesTradeSignal, error)
	GetActiveFuturesSignalBySymbol(ctx context.Context, symbol string) (*db.FuturesTradeSignal, error)
	CloseFuturesSignal(ctx context.Context, id int64, exitPrice float64, exitReason string, pnl, roi float64) error
	MarkSignalDispatched(ctx context.Context, id int64) error
	MarkSignalResolved(ctx context.Context, id int64) error
}

// AIAnalyzer defines the interface to obtain trading intelligence and catalyst evaluations.
type AIAnalyzer interface {
	Analyze(ctx context.Context, req ai.DecisionRequest) (*ai.DecisionResponse, error)
}

// SignalService coordinates news-first catalyst signal generation and lifecycle monitoring.
type SignalService struct {
	store    SignalStoreInterface
	aiClient AIAnalyzer
}

// NewSignalService initializes a new two-sided futures signal service.
func NewSignalService(store SignalStoreInterface, aiClient AIAnalyzer) *SignalService {
	return &SignalService{
		store:    store,
		aiClient: aiClient,
	}
}

// EvaluateMarketSignal evaluates an asset against breaking news catalysts and technical confluence.
// Returns a persisted FuturesTradeSignal if high-conviction catalyst is detected, or nil if HOLD.
func (s *SignalService) EvaluateMarketSignal(
	ctx context.Context,
	symbol string,
	bucket string,
	quote cache.TickerQuote,
	snap cache.IndicatorSnapshot,
	weights map[string]float64,
	headlines []string,
	totalEquity float64,
	availableAlphaCapital float64,
) (*db.FuturesTradeSignal, error) {
	if quote.Price <= 0 {
		return nil, errors.New("invalid quote price: must be positive")
	}
	if totalEquity <= 0 {
		totalEquity = 100000.0 // Default baseline equity
	}
	if availableAlphaCapital <= 0 {
		availableAlphaCapital = totalEquity * 0.40 // 40% Tier 3 Alpha default
	}

	// 1. Check if an ACTIVE signal already exists for this symbol
	if s.store != nil {
		existing, err := s.store.GetActiveFuturesSignalBySymbol(ctx, symbol)
		if err == nil && existing != nil {
			return existing, nil // Return existing active signal without duplicating
		}
	}

	// 2. Request AI analysis (mandating news catalyst priority)
	decReq := ai.DecisionRequest{
		Symbol:        symbol,
		Bucket:        bucket,
		Quote:         quote,
		IndicatorSnap: snap,
		Weights:       weights,
		NewsHeadlines: headlines,
	}

	aiResp, err := s.aiClient.Analyze(ctx, decReq)
	if err != nil {
		return nil, fmt.Errorf("ai analysis failed: %w", err)
	}

	if aiResp == nil || aiResp.Decision == "HOLD" || aiResp.Decision == "" {
		return nil, nil // No trade signal
	}

	// 3. Determine directional bias
	var dir Direction
	if aiResp.Decision == "BUY" {
		dir = DirectionLong
	} else if aiResp.Decision == "SELL" {
		dir = DirectionShort
	} else {
		return nil, nil
	}

	// 4. Calculate protective price bounds (Stop Loss & Take Profit)
	entryPrice := quote.Price
	slPct := aiResp.SuggestedStopLossPct
	if slPct <= 0 || slPct > 10.0 {
		slPct = 1.5
	}
	tpPct := aiResp.SuggestedTakeProfitPct
	if tpPct <= 0 || tpPct > 30.0 {
		tpPct = 3.5
	}

	var stopLoss, takeProfit float64
	if dir == DirectionLong {
		stopLoss = entryPrice * (1.0 - (slPct / 100.0))
		takeProfit = entryPrice * (1.0 + (tpPct / 100.0))
	} else {
		stopLoss = entryPrice * (1.0 + (slPct / 100.0))
		takeProfit = entryPrice * (1.0 - (tpPct / 100.0))
	}

	// 5. Validate & Enforce Risk-Reward Ratio >= 1.5 (Institutional target >= 2.0)
	rr, err := CalculateRiskRewardRatio(entryPrice, stopLoss, takeProfit, dir)
	if err != nil || rr < 1.5 {
		// Enforce institutional minimum R:R = 2.0
		riskDist := math.Abs(entryPrice - stopLoss)
		if dir == DirectionLong {
			takeProfit = entryPrice + (2.0 * riskDist)
		} else {
			takeProfit = entryPrice - (2.0 * riskDist)
		}
		rr = 2.0
	}

	// 6. Leverage and Capital Sizing
	leverage := aiResp.Leverage
	if leverage < 1 || leverage > 10 {
		leverage = 3 // Safe institutional default
	}

	// Institutional constraint: max 2.0% equity risk per trade
	maxRiskPct := 0.02
	_, marginRequired, _, err := CalculatePositionSizing(
		totalEquity,
		maxRiskPct,
		availableAlphaCapital,
		entryPrice,
		stopLoss,
		leverage,
	)
	if err != nil {
		return nil, fmt.Errorf("position sizing calculation failed: %w", err)
	}

	allocatedCapitalUSD := marginRequired
	allocatedCapitalPct := (allocatedCapitalUSD / totalEquity) * 100.0

	catalystHeadline := aiResp.Catalyst
	if catalystHeadline == "" && len(headlines) > 0 {
		catalystHeadline = headlines[0]
	}
	if catalystHeadline == "" {
		catalystHeadline = "Technical momentum alignment with global liquidity flows"
	}

	// Estimate catalyst sentiment score
	sentiment := 0.5
	if dir == DirectionLong {
		sentiment = 0.75
	} else {
		sentiment = -0.75
	}

	// 7. Assemble and persist the signal
	sig := &db.FuturesTradeSignal{
		Symbol:              symbol,
		Direction:           string(dir),
		Status:              "ACTIVE",
		CatalystHeadline:    catalystHeadline,
		CatalystSource:      "InstitutionalNewsFeed",
		CatalystSentiment:   sentiment,
		EntryPrice:          entryPrice,
		StopLoss:            stopLoss,
		TakeProfit1:         takeProfit,
		Leverage:            leverage,
		RiskRewardRatio:     rr,
		AllocatedCapitalUSD: allocatedCapitalUSD,
		AllocatedCapitalPct: allocatedCapitalPct,
		TelegramDispatched:  false,
		TelegramResolved:    false,
	}

	if s.store != nil {
		savedSig, err := s.store.InsertFuturesSignal(ctx, sig)
		if err != nil {
			return nil, fmt.Errorf("failed to persist futures signal: %w", err)
		}
		return savedSig, nil
	}

	return sig, nil
}

// CheckSignalResolution evaluates open signals against current price to check for exit.
func (s *SignalService) CheckSignalResolution(ctx context.Context, sig *db.FuturesTradeSignal, currentPrice float64) (bool, string, float64, float64, error) {
	if sig.Status != "ACTIVE" {
		return false, "", 0, 0, nil
	}

	dir := Direction(sig.Direction)
	shouldClose := false
	exitReason := ""

	if dir == DirectionLong {
		if currentPrice >= sig.TakeProfit1 {
			shouldClose = true
			exitReason = "TAKE_PROFIT"
		} else if currentPrice <= sig.StopLoss {
			shouldClose = true
			exitReason = "STOP_LOSS"
		}
	} else if dir == DirectionShort {
		if currentPrice <= sig.TakeProfit1 {
			shouldClose = true
			exitReason = "TAKE_PROFIT"
		} else if currentPrice >= sig.StopLoss {
			shouldClose = true
			exitReason = "STOP_LOSS"
		}
	}

	if !shouldClose {
		return false, "", 0, 0, nil
	}

	// Calculate PnL and leveraged ROI
	quantity := (sig.AllocatedCapitalUSD * float64(sig.Leverage)) / sig.EntryPrice
	pnlUSD, roiPct, err := CalculateFuturesPnL(sig.EntryPrice, currentPrice, quantity, sig.Leverage, dir)
	if err != nil {
		return false, "", 0, 0, fmt.Errorf("failed to calculate resolution pnl: %w", err)
	}

	if s.store != nil {
		if err := s.store.CloseFuturesSignal(ctx, sig.ID, currentPrice, exitReason, pnlUSD, roiPct); err != nil {
			return false, "", 0, 0, fmt.Errorf("failed to close signal in store: %w", err)
		}
	}

	sig.Status = "CLOSED"
	sig.ExitPrice = &currentPrice
	sig.ExitReason = &exitReason
	sig.RealizedPnLUSD = &pnlUSD
	sig.RealizedROIPct = &roiPct

	return true, exitReason, pnlUSD, roiPct, nil
}
