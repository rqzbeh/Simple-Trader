package trader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/market"
)

// SignalStoreInterface defines persistence operations required by the signal manager.
type SignalStoreInterface interface {
	InsertFuturesSignal(ctx context.Context, sig *db.FuturesTradeSignal) (*db.FuturesTradeSignal, error)
	ListFuturesSignals(ctx context.Context, status string, limit int) ([]db.FuturesTradeSignal, error)
	GetActiveFuturesSignalBySymbol(ctx context.Context, symbol string) (*db.FuturesTradeSignal, error)
	CloseFuturesSignal(ctx context.Context, id int64, exitPrice float64, exitReason string, pnl, roi float64) error
	MarkSignalDispatched(ctx context.Context, id int64) error
	MarkSignalResolved(ctx context.Context, id int64) error
	InsertEntryFilterLog(ctx context.Context, symbol, direction string, catalystEventID *int64, rule string, detail json.RawMessage) error
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

// ErrConcurrentCap is returned when MAX_CONCURRENT_SIGNALS would be exceeded.
// Callers map it to a HOLD, not an error: the scan succeeded, the slot was full.
var ErrConcurrentCap = errors.New("max concurrent signals reached")

// SignalService coordinates news-first catalyst signal generation and lifecycle monitoring.
type SignalService struct {
	store    SignalStoreInterface
	aiClient AIAnalyzer
	config   SignalConfig

	// slotGuard runs atomically immediately before persisting a new signal and
	// answers two questions under one lock: does an ACTIVE signal already exist
	// for this symbol, and is there a free slot. The pre-AI checks run before a
	// ~20s model call, so two evaluators both read "free" and both inserted
	// (duplicate XRP signals, and 10 signals against a cap of 5).
	slotGuard func(symbol string) (*db.FuturesTradeSignal, error)
}

// SetSlotGuard installs the concurrency guard evaluated just before persist.
// It returns an existing ACTIVE signal to reuse, ErrConcurrentCap when full,
// or nil to allow the insert.
func (s *SignalService) SetSlotGuard(guard func(symbol string) (*db.FuturesTradeSignal, error)) {
	s.slotGuard = guard
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
// Returns a persisted FuturesTradeSignal if high-conviction catalyst is detected.
// The AI decision is always returned so HOLD explanations reach logs and the API
// instead of being discarded as a silent nil.
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
) (*db.FuturesTradeSignal, *ai.DecisionResponse, error) {
	if quote.Price <= 0 {
		return nil, nil, errors.New("invalid quote price: must be positive")
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
			return existing, nil, nil // Return existing active signal without duplicating
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

	// Pre-compute the NLP sentiment packet so every prompt carries scored
	// evidence (score, polarity, headline mix, trigger phrases) instead of
	// raw headlines alone. Lexicon path is offline and deterministic.
	if len(headlines) > 0 {
		report := market.AnalyzeNewsSentiment(headlines)
		bullish, bearish := 0, 0
		for _, h := range headlines {
			hs := market.AnalyzeNewsSentiment([]string{h})
			switch hs.Polarity {
			case market.PolarityBullish:
				bullish++
			case market.PolarityBearish:
				bearish++
			}
		}
		decReq.NewsSentiment = &ai.NewsSentimentInput{
			Score:         report.Score,
			Polarity:      string(report.Polarity),
			HeadlineCount: report.HeadlineCount,
			BullishCount:  bullish,
			BearishCount:  bearish,
			KeyPhrases:    report.KeyPhrases,
		}
	}

	// The AI call and the insert each get their own time budget. Sharing one
	// context meant a slow model consumed the evaluation window and the INSERT
	// then failed with "context deadline exceeded" after the answer had already
	// arrived.
	aiCtx, aiCancel := context.WithTimeout(ctx, 20*time.Second)
	aiResp, err := s.aiClient.Analyze(aiCtx, decReq)
	aiCancel()
	if err != nil {
		return nil, nil, fmt.Errorf("ai analysis failed: %w", err)
	}

	if aiResp == nil || aiResp.Decision == "HOLD" || aiResp.Decision == "" {
		return nil, aiResp, nil // No trade signal; carry the reasoning out
	}

	// 3. Determine directional bias
	var dir Direction
	if aiResp.Decision == "BUY" {
		dir = DirectionLong
	} else if aiResp.Decision == "SELL" {
		dir = DirectionShort
	} else {
		return nil, aiResp, nil
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
		if tpPct > s.config.MaxTakeProfitPct {
			// Keep the R:R gate satisfiable within the TP ceiling by
			// tightening SL instead of emitting an under-R:R signal.
			slPct = s.config.MaxTakeProfitPct / s.config.MinRiskRewardRatio
			if slPct < s.config.MinStopLossPct {
				slPct = s.config.MinStopLossPct
			}
			tpPct = slPct * s.config.MinRiskRewardRatio
		}
	}

	// Safety net: TP never exceeds the configured ceiling here. If SL cannot
	// shrink enough to hold R:R within bounds, section 5 stretches TP past
	// the ceiling and the R:R floor wins as the hard gate.
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
		return nil, aiResp, fmt.Errorf("position sizing calculation failed: %w", err)
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
		// Reject signal without a genuine catalyst — prevents fabricated entries.
		// The decision is returned so the operator sees WHY the trade was refused.
		return nil, aiResp, nil
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
	// Decision-time indicator snapshot (spec 012 US7, FR-022): recorded once
	// here so closed-trade outcomes attribute to the indicators that were
	// actually bold in THIS decision, not to whatever the market shows later.
	snapRecord, _ := json.Marshal(db.IndicatorSnapshotRecord{
		RSI:           snap.RSI,
		MACDHistogram: snap.Histogram,
		SuperTrend:    snap.SuperTrend,
		CMF:           snap.CMF,
		KaufmanER:     snap.KaufmanER,
		OBI:           snap.OBI,
		Divergence:    snap.Divergence,
	})
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
		IndicatorSnapshot:   snapRecord,
	}

	// Entry gate (spec 012 US1 / research R1): veto candidates that fail
	// trend, volume, chase, OI or liquidation-buffer checks BEFORE they are
	// persisted. Rejections land in entry_filter_log (FR-020) and surface via
	// DecisionResponse.GateRejected so the server can broadcast them.
	gateProfile := config.GetRiskProfile("CRYPTO")
	if bucket == "CORE" {
		gateProfile = config.GetRiskProfile("COMMODITY")
	}
	var oiPtr *float64
	if oiDelta, oiErr := market.NewBinanceFetcher().FetchOIDeltaPct(ctx, symbol); oiErr == nil {
		oiPtr = oiDelta
	}
	gateIn := EntryGateInput{
		Symbol:               symbol,
		Direction:            string(dir),
		Price:                entryPrice,
		VWAP:                 snap.VWAP,
		UpperBand:            snap.UpperBand,
		LowerBand:            snap.LowerBand,
		MidBand:              snap.MiddleBand,
		SuperTrend:           snap.SuperTrend,
		VolumeRatio:          snap.VolumeRatio,
		OIDeltaPct:           oiPtr,
		SlPct:                slPct,
		Leverage:             leverage,
		LiqBufferMin:         gateProfile.LiqBufferMin,
		MaintenanceMarginPct: 0,
	}
	if gate := EvaluateEntryGate(gateIn); !gate.Allowed {
		detail, _ := json.Marshal(gate.Detail)
		if s.store != nil {
			logCtx, logCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			_ = s.store.InsertEntryFilterLog(logCtx, symbol, string(dir), nil, gate.Rule, detail)
			logCancel()
		}
		aiResp.GateRejected = gate.Rule
		aiResp.GateRejectedDetail = gate.Detail
		prefix := "[entry gate: " + gate.Rule + "] "
		if aiResp.Reasoning == "" {
			aiResp.Reasoning = prefix + "candidate rejected"
		} else {
			aiResp.Reasoning = prefix + aiResp.Reasoning
		}
		return nil, aiResp, nil
	}

	if s.store != nil {
		if s.slotGuard != nil {
			existing, err := s.slotGuard(sig.Symbol)
			if err != nil {
				return nil, aiResp, err
			}
			if existing != nil {
				return existing, aiResp, nil // another evaluator won the race
			}
		}
		insertCtx, insertCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		savedSig, err := s.store.InsertFuturesSignal(insertCtx, sig)
		insertCancel()
		if err != nil {
			return nil, aiResp, fmt.Errorf("failed to persist futures signal: %w", err)
		}
		return savedSig, aiResp, nil
	}

	return sig, aiResp, nil
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

	pnlUSD, roiPct, err := s.settleAndClose(ctx, sig, currentPrice, exitReason)
	if err != nil {
		return false, "", 0, 0, err
	}
	return true, exitReason, pnlUSD, roiPct, nil
}

// CloseSignalNow closes an ACTIVE signal unconditionally at the given price.
//
// CheckSignalResolution is level-triggered: it returns resolved=false whenever
// price has not crossed SL/TP. A manual close is not level-triggered — the
// operator asked to exit — so relying on it left the row ACTIVE while the
// execution engine position was already gone.
func (s *SignalService) CloseSignalNow(ctx context.Context, sig *db.FuturesTradeSignal, exitPrice float64, exitReason string) (float64, float64, error) {
	if sig == nil || sig.Status != "ACTIVE" {
		return 0, 0, nil // already closed: keep the operation idempotent
	}
	if exitPrice <= 0 {
		return 0, 0, fmt.Errorf("exit price must be positive for %s", sig.Symbol)
	}
	if exitReason == "" {
		exitReason = "MANUAL_EXIT"
	}
	return s.settleAndClose(ctx, sig, exitPrice, exitReason)
}

// settleAndClose prices the exit, persists it and marks the signal closed.
// Callers must hold an ACTIVE signal.
func (s *SignalService) settleAndClose(ctx context.Context, sig *db.FuturesTradeSignal, exitPrice float64, exitReason string) (float64, float64, error) {
	dir := Direction(sig.Direction)
	quantity := (sig.AllocatedCapitalUSD * float64(sig.Leverage)) / sig.EntryPrice
	pnlUSD, roiPct, err := CalculateFuturesPnL(sig.EntryPrice, exitPrice, quantity, sig.Leverage, dir)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to calculate resolution pnl: %w", err)
	}

	if s.store != nil {
		if err := s.store.CloseFuturesSignal(ctx, sig.ID, exitPrice, exitReason, pnlUSD, roiPct); err != nil {
			return 0, 0, fmt.Errorf("failed to close signal in store: %w", err)
		}
	}

	sig.Status = "CLOSED"
	sig.ExitPrice = &exitPrice
	sig.ExitReason = &exitReason
	sig.RealizedPnLUSD = &pnlUSD
	sig.RealizedROIPct = &roiPct

	return pnlUSD, roiPct, nil
}
