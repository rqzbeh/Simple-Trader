package trader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
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
	UpdateSignalDecay(ctx context.Context, id int64, state string) error
	UpdateSignalStop(ctx context.Context, id int64, newStop float64) error
}

// AIAnalyzer defines the interface to obtain trading intelligence and catalyst evaluations.
type AIAnalyzer interface {
	Analyze(ctx context.Context, req ai.DecisionRequest) (*ai.DecisionResponse, error)
}

// CatalystMeta carries the clustered Catalyst Event for one evaluation
// (spec 012 US3, T034/T035): the dominant story after syndicated
// deduplication, with its fused (tier-weighted) sentiment, story count and
// freshness. Optional — evaluations without news pass nothing.
type CatalystMeta struct {
	EventID        int64    // catalyst_events row (0 = not persisted)
	Headline       string   // representative headline
	StoryCount     int      // syndicated coverage count
	FusedSentiment float64  // source-tier-weighted fused sentiment (-1..1)
	Freshness      float64  // half-life weight (0..1)
	Sources        []string // contributing feeds (authority tiers)
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

// SampleSignalConfig builds a SignalConfig from config.SampleRequiredEnv —
// bootstrap/test sample ONLY (spec-017 FR-409). Production values come from
// config.Load via SetAppConfig; there is no runtime default substitution.
func SampleSignalConfig() SignalConfig {
	m := config.SampleRequiredEnv()
	f := func(k string) float64 { v, _ := strconv.ParseFloat(m[k], 64); return v }
	return SignalConfig{
		MinRiskRewardRatio: 2.5, // spec-015 optional key — sample value for tests
		DefaultLeverage:    8,   // spec-015 optional key — sample value for tests
		MinStopLossPct:     f("MIN_STOP_LOSS_PCT"),
		MaxStopLossPct:     f("MAX_STOP_LOSS_PCT"),
		MinTakeProfitPct:   f("MIN_TAKE_PROFIT_PCT"),
		MaxTakeProfitPct:   f("MAX_TAKE_PROFIT_PCT"),
		MaxRiskPerTradePct: 0.015, // spec-015 optional key — sample value for tests
		MaxTradeMarginPct:  f("MAX_TRADE_MARGIN_PCT"),
	}
}

// ErrConcurrentCap is returned when MAX_CONCURRENT_SIGNALS would be exceeded.
// Callers map it to a HOLD, not an error: the scan succeeded, the slot was full.
var ErrConcurrentCap = errors.New("max concurrent signals reached")

// profileNameForBucket maps an asset bucket to its risk profile name
// (spec 012 US2: CRYPTO intraday, COMMODITY multi-hour). Exported: the
// decay reconciler resolves the profile from the signal's own class.
func ProfileNameForBucket(bucket string) string {
	if bucket == "CORE" {
		return "COMMODITY"
	}
	return "CRYPTO"
}

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

	// newsClassify classifies headlines via the decision core. Explicit-error
	// contract: never returns a fabricated neutral (spec-013 FR-007).
	newsClassify market.NewsClassifier

	// router is the Jev-first/9Router-escalated decision core (spec-013 FR-001/003).
	router *DecisionRouter
	// shadow records decisions post-commit (spec-013 FR-002).
	shadow *ShadowOrchestrator

	// appConfig holds global configuration including Jev managed param overrides (spec-015).
	appConfig *config.Config
}

// SetAppConfig injects global configuration.
func (s *SignalService) SetAppConfig(cfg *config.Config) { s.appConfig = cfg }

func (s *SignalService) getAppConfig() *config.Config {
	if s.appConfig != nil {
		return s.appConfig
	}
	if cfg, err := config.Load(); err == nil {
		return cfg
	}
	// spec-017 FR-409: no hardcoded fallback values. In production SetAppConfig
	// always ran (boot is fatal without config); in tests a zero Config means
	// all spec-015 params stay core_managed.
	return &config.Config{}
}

// SetDecisionRouter installs the spec-013 decision core for entry judgments.
func (s *SignalService) SetDecisionRouter(r *DecisionRouter) { s.router = r }

// SetShadow installs the post-commit shadow recorder.
func (s *SignalService) SetShadow(o *ShadowOrchestrator) { s.shadow = o }

// SetNewsClassifier injects the core-backed headline classifier.
func (s *SignalService) SetNewsClassifier(fn market.NewsClassifier) {
	s.newsClassify = fn
}

// SetSlotGuard installs the concurrency guard evaluated just before persist.
// It returns an existing ACTIVE signal to reuse, ErrConcurrentCap when full,
// or nil to allow the insert.
func (s *SignalService) SetSlotGuard(guard func(symbol string) (*db.FuturesTradeSignal, error)) {
	s.slotGuard = guard
}

// NewSignalService initializes a new two-sided futures signal service with dynamic configuration.
func NewSignalService(store SignalStoreInterface, aiClient AIAnalyzer, cfgs ...SignalConfig) *SignalService {
	cfg := SampleSignalConfig()
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
	preGates map[string]string,
	catalysts ...CatalystMeta,
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
	// Profile horizon rides into the prompt (US4 T040): the model judges
	// catalysts against the ACTUAL holding frame (4h CORE vs 1h ALPHA)
	// instead of the historical hardcoded 2-hour swing language.
	decProf, decProfErr := EffectiveProfile(ProfileNameForBucket(bucket))
	horizonMin := 60
	if decProfErr == nil && decProf.HorizonMin > 0 {
		horizonMin = decProf.HorizonMin
	}
	// Real pre-AI gate outcomes (spec-018 FR-501) + facts this layer owns.
	gates := make(map[string]string, len(preGates)+3)
	for k, v := range preGates {
		gates[k] = v
	}
	if len(headlines) > 0 {
		gates["catalyst_headlines"] = "present"
	} else {
		gates["catalyst_headlines"] = "absent"
	}
	gates["capital_available"] = "clear"

	decReq := ai.DecisionRequest{
		Symbol:         symbol,
		Bucket:         bucket,
		Quote:          quote,
		IndicatorSnap:  snap,
		Weights:        weights,
		NewsHeadlines:  headlines,
		HorizonMinutes: horizonMin,
		Gates:          gates,
	}

	// Core-classified sentiment packet (semantic, Jev+9Router). Failure is an
	// explicit error — no lexicon fallback exists (spec-013 FR-013).
	if len(headlines) > 0 {
		classify := s.newsClassify
		if classify == nil {
			classify = market.DefaultClassifier
		}
		report, err := classify(headlines)
		if err != nil {
			return nil, nil, fmt.Errorf("component=news-classifier cycle=%s: %w", symbol, err)
		}
		bullish, bearish := 0, 0
		switch report.Polarity {
		case market.PolarityBullish:
			bullish = len(headlines)
		case market.PolarityBearish:
			bearish = len(headlines)
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

	// Clustered catalyst event (US3 T035/T036): fused sentiment and story
	// count feed the prompt AND the persisted signal — sentiment and model
	// confidence are stored as separate values (FR-012).
	var catMeta *CatalystMeta
	if len(catalysts) > 0 {
		gates["catalyst_cluster"] = "present"
		decReq.Gates = gates
		catMeta = &catalysts[0]
		decReq.CatalystEvents = []ai.CatalystEventInput{{
			Headline:   catMeta.Headline,
			StoryCount: catMeta.StoryCount,
			FusedScore: catMeta.FusedSentiment,
			Freshness:  catMeta.Freshness,
			Sources:    catMeta.Sources,
		}}
	}

	// The AI call and the insert each get their own time budget. Sharing one
	// context meant a slow model consumed the evaluation window and the INSERT
	// then failed with "context deadline exceeded" after the answer had already
	// arrived.
	var aiResp *ai.DecisionResponse
	if s.router != nil {
		// Decision core: Jev-first, 9Router-escalated (FR-001/FR-003).
		aiCtx, aiCancel := context.WithTimeout(ctx, 20*time.Second)
		coreDecision, rerr := s.judgeEntryCore(aiCtx, symbol, decReq)
		aiCancel()
		if rerr != nil {
			return nil, nil, rerr
		}
		aiResp = coreDecision
	} else {
		aiCtx, aiCancel := context.WithTimeout(ctx, 20*time.Second)
		var err error
		aiResp, err = s.aiClient.Analyze(aiCtx, decReq)
		aiCancel()
		if err != nil {
			return nil, nil, fmt.Errorf("ai analysis failed: %w", err)
		}
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

	// 4. Calculate protective price bounds from measured volatility (spec 012
	// US2, FR-002/003): ATR stop behind the 15m swing, staged targets from
	// the profile. Priority: profile ATR model > AI-suggested percentages >
	// config defaults. ATR at entry is persisted for the audit (FR-002).
	entryPrice := quote.Price
	slPct := aiResp.SuggestedStopLossPct
	var atrPrice float64
	if snap.NATR > 0 {
		atrPrice = snap.NATR * entryPrice / 100.0 // NATR% -> absolute ATR
	}

	profile, profErr := EffectiveProfile(ProfileNameForBucket(bucket))
	if profErr != nil {
		return nil, aiResp, fmt.Errorf("risk profile for %s: %w", bucket, profErr)
	}
	if aiResp != nil && aiResp.Timeframe != "" {
		if tfProf, ok := config.GetTimeframeProfile(aiResp.Timeframe); ok {
			profile.HorizonMin = tfProf.HorizonMin
			profile.DecayBreakevenAtMin = tfProf.BEOffsetMin
			profile.DecayFlatAtMin = tfProf.FlatOffsetMin
		}
	}

	stopLoss, slPctFromATR := CalculateATRStop(entryPrice, 0, 0, atrPrice, dir, profile)
	slPct = slPctFromATR

	// Managed trade parameters resolution & dynamic adjustments (spec-015)
	var paramValues map[string]interface{}
	var paramClamps map[string]interface{}
	if aiResp != nil {
		if len(aiResp.ParameterValues) > 0 {
			_ = json.Unmarshal(aiResp.ParameterValues, &paramValues)
		}
		if len(aiResp.ParameterClamps) > 0 {
			_ = json.Unmarshal(aiResp.ParameterClamps, &paramClamps)
		}
	}
	if paramClamps == nil {
		paramClamps = make(map[string]interface{})
	}
	if paramValues == nil {
		reg := NewParamRegistry(s.getAppConfig())
		resolved, _ := reg.ResolveAll(nil, "fallback")
		modesJSON, valsJSON, distsJSON, clampsJSON, _ := PackageParamRecord(resolved)
		if aiResp != nil {
			aiResp.ParameterModes = modesJSON
			aiResp.ParameterValues = valsJSON
			aiResp.ParameterDistributions = distsJSON
			aiResp.ParameterClamps = clampsJSON
		}
		_ = json.Unmarshal(valsJSON, &paramValues)
	}

	// Confluence acceptance gate (spec-015 FR-302)
	if confVal, ok := paramValues["confluence"]; ok {
		var confMin float64
		if cf, ok := confVal.(float64); ok {
			confMin = cf
		}
		if confMin > 0 && snap.ConfluenceScore > 0 && snap.ConfluenceScore < confMin {
			aiResp.GateRejected = "confluence_threshold"
			aiResp.GateRejectedDetail = map[string]interface{}{
				"confluence_score": snap.ConfluenceScore,
				"confluence_min":   confMin,
			}
			prefix := "[entry gate: confluence_threshold] "
			if aiResp.Reasoning == "" {
				aiResp.Reasoning = prefix + "confluence below threshold"
			} else {
				aiResp.Reasoning = prefix + aiResp.Reasoning
			}
			return nil, aiResp, nil
		}
	}

	// ATR regime adjustment (spec-015 FR-302)
	if atrVal, ok := paramValues["atr_regime"]; ok {
		if m, ok := atrVal.(map[string]interface{}); ok {
			if slm, ok := m["sl_atr_mult"].(float64); ok && slm > 0 {
				profile.SLAtrMult = slm
			}
			if tpm, ok := m["tp_atr_mult"].(float64); ok && tpm > 0 {
				profile.TP1AtrMult = tpm
			}
		}
	}

	tp1Price, tp2Price, closeFrac := CalculateStagedTargets(entryPrice, atrPrice, dir, profile)

	effectiveMinRR := s.config.MinRiskRewardRatio
	if rrVal, ok := paramValues["min_rr"]; ok {
		if rrF, ok := rrVal.(float64); ok && rrF > 0 {
			effectiveMinRR = rrF
		}
	}

	// No measured ATR: stretch TP1 to at least the configured minimum R:R
	// against the effective stop so the fallback target stays reachable and
	// the R:R contract holds for legacy snapshots.
	if atrPrice <= 0 {
		riskDist := math.Abs(entryPrice - stopLoss)
		minReward := effectiveMinRR * riskDist
		if math.Abs(tp1Price-entryPrice) < minReward {
			if dir == DirectionLong {
				tp1Price = entryPrice + minReward
			} else {
				tp1Price = entryPrice - minReward
			}
		}
		if math.Abs(tp2Price-entryPrice) < minReward*1.5 {
			if dir == DirectionLong {
				tp2Price = entryPrice + minReward*1.5
			} else {
				tp2Price = entryPrice - minReward*1.5
			}
		}
	}

	// AI-suggested stop respected only when it widens inside the profile clamp
	// and beats the structural stop distance (defense in depth, never a floor
	// override of the ATR model).
	if aiSl := aiResp.SuggestedStopLossPct; aiSl > slPct && aiSl <= profile.SLMaxPct {
		slPct = aiSl
		if dir == DirectionLong {
			stopLoss = entryPrice * (1.0 - slPct/100.0)
		} else {
			stopLoss = entryPrice * (1.0 + slPct/100.0)
		}
	}

	// 5. Leverage: vol-target formula from the profile (FR-006) when a
	// volatility measurement exists; AI suggestion respected only when it
	// lowers leverage inside the cap. No measurement -> AI/config leverage.
	effectiveDefaultLev := s.config.DefaultLeverage
	if levVal, ok := paramValues["leverage"]; ok {
		if levF, ok := levVal.(float64); ok {
			effectiveDefaultLev = int(levF)
		} else if levI, ok := levVal.(int); ok {
			effectiveDefaultLev = levI
		}
	}
	leverage := aiResp.Leverage
	if leverage < 1 {
		leverage = effectiveDefaultLev
	}
	if snap.NATR > 0 {
		volTargetLev := CalculateVolTargetLeverage(snap.NATR*100.0, effectiveDefaultLev, profile)
		if leverage > volTargetLev {
			leverage = volTargetLev
		}
	} else if leverage > effectiveDefaultLev {
		leverage = effectiveDefaultLev
	}

	// Liquidation-buffer invariant (FR-006): stop must sit far enough from the
	// liquidation price at the chosen leverage, else widen leverage down.
	origLev := leverage
	if !LiquidationBufferOK(entryPrice, slPct, leverage, dir, profile) {
		for leverage > 1 {
			leverage--
			if LiquidationBufferOK(entryPrice, slPct, leverage, dir, profile) {
				break
			}
		}
		if origLev != leverage {
			paramClamps["leverage"] = map[string]interface{}{
				"requested": origLev,
				"applied":   leverage,
				"bound":     "liquidation_buffer",
			}
			if aiResp != nil {
				aiResp.ParameterClamps, _ = json.Marshal(paramClamps)
			}
		}
	}

	// 6. Capital sizing: fixed-fractional risk with slippage buffer (FR-007).
	// Quantity is intentionally discarded: the execution engine derives
	// position size from the final clamped margin (see OpenPositionFromSignal),
	// so the persisted allocation and the live position stay consistent.
	maxRiskPct := profile.RiskPerTradePct
	if convVal, ok := paramValues["conviction"]; ok {
		if convF, ok := convVal.(float64); ok && convF > 0 {
			maxRiskPct = convF
		}
	}
	if maxRiskPct <= 0 {
		maxRiskPct = s.config.MaxRiskPerTradePct
	}
	if maxRiskPct <= 0 {
		return nil, aiResp, ai.WrapDecision("config", symbol, ai.ErrConfigMissing,
			"MAX_RISK_PER_TRADE_PCT zero/absent and no conviction answer (spec-017: no in-code fallback)")
	}
	// Quantity is intentionally discarded: the execution engine derives
	// position size from the final clamped margin (see OpenPositionFromSignal),
	// so the persisted allocation and the live position stay consistent.
	_, marginRequired, _, err := CalculatePositionSizingWithSlippage(
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

	// US3 T035 (FR-012): fused cluster sentiment replaces the raw confidence
	// heuristic when a Catalyst Event exists; model confidence is stored as
	// its own column.
	modelConfidence := aiResp.Confidence
	if catMeta != nil {
		sentiment = catMeta.FusedSentiment
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
	// Risk/reward from the staged structure. The ATR-derived TP1 keeps its
	// measured reachability, but the effective minimum R:R still wins as the
	// hard gate when the final stop distance makes the measured target too
	// tight (e.g. no measured ATR + a wide AI stop): TP1 stretches, RR floor
	// holds (FR-003 contract, plan G2).
	rr, rrErr := CalculateRiskRewardRatio(entryPrice, stopLoss, tp1Price, dir)
	if rrErr != nil || rr < effectiveMinRR {
		riskDist := math.Abs(entryPrice - stopLoss)
		if dir == DirectionLong {
			tp1Price = entryPrice + effectiveMinRR*riskDist
		} else {
			tp1Price = entryPrice - effectiveMinRR*riskDist
		}
		rr = effectiveMinRR
	}

	sig := &db.FuturesTradeSignal{
		Symbol:              symbol,
		Direction:           string(dir),
		Status:              "ACTIVE",
		CatalystHeadline:    catalystHeadline,
		CatalystSource:      "InstitutionalNewsFeed",
		CatalystSentiment:   sentiment,
		EntryPrice:          entryPrice,
		StopLoss:            stopLoss,
		TakeProfit1:         tp1Price,
		TakeProfit2:         &tp2Price,
		Leverage:            leverage,
		RiskRewardRatio:     rr,
		AllocatedCapitalUSD: allocatedCapitalUSD,
		AllocatedCapitalPct: allocatedCapitalPct,
		TelegramDispatched:  false,
		TelegramResolved:    false,
		IndicatorSnapshot:   snapRecord,
		// FR-012: confidence is NOT sentiment — model confidence always
		// recorded separately; catalyst_sentiment carries the fused cluster
		// score when a Catalyst Event exists, else the heuristic above.
		ModelConfidence:  &modelConfidence,
		ATRAtEntry:       &atrPrice,
		TP1CloseFraction: &closeFrac,
	}

	if aiResp != nil {
		if aiResp.Timeframe != "" {
			tf := aiResp.Timeframe
			sig.Timeframe = &tf
			sig.TimeframeConfidence = &aiResp.TimeframeConfidence
			if len(aiResp.TimeframeDistribution) > 0 {
				distBytes, _ := json.Marshal(aiResp.TimeframeDistribution)
				sig.TimeframeDistribution = distBytes
			}
		}
		if len(aiResp.ParameterModes) > 0 {
			sig.ParameterModes = aiResp.ParameterModes
		}
		if len(aiResp.ParameterValues) > 0 {
			sig.ParameterValues = aiResp.ParameterValues
		}
		if len(aiResp.ParameterDistributions) > 0 {
			sig.ParameterDistributions = aiResp.ParameterDistributions
		}
		if len(aiResp.ParameterClamps) > 0 {
			sig.ParameterClamps = aiResp.ParameterClamps
		}
	}
	if len(sig.ParameterModes) == 0 {
		reg := NewParamRegistry(s.getAppConfig())
		resolved, _ := reg.ResolveAll(nil, "default")
		modesJSON, valsJSON, distsJSON, clampsJSON, _ := PackageParamRecord(resolved)
		sig.ParameterModes = modesJSON
		sig.ParameterValues = valsJSON
		sig.ParameterDistributions = distsJSON
		sig.ParameterClamps = clampsJSON
	}

	if catMeta != nil && catMeta.EventID > 0 {
		sig.CatalystEventID = &catMeta.EventID // links signal -> catalyst_events (T034)
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

// judgeEntryCore runs the decision core (Jev first → optional 9Router
// escalation) and maps the typed outcome onto the legacy DecisionResponse
// consumed by signal assembly (position intent → BUY/SELL mapping unchanged).
// Timeframe choice rides the same batched request (spec-016 FR-201).
func (s *SignalService) judgeEntryCore(ctx context.Context, symbol string, req ai.DecisionRequest) (*ai.DecisionResponse, error) {
	// spec-018 FR-501: evidence-complete state from the REAL request data
	// (headlines, catalysts, buckets, hydrated gates). The old builder fed a
	// headline COUNT and fabricated ATR levels — both deleted.
	state, err := BuildEntryState(symbol, time.Now().UTC().Format(time.RFC3339), req, req.Gates)
	if err != nil {
		return nil, err
	}
	cycle := fmt.Sprintf("%s-%d", symbol, time.Now().UnixNano())
	bucket := req.Bucket
	if bucket == "" {
		bucket = "ALPHA"
	}
	questions := EntryQuestions(bucket)
	appCfg := s.getAppConfig()
	managedQs := BuildManagedEntryQuestions(appCfg, map[string]interface{}{
		"symbol":     symbol,
		"confluence": req.IndicatorSnap.ConfluenceScore,
		"atr_pct":    req.IndicatorSnap.NATR,
	})
	for qk, q := range managedQs {
		questions[qk] = q
	}

	thr := s.router.GetThreshold()
	if thr <= 0 {
		return nil, ai.WrapDecision("config", cycle, ErrThresholdMissing, "ROUTING_CONFIDENCE_THRESHOLD")
	}

	answers, _, err := s.router.Jev.Evaluate(ctx, cycle, state, questions)
	if err != nil {
		if strings.Contains(err.Error(), "timeframe") {
			return nil, ai.WrapDecision("timeframe", cycle, ai.ErrJevSchema, "missing or invalid timeframe answer in batch: "+err.Error())
		}
		if strings.Contains(err.Error(), "component=managed-params") {
			return nil, err
		}
		return nil, err
	}

	// Validate timeframe choice ∈ bucket set (FR-202, SC-202). No default.
	bucketSet := config.GetBucketTimeframeSet(bucket)
	tfAns, ok := answers["timeframe"]
	if !ok {
		return nil, ai.WrapDecision("timeframe", cycle, ai.ErrJevSchema, "missing timeframe answer in batch")
	}
	if err := ValidateTimeframe(tfAns, bucketSet, cycle); err != nil {
		return nil, err
	}

	// spec-018 FR-502/503: validate the split answers, then compose.
	dirAns, ok := answers["direction"]
	if !ok {
		return nil, ai.WrapDecision("direction", cycle, ai.ErrJevSchema, "missing direction answer in batch")
	}
	if err := ai.ValidateChoice(dirAns, directionVocab, cycle); err != nil {
		return nil, err
	}
	edgeAns, ok := answers["edge"]
	if !ok || edgeAns.Noul == nil {
		return nil, ai.WrapDecision("edge", cycle, ai.ErrJevSchema, "missing edge noul answer in batch")
	}
	edgeProb := *edgeAns.Noul
	if edgeProb < 0 || edgeProb > 1 {
		return nil, ai.WrapDecision("edge", cycle, ai.ErrJevSchema, fmt.Sprintf("edge noul out of range: %v", edgeProb))
	}
	composedChoice := "NO_TRADE"
	composedConf := 1 - edgeProb
	if edgeProb >= edgeConfirmFloor {
		composedChoice = dirAns.Choice
		composedConf = math.Min(dirAns.Confidence, edgeProb)
	}

	// Resolve managed trade parameters (spec-015 FR-302, FR-307). Entry batch
	// carries 5 param questions; decay is answered per cluster by the news
	// path and folded into the record by ResolveEntry.
	reg := NewParamRegistry(appCfg)
	resolved, err := reg.ResolveEntry(answers, cycle)
	if err != nil {
		return nil, err
	}
	modesJSON, valsJSON, distsJSON, clampsJSON, err := PackageParamRecord(resolved)
	if err != nil {
		return nil, fmt.Errorf("component=managed-params cycle=%s: package params: %w", cycle, err)
	}

	entryChoice := composedChoice
	entryConf := composedConf
	route := "jev_direct"
	baseline := ""

	if composedConf < thr {
		if s.router.Escalate == nil {
			return nil, ai.WrapDecision("router", cycle, ai.ErrLLMClassify, "confidence below threshold but no escalation path configured")
		}
		// Pass the full request: escalation judges the same symbols,
		// indicators and headlines the entry cycle saw (spec-013 FR-003).
		esc, err := s.router.Escalate(ctx, req)
		if err != nil {
			return nil, ai.WrapDecision("router", cycle, ai.ErrLLMClassify, "escalation failed: "+err.Error())
		}
		if !entryVocab[esc.Choice] {
			return nil, ai.WrapDecision("router", cycle, ai.ErrJevSchema, "escalated choice outside vocabulary: "+esc.Choice)
		}
		// spec-018 FR-504: trust the NUMBERS — the higher-confidence answer
		// wins. A 0.15 HOLD must not silently beat a 0.76 direction.
		baseline = composedChoice
		if esc.Confidence > composedConf {
			entryChoice = esc.Choice
			entryConf = esc.Confidence
			route = "escalated"
		} else {
			entryChoice = composedChoice
			entryConf = composedConf
			route = "escalated_jev_kept"
		}
	}

	// Decision telemetry (2026-09-29 research): both brains, the threshold,
	// and feed sizes in one line — the evidence trail for confidence work.
	log.Printf("[DECISION] cycle=%s route=%s jev=%s/%.2f edge=%.2f final=%s/%.2f thr=%.2f batch_q=%d headlines=%d",
		cycle, route, composedChoice, composedConf, edgeProb, entryChoice, entryConf, thr, len(questions), len(req.NewsHeadlines))

	// Position intent only — execution layer maps to order sides (FR-005).
	decision := "HOLD"
	switch entryChoice {
	case "LONG":
		decision = "BUY"
	case "SHORT":
		decision = "SELL"
	}
	resp := &ai.DecisionResponse{
		Decision:               decision,
		Confidence:             entryConf,
		Reasoning:              fmt.Sprintf("route=%s", route),
		Timeframe:              tfAns.Choice,
		TimeframeDistribution:  tfAns.Probabilities,
		TimeframeConfidence:    tfAns.Confidence,
		ParameterModes:         modesJSON,
		ParameterValues:        valsJSON,
		ParameterDistributions: distsJSON,
		ParameterClamps:        clampsJSON,
	}
	if s.shadow != nil {
		s.shadow.JudgeEntry(cycle, symbol, state, questions, directionVocab, baseline)
	}
	return resp, nil
}

var entryVocab = map[string]bool{"LONG": true, "SHORT": true, "NO_TRADE": true}

// directionVocab: the relative direction Choice — deliberately WITHOUT
// NO_TRADE (docs: a catch-all option absorbs probability and starves
// confidence; NO_TRADE is composed in code, FR-503).
var directionVocab = map[string]bool{"LONG": true, "SHORT": true}

// edgeConfirmFloor: probability at which the absolute edge Noul confirms a
// setup (0.5 = more likely than not). Policy constant (code), not tunable
// config — documented in TradingPolicyText.
const edgeConfirmFloor = 0.5

// snapToMap flattens the snapshot into context fields (pre-computed — the
// core never does arithmetic, research.md rule).
func snapToMap(snap cache.IndicatorSnapshot) map[string]float64 {
	return map[string]float64{
		"rsi": snap.RSI, "macd": snap.MACD, "macd_signal": snap.Signal,
		"macd_histogram": snap.Histogram, "bb_upper": snap.UpperBand,
		"bb_middle": snap.MiddleBand, "bb_lower": snap.LowerBand,
		"confluence": snap.ConfluenceScore, "obi": snap.OBI,
	}
}

// EntryQuestions: one batched Jev request per cycle (research.md batch rule, spec-016).
func EntryQuestions(bucketOpt ...string) map[string]ai.JevQuestion {
	bucket := "ALPHA"
	if len(bucketOpt) > 0 && bucketOpt[0] != "" {
		bucket = bucketOpt[0]
	}
	// spec-018 FR-502: the mega-choice is split per TypeSafe docs — a
	// RELATIVE direction Choice (no NO_TRADE attractor, no numeric clauses,
	// rubrics reference only state fields that exist) plus an ABSOLUTE edge
	// Noul. Code composes NO_TRADE (FR-503).
	return map[string]ai.JevQuestion{
		"direction": {
			Type: "choice",
			Instructions: map[string]interface{}{
				"question": "Which direction do `headlines`, `catalysts`, `semantic` and `indicators` support for this setup?",
				"not_for":  "trade viability — the `edge` answer decides whether to trade; order sides belong to the execution layer",
				"evidence": "`headlines`, `catalysts`, `semantic.supertrend`, `semantic.trend`, `semantic.confluence_band`, `indicators`",
			},
			Criteria: map[string]interface{}{
				"LONG": map[string]interface{}{
					"covers":   "upward directional opportunity the visible evidence supports",
					"basis":    "`semantic.supertrend` bull or `semantic.trend` above_vwap, agreeing with supportive `catalysts` and `headlines`",
					"not_for":  "weak, mixed, or downside-leaning evidence",
					"examples": []string{"supertrend bull plus a positive catalyst cluster", "above_vwap momentum confirmed by bullish headlines"},
				},
				"SHORT": map[string]interface{}{
					"covers":   "downward directional opportunity the visible evidence supports",
					"basis":    "`semantic.supertrend` bear or `semantic.trend` below_vwap, agreeing with negative `catalysts` and `headlines`",
					"not_for":  "weak, mixed, or upside-leaning evidence",
					"examples": []string{"supertrend bear plus a negative catalyst cluster", "below_vwap weakness confirmed by bearish headlines"},
				},
			},
		},
		"edge": {
			Type: "noul",
			Instructions: map[string]interface{}{
				"question": "Do `headlines`/`catalysts` together with `semantic` and `indicators` show a confirmed setup worth a paper trade right now?",
			},
			Criteria: map[string]interface{}{
				"true":  "a credible catalyst story exists and the technical picture agrees; `gates_as_fields` show checks cleared",
				"false": "no credible catalyst, contradictory evidence, or gates not cleared",
			},
		},
		"timeframe": TimeframeQuestion(bucket),
	}
}
