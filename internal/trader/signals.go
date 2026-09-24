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

// SignalConfig encapsulates dynamically configured trading parameters loaded from .env.
type SignalConfig struct {
	MinRiskRewardRatio float64
	DefaultLeverage    int
	MinStopLossPct     float64
	MaxStopLossPct     float64
	MinTakeProfitPct   float64
	MaxTakeProfitPct   float64
	MaxRiskPerTradePct float64
	MaxTradeMarginPct  float64 // Max margin per trade as fraction of total equity (e.g. 0.20 = 20%)
}

// DefaultSignalConfig returns baseline parameters. These are used ONLY when
// no .env override is provided. In production, all values come from config.Load().
func DefaultSignalConfig() SignalConfig {
	return SignalConfig{
		MinRiskRewardRatio: 2.5,
		DefaultLeverage:    8,
		MinStopLossPct:     0.6,
		MaxStopLossPct:     2.5,
		MinTakeProfitPct:   1.5,
		MaxTakeProfitPct:   8.0,
		MaxRiskPerTradePct: 0.015,
		MaxTradeMarginPct:  0.20,
	}
}

// SignalService coordinates news-first catalyst signal generation and lifecycle monitoring.
type SignalService struct {
	store    SignalStoreInterface
	aiClient AIAnalyzer
	config   SignalConfig
}

// NewSignalService initializes a new two-sided futures signal service with dynamic configuration.
func NewSignalService(store SignalStoreInterface, aiClient AIAnalyzer, cfgs ...SignalConfig) *SignalService {
	cfg := DefaultSignalConfig()
	if len(cfgs) > 0 {
		provided := cfgs[0]
		if provided.MinRiskRewardRatio > 0 {
			cfg.MinRiskRewardRatio = provided.MinRiskRewardRatio
		}
		if provided.DefaultLeverage > 0 {
			cfg.DefaultLeverage = provided.DefaultLeverage
		}
		if provided.MinStopLossPct > 0 {
			cfg.MinStopLossPct = provided.MinStopLossPct
		}
		if provided.MaxStopLossPct > 0 {
			cfg.MaxStopLossPct = provided.MaxStopLossPct
		}
		if provided.MinTakeProfitPct > 0 {
			cfg.MinTakeProfitPct = provided.MinTakeProfitPct
		}
		if provided.MaxTakeProfitPct > 0 {
			cfg.MaxTakeProfitPct = provided.MaxTakeProfitPct
		}
		if provided.MaxRiskPerTradePct > 0 {
			cfg.MaxRiskPerTradePct = provided.MaxRiskPerTradePct
		}
		if provided.MaxTradeMarginPct > 0 {
			cfg.MaxTradeMarginPct = provided.MaxTradeMarginPct
		}
	}
	return &SignalService{
		store:    store,
		aiClient: aiClient,
		config:   cfg,
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
		totalEquity = 100.0 // Default baseline equity supporting $100 starting accounts
	}
	if availableAlphaCapital <= 0 {
		availableAlphaCapital = totalEquity * 0.40 // 40% Tier 3 Alpha default ($40 on $100 account)
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

	// 4. Calculate protective price bounds (Stop Loss & Take Profit) dynamically
	// Priority: ATR-based from indicator snapshot > AI-suggested percentages > config defaults
	entryPrice := quote.Price
	slPct := aiResp.SuggestedStopLossPct
	tpPct := aiResp.SuggestedTakeProfitPct

	// Use NATR (Normalized ATR %) from indicator snapshot for more accurate volatility-based SL/TP
	if snap.NATR > 0 {
		atrBasedSL := snap.NATR * 1.5 // 1.5x ATR for stop loss
		if atrBasedSL >= s.config.MinStopLossPct && atrBasedSL <= s.config.MaxStopLossPct {
			slPct = atrBasedSL
		}
	}

	if slPct < s.config.MinStopLossPct || slPct > s.config.MaxStopLossPct {
		slPct = s.config.MinStopLossPct
	}
	if tpPct < s.config.MinTakeProfitPct || tpPct > s.config.MaxTakeProfitPct {
		tpPct = slPct * s.config.MinRiskRewardRatio
	}

	// Clamp TP within configured bounds
	if tpPct > s.config.MaxTakeProfitPct {
		tpPct = s.config.MaxTakeProfitPct
	}

	var stopLoss, takeProfit float64
	if dir == DirectionLong {
		stopLoss = entryPrice * (1.0 - (slPct / 100.0))
		takeProfit = entryPrice * (1.0 + (tpPct / 100.0))
	} else {
		stopLoss = entryPrice * (1.0 + (slPct / 100.0))
		takeProfit = entryPrice * (1.0 - (tpPct / 100.0))
	}

	// 5. Validate & Enforce Risk-Reward Ratio dynamically sourced from config
	rr, err := CalculateRiskRewardRatio(entryPrice, stopLoss, takeProfit, dir)
	if err != nil || rr < s.config.MinRiskRewardRatio {
		targetRR := s.config.MinRiskRewardRatio
		riskDist := math.Abs(entryPrice - stopLoss)
		if dir == DirectionLong {
			takeProfit = entryPrice + (targetRR * riskDist)
		} else {
			takeProfit = entryPrice - (targetRR * riskDist)
		}
		rr = targetRR
	}

	// 6. Leverage and Capital Sizing dynamically sourced from config
	leverage := aiResp.Leverage
	if leverage < 1 || leverage > s.config.DefaultLeverage {
		leverage = s.config.DefaultLeverage
	}

	// Dynamic equity risk per trade sourced from config
	maxRiskPct := s.config.MaxRiskPerTradePct
	if maxRiskPct <= 0 {
		maxRiskPct = 0.015
	}
	// Quantity is intentionally discarded: the execution engine derives
	// position size from the final clamped margin (see OpenPositionFromSignal),
	// so the persisted allocation and the live position stay consistent.
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

	// Bound margin required to configured fraction of total equity and within available Alpha capital
	maxMarginPct := s.config.MaxTradeMarginPct
	if maxMarginPct <= 0 {
		maxMarginPct = 0.20
	}
	maxTradeMargin := totalEquity * maxMarginPct
	if marginRequired > maxTradeMargin {
		marginRequired = maxTradeMargin
	}
	if marginRequired > availableAlphaCapital {
		marginRequired = availableAlphaCapital
	}

	allocatedCapitalUSD := marginRequired
	allocatedCapitalPct := (allocatedCapitalUSD / totalEquity) * 100.0

	catalystHeadline := aiResp.Catalyst
	if catalystHeadline == "" && len(headlines) > 0 {
		catalystHeadline = headlines[0]
	}
	if catalystHeadline == "" {
		// Reject signal without a genuine catalyst — prevents fabricated entries
		return nil, nil
	}

	// Estimate catalyst sentiment from AI response
	sentiment := aiResp.Confidence
	if sentiment == 0 {
		if dir == DirectionLong {
			sentiment = 0.6
		} else {
			sentiment = -0.6
		}
	}
	if dir == DirectionShort && sentiment > 0 {
		sentiment = -sentiment
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
