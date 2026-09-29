package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/market"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

// ListFuturesSignalsHandler handles GET /api/v1/signals/futures
func (s *Server) newSignalService(optCtx ...context.Context) *trader.SignalService {
	ctx := context.Background()
	if len(optCtx) > 0 && optCtx[0] != nil {
		ctx = optCtx[0]
	}
	var sigCfg trader.SignalConfig
	if s.cfg != nil {
		sigCfg = trader.SignalConfig{
			MinRiskRewardRatio: s.cfg.MinRiskRewardRatio,
			DefaultLeverage:    s.cfg.DefaultLeverage,
			MinStopLossPct:     s.cfg.MinStopLossPct,
			MaxStopLossPct:     s.cfg.MaxStopLossPct,
			MinTakeProfitPct:   s.cfg.MinTakeProfitPct,
			MaxTakeProfitPct:   s.cfg.MaxTakeProfitPct,
			MaxRiskPerTradePct: s.cfg.MaxRiskPerTradePct,
			MaxTradeMarginPct:  s.cfg.MaxTradeMarginPct,
		}
	} else {
		// spec-017 FR-409: no hardcoded fallback config — a nil server config
		// is a wiring bug (boot is fatal without config.Load success).
		log.Fatalf("[FATAL] newSignalService: server config not loaded (spec-017 wiring error)")
	}
	// A typed-nil *db.Store must not enter an interface field: SignalService
	// checks `store != nil`, which stays true for a nil pointer in a non-nil
	// interface, and the first store call would then panic and take the whole
	// server down in no-database mode.
	var store trader.SignalStoreInterface
	if s.dbStore != nil {
		store = s.dbStore
	}
	svc := trader.NewSignalService(store, s.aiClient, sigCfg)
	if s.cfg != nil {
		svc.SetAppConfig(s.cfg)
	}
	if s.decisionRouter != nil {
		svc.SetDecisionRouter(s.decisionRouter)
	}
	if s.shadow != nil {
		svc.SetShadow(s.shadow)
	}
	svc.SetNewsClassifier(s.newsClassifier)
	// Serialise the final cap check so decide-all and the background scanner
	// cannot both pass an open-slot read and overshoot MAX_CONCURRENT_SIGNALS.
	svc.SetSlotGuard(func(symbol string) (*db.FuturesTradeSignal, error) {
		s.signalSlotMu.Lock()
		defer s.signalSlotMu.Unlock()
		if s.dbStore == nil {
			return nil, nil
		}

		// Duplicate guard first: the background scanner and decide-all both
		// evaluate the same symbol concurrently and both read "none active"
		// before either inserted, producing two OPEN signals for one symbol.
		if existing, err := s.dbStore.GetActiveFuturesSignalBySymbol(ctx, symbol); err == nil && existing != nil {
			return existing, nil
		}

		maxActive := 5
		if s.cfg != nil && s.cfg.MaxConcurrentSignals > 0 {
			maxActive = s.cfg.MaxConcurrentSignals
		}
		active, err := s.dbStore.ListFuturesSignals(ctx, "ACTIVE", 100)
		if err != nil {
			return nil, nil
		}
		if len(active) >= maxActive {
			return nil, fmt.Errorf("%w (%d/%d)", trader.ErrConcurrentCap, len(active), maxActive)
		}
		return nil, nil
	})
	return svc
}

func (s *Server) ListFuturesSignalsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	status := r.URL.Query().Get("status")
	if status == "" {
		status = "ACTIVE"
	}

	limit := 20
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	profile := r.URL.Query().Get("profile")
	cacheKey := fmt.Sprintf("cache:signals:futures:%s:%s:%d", profile, status, limit)
	if s.redisClient != nil {
		if cached, err := s.redisClient.Get(r.Context(), cacheKey); err == nil && len(cached) > 0 {
			w.Write(cached)
			return
		}
	}

	if s.dbStore == nil {
		json.NewEncoder(w).Encode([]db.FuturesTradeSignal{})
		return
	}

	// US4 T043: ?profile=COMMODITY scopes the Commodities view.
	var signals []db.FuturesTradeSignal
	var err error
	if profile != "" {
		signals, err = s.dbStore.ListFuturesSignalsByProfile(r.Context(), status, limit, profile)
	} else {
		signals, err = s.dbStore.ListFuturesSignals(r.Context(), status, limit)
	}
	if err != nil {
		http.Error(w, `{"error":"failed to fetch futures signals: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if signals == nil {
		signals = []db.FuturesTradeSignal{}
	}

	data, err := json.Marshal(signals)
	if err != nil {
		http.Error(w, `{"error":"failed to marshal futures signals"}`, http.StatusInternalServerError)
		return
	}
	if s.redisClient != nil {
		_ = s.redisClient.Set(r.Context(), cacheKey, data, 10*time.Second)
	}
	w.Write(data)
}

// GenerateFuturesSignalRequest defines the payload for POST /api/v1/signals/futures/decide
type GenerateFuturesSignalRequest struct {
	Symbol        string   `json:"symbol"`
	Bucket        string   `json:"bucket"`
	NewsHeadlines []string `json:"news_headlines,omitempty"`
}

// freeSignalSlots returns how many more active signals may be opened under
// MAX_CONCURRENT_SIGNALS. A negative result means the cap is already reached.
func (s *Server) freeSignalSlots(ctx context.Context) int {
	maxActive := 5
	if s.cfg != nil && s.cfg.MaxConcurrentSignals > 0 {
		maxActive = s.cfg.MaxConcurrentSignals
	}
	if s.dbStore == nil {
		return maxActive
	}
	active, err := s.dbStore.ListFuturesSignals(ctx, "ACTIVE", 100)
	if err != nil {
		return maxActive
	}
	return maxActive - len(active)
}

// ensureSignalPosition opens an execution-engine position for an active signal
// unless one already exists. Called on every path that can return an active
// signal (creation and the "already active" early return) so a signal can never
// exist without its position.
func (s *Server) ensureSignalPosition(sig *db.FuturesTradeSignal) {
	if s.execEngine == nil || sig == nil || sig.ID <= 0 || sig.Status != "ACTIVE" {
		return
	}
	trade, err := s.execEngine.OpenPositionFromSignal(sig)
	if err != nil {
		log.Printf("[Signal->Position] Failed to open position for %s: %v", sig.Symbol, err)
		return
	}
	if trade == nil {
		return
	}
	if s.broadcaster != nil {
		if tradeBytes, err := json.Marshal(trade); err == nil {
			s.broadcaster.Broadcast("trade", string(tradeBytes))
		}
	}
	log.Printf("[Signal->Position] Opened %s position for %s at $%.2f (margin=$%.2f, lev=%dx)",
		sig.Direction, sig.Symbol, sig.EntryPrice, sig.AllocatedCapitalUSD, sig.Leverage)
}

// blackoutEventName maps a macro-calendar title onto a configured blackout
// window name (NFP/CPI/FOMC/EIA) or "" when the event has no blackout.
func blackoutEventName(title string) string {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "non-farm") || strings.Contains(lower, "nonfarm") ||
		strings.Contains(lower, "nfp") || strings.Contains(lower, "payroll"):
		return "NFP"
	case strings.Contains(lower, "cpi") || strings.Contains(lower, "consumer price"):
		return "CPI"
	case strings.Contains(lower, "fomc") || strings.Contains(lower, "federal funds") ||
		strings.Contains(lower, "fed chair") || strings.Contains(lower, "fomc member"):
		return "FOMC"
	case strings.Contains(lower, "eia") || strings.Contains(lower, "crude oil") ||
		strings.Contains(lower, "gasoline"):
		return "EIA"
	default:
		return ""
	}
}

// metaSlice adapts an optional catalyst meta to the variadic EvaluateMarketSignal
// argument (nil -> no argument).
func metaSlice(m *trader.CatalystMeta) []trader.CatalystMeta {
	if m == nil {
		return nil
	}
	return []trader.CatalystMeta{*m}
}

// EvaluateSymbolSignal evaluates a single symbol against breaking news catalysts and technical confluence.
// If high conviction is detected, it persists the signal, broadcasts via SSE and Telegram, and returns the signal.
func (s *Server) EvaluateSymbolSignal(ctx context.Context, symbol string, headlines []string) (*db.FuturesTradeSignal, string, error) {
	if s.aiClient == nil {
		return nil, "AI client not configured", errors.New("ai client not configured")
	}

	if symbol == "" || symbol == "BTC/USD" {
		symbol = "BTC/USDT"
	}
	bucket := market.GetBucket(symbol)

	// Ingest latest breaking news headlines from crawler if not provided
	if len(headlines) == 0 && s.newsCrawler != nil {
		latest := s.newsCrawler.GetLatestArticles()
		for i := 0; i < len(latest) && i < 15; i++ {
			headlines = append(headlines, latest[i].Title)
		}
	}

	// Keep only headlines that can genuinely act as a catalyst for THIS asset.
	// Symbols with no relevant news hold deterministically and skip an AI
	// round-trip entirely, which is what lets a full-catalog scan finish
	// inside the batch deadline instead of timing out on 123 AI calls.
	headlines = market.HeadlinesForSymbol(headlines, symbol)
	// T039 (FR-013) dry-run stage log: records exactly where the commodity
	// evaluation branch dies (catalyst scarcity vs later stages).
	log.Printf("[DRY] %s bucket=%s stages: headlines_matched=%d", symbol, bucket, len(headlines))
	if len(headlines) == 0 {
		return nil, "No asset-relevant news headline for " + symbol, nil
	}

	prof, profErr := trader.EffectiveProfile(trader.ProfileNameForBucket(bucket))
	if profErr != nil {
		prof = config.GetRiskProfile(trader.ProfileNameForBucket(bucket))
	}

	// --- Spec 012 US4: commodity session entry guards (FR-014) ---
	// Weekend gap and event blackouts block NEW entries only for commodity
	// (weekendFlat) profiles; each veto lands in entry_filter_log (quickstart §5).
	if prof.WeekendFlat {
		now := time.Now()
		if trader.WeekendGapOpen(now) {
			detail, _ := json.Marshal(map[string]interface{}{
				"window": "Fri 16:45 ET - Sun 18:00 ET",
				"at":     now.UTC().Format(time.RFC3339),
			})
			if s.dbStore != nil {
				_ = s.dbStore.InsertEntryFilterLog(ctx, symbol, "", nil, "WEEKEND_GAP", detail)
			}
			log.Printf("[US4] weekend gap blocks new entries for %s", symbol)
			return nil, "Commodity weekend gap: market closed (Fri 16:45 - Sun 18:00 ET)", nil
		}
		if s.calendar != nil {
			var events []trader.ScheduledEvent
			for _, ev := range s.calendar.GetEvents() {
				// Only HIGH-impact events open blackout windows — LOW/MEDIUM
				// "FOMC Member Speaks" speeches are noise, not decision events.
				if ev.Impact != market.ImpactHigh {
					continue
				}
				if name := blackoutEventName(ev.Title); name != "" {
					events = append(events, trader.ScheduledEvent{Name: name, Time: ev.ScheduledAt})
				}
			}
			if blocked, name := trader.InBlackoutWindow(prof, events, now); blocked {
				detail, _ := json.Marshal(map[string]interface{}{
					"event":     name,
					"at":        now.UTC().Format(time.RFC3339),
					"blackouts": prof.BlackoutWindows,
				})
				if s.dbStore != nil {
					_ = s.dbStore.InsertEntryFilterLog(ctx, symbol, "", nil, "EVENT_BLACKOUT", detail)
				}
				log.Printf("[US4] %s blackout blocks new entries for %s", name, symbol)
				return nil, fmt.Sprintf("EVENT_BLACKOUT: %s window active", name), nil
			}
		}
	}

	// --- Spec 012 US3: catalyst clustering + polarization veto (FR-008/011) ---
	// Near-identical syndicated headlines merge into ONE Catalyst Event; a
	// polarized (contradictorily balanced) story is vetoed BEFORE the AI
	// round-trip, with the veto recorded in entry_filter_log (T033/T034).
	var catalystMeta *trader.CatalystMeta
	if s.catalystClusterer != nil {
		// Source lookup: title -> feed source from the crawler's in-memory
		// retention (100 newest); unknown titles fall back to tier 0.2.
		srcOf := make(map[string]string)
		if s.newsCrawler != nil {
			for _, a := range s.newsCrawler.GetLatestArticles() {
				srcOf[a.Title] = a.Source
			}
		}
		classify := s.newsClassifier
		if classify == nil {
			classify = market.DefaultClassifier
		}
		for _, h := range headlines {
			rep, err := classify([]string{h})
			if err != nil {
				return nil, "", fmt.Errorf("component=news-classifier cycle=%s: %w", h, err)
			}
			s.catalystClusterer.Ingest(srcOf[h], h, rep.Score, time.Now())
		}

		// Clusters covering at least one supplied headline.
		var covering []*market.NewsCluster
		for _, cl := range s.catalystClusterer.Clusters(time.Now()) {
			for _, h := range headlines {
				if market.TrigramJaccard(h, cl.Headline) >= market.ClusterJaccardThreshold {
					covering = append(covering, cl)
					break
				}
			}
		}

		// Dominant = most-covered story (the one the AI will cite).
		var dominant *market.NewsCluster
		for _, cl := range covering {
			if dominant == nil || cl.StoryCount >= dominant.StoryCount {
				dominant = cl
			}
		}

		// News decay resolution: FAST_BREAKING vs MACRO_THEMATIC drives freshness half-life (spec-015 FR-302)
		decayHalfLife := prof.FreshnessHalfLifeMin
		reg := trader.NewParamRegistry(s.cfg)
		if decaySpec, ok := reg.Get("decay"); ok {
			if decaySpec.Mode() == trader.ModeOverride {
				if s.cfg != nil && s.cfg.OverrideClusterDecayMode == "MACRO_THEMATIC" {
					decayHalfLife = 120.0
				} else if s.cfg != nil && s.cfg.OverrideClusterDecayMode == "FAST_BREAKING" {
					decayHalfLife = 15.0
				}
			} else if dominant != nil && s.decisionRouter != nil && s.decisionRouter.Jev != nil {
				q := decaySpec.Question(map[string]interface{}{
					"headline":    dominant.Headline,
					"story_count": dominant.StoryCount,
				})
				cycleID := fmt.Sprintf("decay-%s-%d", symbol, time.Now().UnixNano())
				decayAnswers, _, err := s.decisionRouter.Jev.Evaluate(ctx, cycleID, map[string]interface{}{
					"headline":    dominant.Headline,
					"story_count": dominant.StoryCount,
					"symbol":      symbol,
				}, map[string]ai.JevQuestion{"decay": q})
				if err == nil {
					if dAns, ok := decayAnswers["decay"]; ok {
						res, rerr := reg.Resolve("decay", &dAns, cycleID)
						if rerr == nil {
							if res.Value == "MACRO_THEMATIC" {
								decayHalfLife = 120.0
							} else {
								decayHalfLife = 15.0
							}
						}
					}
				}
			}
		}

		// Persist each covering event; keep the dominant event id (T034).
		var dominantID int64
		if s.dbStore != nil {
			for _, cl := range covering {
				fresh := market.FreshnessWeight(time.Since(cl.LastSeen), decayHalfLife)
				id, err := s.dbStore.InsertCatalystEvent(
					ctx, cl.Fingerprint, cl.Headline, cl.Sources, []string{symbol},
					cl.StoryCount, cl.FusedSentiment, cl.Polarization, fresh,
					int(decayHalfLife),
				)
				if err != nil {
					log.Printf("[US3] insert catalyst event for %s: %v", symbol, err)
					continue
				}
				if cl == dominant {
					dominantID = id
				}
			}
		}

		if dominant != nil {
			// FR-011: contradictory balanced coverage -> no trade, audited.
			if market.PolarizationVetoed(dominant.Polarization) {
				detail, _ := json.Marshal(map[string]interface{}{
					"polarization": dominant.Polarization,
					"threshold":    0.40,
					"story_count":  dominant.StoryCount,
					"sources":      dominant.Sources,
					"headline":     dominant.Headline,
				})
				if s.dbStore != nil {
					var evtID *int64
					if dominantID > 0 {
						evtID = &dominantID
					}
					_ = s.dbStore.InsertEntryFilterLog(ctx, symbol, "", evtID, "POLARIZED", detail)
				}
				log.Printf("[US3] POLARIZED veto for %s: P=%.2f on '%s'", symbol, dominant.Polarization, dominant.Headline)
				return nil, fmt.Sprintf("POLARIZED: contradictory coverage (P=%.2f > 0.40) on '%s'", dominant.Polarization, dominant.Headline), nil
			}

			// Collapse the dominant cluster's members to its representative
			// headline; distinct stories pass through untouched (T036).
			kept := make([]string, 0, len(headlines))
			for _, h := range headlines {
				if market.TrigramJaccard(h, dominant.Headline) >= market.ClusterJaccardThreshold {
					continue
				}
				kept = append(kept, h)
			}
			kept = append(kept, dominant.Headline)
			headlines = kept

			catalystMeta = &trader.CatalystMeta{
				EventID:        dominantID,
				Headline:       dominant.Headline,
				StoryCount:     dominant.StoryCount,
				FusedSentiment: dominant.FusedSentiment,
				Freshness:      market.FreshnessWeight(time.Since(dominant.LastSeen), prof.FreshnessHalfLifeMin),
				Sources:        dominant.Sources,
			}
		}
	}

	// Macroeconomic Calendar Halt Guard (FR-005)
	// If a high-impact release is pending within the halt window, halt opening new trades to protect capital
	if s.calendar != nil {
		if halted, reason := s.calendar.IsSymbolHalted(symbol, time.Now()); halted {
			log.Printf("[MACRO HALT] Signal generation halted for %s due to high-impact macro event: %s", symbol, reason)
			return nil, "Macro event halt: " + reason, nil // Preserves capital in HOLD
		}
	}

	// Fetch current market price
	currentPrice := 0.0
	if s.redisClient != nil {
		if quote, err := s.redisClient.GetTicker(ctx, symbol); err == nil && quote != nil && quote.Price > 0 {
			currentPrice = quote.Price
		}
	}
	if currentPrice <= 0 && s.marketData != nil {
		if p, err := s.marketData.GetLatestPrice(symbol); err == nil && p > 0 {
			currentPrice = p
		}
	}
	if currentPrice <= 0 {
		fetcher := market.NewBinanceFetcher()
		cleanSym := strings.ReplaceAll(symbol, "/", "")
		if q, err := fetcher.FetchTicker(ctx, cleanSym); err == nil && q != nil && q.Price > 0 {
			currentPrice = q.Price
			if s.marketData != nil {
				s.marketData.UpdateQuote(*q)
			}
		}
	}
	if currentPrice <= 0 {
		return nil, "", fmt.Errorf("live market price unavailable for %s", symbol)
	}

	quote := cache.TickerQuote{
		Symbol: symbol,
		Price:  currentPrice,
	}

	var snap cache.IndicatorSnapshot
	if computedSnap, err := s.GetIndicatorSnapshot(ctx, symbol); err == nil && computedSnap != nil {
		snap = *computedSnap
	} else {
		snap = cache.IndicatorSnapshot{Symbol: symbol}
	}

	totalEquity := s.cfg.InitialCapital
	availableAlphaCapital := totalEquity * s.cfg.AlphaTargetPct
	if totalEquity <= 0 {
		totalEquity = 10000.0
		availableAlphaCapital = totalEquity * 0.40
	}
	if s.execEngine != nil {
		totalEquity = s.execEngine.GetTotalEquity()
	}
	if s.allocator != nil {
		allocBreakdown := s.allocator.Get3TierBreakdown()
		if te, ok := allocBreakdown["total_equity"].(float64); ok && te > 0 {
			totalEquity = te
		}
		// Size a CORE commodity from the Core tier and an ALPHA coin from the
		// Tactical Alpha tier. Every candidate previously drew on Tier 3, so
		// commodity signals were silently funded by the crypto budget.
		tierKey := "tier3_tactical"
		if bucket == "CORE" {
			tierKey = "tier2_core"
		}
		if tierAmt, ok := allocBreakdown[tierKey].(float64); ok && tierAmt > 0 {
			availableAlphaCapital = tierAmt
		}
	}

	// 1. Reserved capital, scoped to this candidate's tier. CORE commodities draw
	// on the Core budget and ALPHA crypto on the Tactical Alpha budget: counting a
	// gold position against the alpha budget (as before) starved every crypto
	// signal of capital and made the whole scan report "fully reserved".
	reservedCapital := 0.0
	activeSignalsCount := 0
	// Signals and positions are two views of the SAME exposure: every signal
	// opens an engine position. Counting both reserves each trade twice and
	// reported the whole tier as exhausted. Count positions as the truth, and
	// only count signals that have not yet become positions (pre-rehydration).
	openPosition := make(map[string]bool)
	if s.execEngine != nil {
		for _, tr := range s.execEngine.GetOpenTrades() {
			if tr == nil {
				continue
			}
			openPosition[tr.Symbol] = true
			if tr.Symbol == symbol || tr.Bucket != bucket {
				continue
			}
			lev := tr.Leverage
			if lev < 1 {
				lev = 1
			}
			margin := (tr.PositionSize * tr.EntryPrice) / float64(lev)
			reservedCapital += margin
		}
	}
	if s.dbStore != nil {
		activeSignals, err := s.dbStore.ListFuturesSignals(ctx, "ACTIVE", 50)
		if err == nil && activeSignals != nil {
			activeSignalsCount = len(activeSignals)
			for _, as := range activeSignals {
				if as.Symbol != symbol && market.GetBucket(as.Symbol) == bucket && !openPosition[as.Symbol] {
					reservedCapital += as.AllocatedCapitalUSD
				}
			}
		}
	}

	// 2. Concurrency & Correlated Commodity Exposure Gating
	maxActive := 5
	if s.cfg != nil && s.cfg.MaxConcurrentSignals > 0 {
		maxActive = s.cfg.MaxConcurrentSignals
	}
	if s.dbStore != nil {
		existing, err := s.dbStore.GetActiveFuturesSignalBySymbol(ctx, symbol)
		if err == nil && existing != nil {
			// Signal already active for this symbol; return existing without duplicate
			// notifications. Still guarantee its position exists.
			s.ensureSignalPosition(existing)
			return existing, "", nil
		}

		// Correlated Commodity Guard: prevent concurrent active signals across correlated assets in same exposure group
		activeSignals, err := s.dbStore.ListFuturesSignals(ctx, "ACTIVE", 50)
		if err == nil && activeSignals != nil {
			for _, as := range activeSignals {
				if market.AreCorrelatedCommodities(as.Symbol, symbol) {
					// Correlated commodity (e.g. PAXG & XAU) already active; prevent duplicate risk and cash splitting
					return nil, "Correlated commodity already active: " + as.Symbol, nil // HOLD
				}
			}
		}

		if activeSignalsCount >= maxActive {
			// Capital guard: dynamically bounded active signals permitted
			return nil, fmt.Sprintf("Max concurrent signals reached (%d/%d)", activeSignalsCount, maxActive), nil
		}
	}

	if s.execEngine != nil {
		for _, tr := range s.execEngine.GetOpenTrades() {
			if tr != nil && market.AreCorrelatedCommodities(tr.Symbol, symbol) {
				// Correlated commodity already open in execution engine
				return nil, "Correlated position already open: " + tr.Symbol, nil // HOLD
			}
		}
	}

	// 3. Dynamic capital reservation
	unreservedAlphaCapital := availableAlphaCapital - reservedCapital
	minRequiredUnreserved := totalEquity * 0.05
	if unreservedAlphaCapital < minRequiredUnreserved {
		return nil, "Capital fully reserved by active trades", nil // HOLD
	}

	// Split the unreserved tier budget across the remaining slots. Each signal
	// may size itself up to MAX_TRADE_MARGIN_PCT of total equity, but the tier is
	// only ~40% of equity: two full-size ALPHA trades exhausted the whole tier and
	// every later signal then reported "fully reserved" instead of trading.
	if remainingSlots := maxActive - activeSignalsCount; remainingSlots > 1 {
		if perSlot := unreservedAlphaCapital / float64(remainingSlots); perSlot > 0 {
			unreservedAlphaCapital = perSlot
		}
	}

	// Pre-AI gate outcomes (spec-018 FR-501): every guard listed here either
	// ran and passed above, or is honestly marked not_applicable. Failures
	// already returned HOLD before this point — these are REAL outcomes.
	preGates := map[string]string{
		"polarization":         "clear",
		"calendar_halt":        "clear",
		"correlated_positions": "clear",
		"capital_available":    "clear",
	}
	if prof.WeekendFlat {
		preGates["weekend_gap"] = "clear"
		preGates["event_blackout"] = "clear"
	} else {
		preGates["weekend_gap"] = "not_applicable"
		preGates["event_blackout"] = "not_applicable"
	}
	if s.calendar == nil {
		preGates["calendar_halt"] = "not_applicable"
	}
	if s.catalystClusterer == nil {
		preGates["polarization"] = "not_applicable"
	}

	signalSvc := s.newSignalService(ctx)
	sig, decision, err := signalSvc.EvaluateMarketSignal(
		ctx,
		symbol,
		bucket,
		quote,
		snap,
		nil,
		headlines,
		totalEquity,
		unreservedAlphaCapital,
		preGates,
		metaSlice(catalystMeta)...,
	)
	if err != nil {
		if errors.Is(err, trader.ErrConcurrentCap) {
			return nil, err.Error(), nil // slot filled mid-scan: HOLD, not ERROR
		}
		return nil, "", fmt.Errorf("signal generation failed: %w", err)
	}

	if sig == nil {
		// Surface the AI's own explanation rather than a generic "no catalyst":
		// an operator must be able to see WHY a scan produced no trade.
		reason := "No high-conviction catalyst detected"
		if decision != nil {
			if decision.GateRejected != "" {
				// Entry gate veto (spec 012 US1): broadcast for the audit UI.
				reason = "Entry gate [" + decision.GateRejected + "]"
				if decision.Reasoning != "" {
					reason += ": " + decision.Reasoning
				}
				if s.broadcaster != nil {
					payload := map[string]interface{}{
						"symbol":    symbol,
						"direction": decision.Decision,
						"rule":      decision.GateRejected,
						"detail":    decision.GateRejectedDetail,
						"ts":        time.Now().UTC().Format(time.RFC3339),
					}
					if b, err := json.Marshal(payload); err == nil {
						s.broadcaster.Broadcast("filter_rejected", string(b))
					}
				}
				log.Printf("[ENTRY GATE] rejected %s %s: %s", symbol, decision.Decision, decision.GateRejected)
			} else if decision.Reasoning != "" {
				reason = decision.Reasoning
			} else if decision.Catalyst != "" {
				reason = "Catalyst rejected: " + decision.Catalyst
			}
		}
		return nil, reason, nil // HOLD
	}

	// Every active signal must hold a matching execution-engine position.
	s.ensureSignalPosition(sig)

	// Broadcast signal via SSE and Telegram ONLY if brand-new and not yet dispatched
	if !sig.TelegramDispatched {
		if s.broadcaster != nil {
			if sigBytes, err := json.Marshal(sig); err == nil {
				s.broadcaster.Broadcast("futures_signal", string(sigBytes))
			}
		}

		if s.telegramBot != nil && s.telegramBot.GetConfig().Enabled {
			s.telegramBot.BroadcastSignalEntry(sig)
			if s.dbStore != nil && sig.ID > 0 {
				_ = s.dbStore.MarkSignalDispatched(ctx, sig.ID)
				sig.TelegramDispatched = true
			}
		}
	}

	return sig, "", nil
}

// GenerateFuturesSignalHandler handles POST /api/v1/signals/futures/decide
func (s *Server) GenerateFuturesSignalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.aiClient == nil {
		http.Error(w, `{"error":"ai client not configured"}`, http.StatusServiceUnavailable)
		return
	}

	var req GenerateFuturesSignalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Symbol == "" || req.Symbol == "BTC/USD" {
		req.Symbol = "BTC/USDT"
	}
	if req.Bucket == "" {
		req.Bucket = "ALPHA"
	}

	sig, holdReason, err := s.EvaluateSymbolSignal(r.Context(), req.Symbol, req.NewsHeadlines)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if sig == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "HOLD",
			"message": holdReason,
			"symbol":  req.Symbol,
		})
		return
	}

	json.NewEncoder(w).Encode(sig)
}

// GenerateAllFuturesSignalsRequest defines the payload for POST /api/v1/signals/futures/decide-all
type GenerateAllFuturesSignalsRequest struct {
	Symbols []string `json:"symbols,omitempty"`
	Bucket  string   `json:"bucket,omitempty"`
}

// AssetScanResult provides the scan resolution for each asset in a batch scan.
type AssetScanResult struct {
	Symbol  string                 `json:"symbol"`
	Status  string                 `json:"status"` // "SIGNAL", "HOLD", "ERROR"
	Signal  *db.FuturesTradeSignal `json:"signal,omitempty"`
	Message string                 `json:"message,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// GenerateAllFuturesSignalsHandler handles POST /api/v1/signals/futures/decide-all
func (s *Server) GenerateAllFuturesSignalsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.aiClient == nil {
		http.Error(w, `{"error":"ai client not configured"}`, http.StatusServiceUnavailable)
		return
	}

	var req GenerateAllFuturesSignalsRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	symbols := req.Symbols
	bucket := strings.ToUpper(strings.TrimSpace(req.Bucket))
	if bucket == "" {
		bucket = "ALL"
	}

	if len(symbols) == 0 {
		// Scan the FULL catalog: the operator pressed "Scan All Assets" and
		// expects every instrument evaluated, not just the liquidity-qualified
		// subset. The batch is bounded by its own deadline (below) rather than
		// by shrinking the universe, so a slow gateway can never 502 the request.
		universe := make([]string, 0, len(market.GetSupportedAssets()))
		for _, a := range market.GetSupportedAssets() {
			universe = append(universe, a.Symbol)
		}
		switch bucket {
		case "CORE":
			for _, sym := range universe {
				if market.GetBucket(sym) == "CORE" {
					symbols = append(symbols, sym)
				}
			}
		case "ALPHA":
			for _, sym := range universe {
				if market.GetBucket(sym) == "ALPHA" {
					symbols = append(symbols, sym)
				}
			}
		default: // "ALL" or unrecognized
			symbols = universe
		}
	}

	// Ingest latest breaking news headlines once for the batch
	var newsHeadlines []string
	if s.newsCrawler != nil {
		latest := s.newsCrawler.GetLatestArticles()
		for i := 0; i < len(latest) && i < 15; i++ {
			newsHeadlines = append(newsHeadlines, latest[i].Title)
		}
	}

	results := make([]AssetScanResult, len(symbols))
	for i, sym := range symbols {
		results[i] = AssetScanResult{
			Symbol:  sym,
			Status:  "SKIPPED",
			Message: "Not evaluated: batch deadline reached before this asset started.",
		}
	}

	// Overall deadline for the whole batch. The response must always return:
	// a sync handler blocked past the gateway timeout 502s and the dashboard
	// shows a broken scan even though the backend kept working.
	batchCtx, batchCancel := context.WithTimeout(r.Context(), 85*time.Second)
	defer batchCancel()

	var wg sync.WaitGroup
	// Modest concurrency: 24 parallel calls starved the gateway (100 of 123
	// hit the deadline and fell back to the lexicon heuristic, which always
	// reports neutral HOLD). Fewer in-flight calls keep each AI round-trip
	// fast enough to finish the whole catalog inside the batch deadline.
	//
	// The semaphore is ALSO the concurrency-cap guard: every worker reads the
	// active-signal count independently, so an uncapped wave all read "0 active"
	// at once and inserted past MAX_CONCURRENT_SIGNALS (10 signals against a cap
	// of 5). Limiting in-flight work to the free slots makes that impossible:
	// wave one fills the slots, wave two re-reads the updated count and holds.
	semCap := 12
	if freeSlots := s.freeSignalSlots(r.Context()); freeSlots < semCap {
		semCap = freeSlots
	}
	if semCap < 1 {
		semCap = 1
	}
	sem := make(chan struct{}, semCap)

	for i, sym := range symbols {
		wg.Add(1)
		go func(idx int, symbol string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Do not start new evaluations after the batch deadline: their
			// results would land as SKIPPED anyway and we would hold the
			// response for nothing.
			if batchCtx.Err() != nil {
				return
			}

			evalCtx, evalCancel := context.WithTimeout(batchCtx, 20*time.Second)
			defer evalCancel()

			sig, holdReason, err := s.EvaluateSymbolSignal(evalCtx, symbol, newsHeadlines)
			if err != nil {
				results[idx] = AssetScanResult{
					Symbol: symbol,
					Status: "ERROR",
					Error:  err.Error(),
				}
				return
			}

			if sig == nil {
				results[idx] = AssetScanResult{
					Symbol:  symbol,
					Status:  "HOLD",
					Message: holdReason,
				}
			} else {
				results[idx] = AssetScanResult{
					Symbol: symbol,
					Status: "SIGNAL",
					Signal: sig,
				}
			}
		}(i, sym)
	}

	wg.Wait()

	signalsCount := 0
	for _, res := range results {
		if res.Status == "SIGNAL" {
			signalsCount++
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"scanned_count": len(symbols),
		"signals_count": signalsCount,
		"results":       results,
		"timestamp":     time.Now(),
	})
}

// CloseFuturesSignalHandler handles POST /api/v1/signals/futures/{id}/close
func (s *Server) CloseFuturesSignalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid signal id"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		ExitPrice  float64 `json:"exit_price"`
		ExitReason string  `json:"exit_reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.ExitReason = "MANUAL_EXIT"
	}
	if req.ExitReason == "" {
		req.ExitReason = "MANUAL_EXIT"
	}

	if s.dbStore == nil {
		http.Error(w, `{"error":"db store unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	// Fetch signals list to find target signal
	signals, err := s.dbStore.ListFuturesSignals(r.Context(), "ACTIVE", 100)
	if err != nil {
		http.Error(w, `{"error":"failed to locate signal: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	var targetSig *db.FuturesTradeSignal
	for i := range signals {
		if signals[i].ID == id {
			targetSig = &signals[i]
			break
		}
	}

	if targetSig == nil {
		http.Error(w, `{"error":"active futures signal not found"}`, http.StatusNotFound)
		return
	}

	if req.ExitPrice <= 0 {
		if s.marketData != nil {
			livePrice, err := s.marketData.GetLatestPrice(targetSig.Symbol)
			if err == nil && livePrice > 0 {
				req.ExitPrice = livePrice
			}
		}
	}

	if req.ExitPrice <= 0 {
		http.Error(w, `{"error":"exit_price must be positive or live market price must be available"}`, http.StatusBadRequest)
		return
	}

	signalSvc := s.newSignalService(r.Context())
	// Close unconditionally: CheckSignalResolution only settles when price has
	// crossed SL/TP, and discarding its resolved flag returned {"status":"CLOSED"}
	// while the database row stayed ACTIVE — the engine position disappeared but
	// the signal never did.
	pnl, roi, err := signalSvc.CloseSignalNow(r.Context(), targetSig, req.ExitPrice, req.ExitReason)
	if err != nil {
		http.Error(w, `{"error":"failed to close signal: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	// Force-close the matching engine position: a manual close means "exit now",
	// so it must not depend on a level being crossed.
	if s.execEngine != nil {
		if closedTrade, exited := s.execEngine.ForceClosePosition(targetSig.Symbol, req.ExitPrice, req.ExitReason); exited {
			log.Printf("[Signal→Position] Manual close %s position for %s: pnl=%.2f",
				closedTrade.Side, closedTrade.Symbol, closedTrade.RealizedPnL)
		}
	}

	// Broadcast resolution via SSE
	if s.broadcaster != nil {
		closePayload, _ := json.Marshal(map[string]interface{}{
			"id":          id,
			"symbol":      targetSig.Symbol,
			"exit_price":  req.ExitPrice,
			"exit_reason": req.ExitReason,
			"pnl_usd":     pnl,
			"roi_pct":     roi,
		})
		s.broadcaster.Broadcast("futures_signal_closed", string(closePayload))
	}

	// Broadcast resolution via Telegram Bot if active
	if s.telegramBot != nil && s.telegramBot.GetConfig().Enabled {
		s.telegramBot.BroadcastSignalResolution(targetSig, req.ExitPrice, req.ExitReason, pnl, roi)
		if s.dbStore != nil && targetSig.ID > 0 {
			_ = s.dbStore.MarkSignalResolved(r.Context(), targetSig.ID)
		}
	}

	// Online Thompson Sampling fine-tuning for CPU-only VPS runtime
	if s.sampler != nil {
		s.sampler.RecordOutcome(tradeOutcomeFromSignal(targetSig, pnl, roi))
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "CLOSED",
		"id":          id,
		"symbol":      targetSig.Symbol,
		"exit_price":  req.ExitPrice,
		"exit_reason": req.ExitReason,
		"pnl_usd":     pnl,
		"roi_pct":     roi,
	})
}

// ReconcileActiveSignals closes every ACTIVE signal whose level was crossed by
// the current live price or whose max age (SIGNAL_MAX_AGE_MINUTES) has lapsed.
// Tick-triggered exits alone miss crossings during restarts or quiet periods:
// BNB/USDT fell through its TP on 2026-09-23 14:00 UTC and stayed ACTIVE for a
// full day because no tick fired while the backend was down. This sweep runs at
// startup and periodically so exits can never be missed for long.
func (s *Server) ReconcileActiveSignals(ctx context.Context) {
	if s.dbStore == nil {
		return
	}

	signals, err := s.dbStore.ListFuturesSignals(ctx, "ACTIVE", 50)
	if err != nil {
		log.Printf("[Reconcile] Failed to list active signals: %v", err)
		return
	}
	if len(signals) == 0 {
		return
	}

	maxAge := time.Duration(60) * time.Minute
	if s.cfg != nil && s.cfg.SignalMaxAgeMinutes > 0 {
		maxAge = time.Duration(s.cfg.SignalMaxAgeMinutes) * time.Minute
	}

	now := time.Now()
	reconciled := 0
	for i := range signals {
		sig := &signals[i]
		if sig.Status != "ACTIVE" {
			continue
		}

		// Age-based time exit: an intraday signal has a fixed lifetime
		if now.Sub(sig.CreatedAt) >= maxAge {
			price := s.livePriceFor(ctx, sig.Symbol)
			exitReason := "TIME_EXIT"
			if price > 0 {
				// Settle PnL/ROI at the live price like every other exit path.
				// This used to hardcode 0, 0 which erased all timed-exit results.
				signalSvc := s.newSignalService(ctx)
				if _, _, err := signalSvc.CloseSignalNow(ctx, sig, price, exitReason); err != nil {
					log.Printf("[Reconcile] Failed to settle time-exit for signal #%d: %v", sig.ID, err)
					continue
				}
			} else {
				_ = s.dbStore.CloseFuturesSignal(ctx, sig.ID, price, exitReason, 0, 0)
			}
			if s.execEngine != nil {
				if closedTrade, exited := s.execEngine.ForceClosePosition(sig.Symbol, price, exitReason); exited {
					log.Printf("[Reconcile] Closed %s position for %s on %s: pnl=%.2f",
						closedTrade.Side, closedTrade.Symbol, exitReason, closedTrade.RealizedPnL)
				}
			}
			log.Printf("[Reconcile] Time-exited signal #%d %s %s (age %s > %s)", sig.ID, sig.Symbol, sig.Direction, now.Sub(sig.CreatedAt).Round(time.Minute), maxAge)
			reconciled++
			continue
		}

		// Level reconciliation against the current live price
		price := s.livePriceFor(ctx, sig.Symbol)
		if price <= 0 {
			continue
		}
		signalSvc := s.newSignalService(ctx)
		resolved, exitReason, pnl, roi, err := signalSvc.CheckSignalResolution(ctx, sig, price)
		if err != nil || !resolved {
			// Not closed by TP/SL: run the time-decay state machine (spec 012
			// US2, FR-005). The reconciler is the only driver: the profile's
			// breakeven/flat checkpoints decide protection and closure.
			s.applyDecayState(ctx, sig, price)
			continue
		}
		_ = s.dbStore.CloseFuturesSignal(ctx, sig.ID, price, exitReason, pnl, roi)
		if s.execEngine != nil {
			if closedTrade, exited := s.execEngine.CheckExit(sig.Symbol, price); exited {
				log.Printf("[Reconcile] Closed %s position for %s on %s: pnl=%.2f",
					closedTrade.Side, closedTrade.Symbol, exitReason, closedTrade.RealizedPnL)
			}
		}
		log.Printf("[Reconcile] Level-reconciled signal #%d %s %s: reason=%s pnl=%.2f roi=%.2f", sig.ID, sig.Symbol, sig.Direction, exitReason, pnl, roi)
		reconciled++
	}

	if reconciled > 0 {
		log.Printf("[Reconcile] %d active signal(s) reconciled this pass.", reconciled)
	}
}

// ListCommoditiesStatusHandler serves GET /api/v1/commodities/status
// (spec 012 US4, FR-014/015): horizon, weekend-gap state and active event
// blackouts for the Commodities view banner.
func (s *Server) ListCommoditiesStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	prof, profErr := trader.EffectiveProfile("COMMODITY")
	if profErr != nil {
		prof = config.GetRiskProfile("COMMODITY")
	}
	now := time.Now()

	blocked := make([]string, 0, 2)
	if s.calendar != nil {
		var events []trader.ScheduledEvent
		for _, ev := range s.calendar.GetEvents() {
			if name := blackoutEventName(ev.Title); name != "" {
				events = append(events, trader.ScheduledEvent{Name: name, Time: ev.ScheduledAt})
			}
		}
		if b, name := trader.InBlackoutWindow(prof, events, now); b {
			blocked = append(blocked, name)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"profile":                prof.Name,
		"weekend_flat":           prof.WeekendFlat,
		"in_weekend_gap":         trader.WeekendGapOpen(now),
		"blocked_events":         blocked,
		"horizon_min":            prof.HorizonMin,
		"horizon_max":            prof.HorizonMax,
		"freshness_halflife_min": prof.FreshnessHalfLifeMin,
		"as_of":                  now.UTC().Format(time.RFC3339),
	})
}

// applyDecayState runs the time-decay state machine for one ACTIVE signal
// (spec 012 US2, FR-005): protects (stop to break-even) a laggard at the
// breakeven checkpoint, closes a still-unprofitable trade at the flat
// checkpoint, and time-exits at the hard horizon. Profile comes from the
// signal's own class; failures log and never panic.
func (s *Server) applyDecayState(ctx context.Context, sig *db.FuturesTradeSignal, price float64) {
	if sig == nil || sig.Status != "ACTIVE" || sig.EntryPrice <= 0 || sig.StopLoss <= 0 {
		return
	}

	prof, err := trader.EffectiveProfile(trader.ProfileNameForBucket(sig.Profile))
	if err != nil {
		log.Printf("[Decay] invalid profile for signal #%d: %v", sig.ID, err)
		return
	}

	tf := ""
	if sig.Timeframe != nil {
		tf = *sig.Timeframe
	}

	// Legacy migration (spec-016 FR-206):
	// NULL legacy row → map CRYPTO→1h COMMODITY→4h, write back + log legacy_timeframe_mapped.
	if tf == "" {
		if strings.ToUpper(sig.Profile) == "COMMODITY" {
			tf = "4h"
		} else {
			tf = "1h"
		}
		sig.Timeframe = &tf
		if s.dbStore != nil {
			if err := s.dbStore.UpdateTimeframe(ctx, sig.ID, tf); err != nil {
				log.Printf("[Decay] failed to persist legacy timeframe for signal #%d: %v", sig.ID, err)
			}
		}
		log.Printf("legacy_timeframe_mapped: signal #%d profile %s mapped to %s", sig.ID, sig.Profile, tf)
	}

	if tfProf, ok := config.GetTimeframeProfile(tf); ok {
		prof.HorizonMin = tfProf.HorizonMin
		prof.DecayBreakevenAtMin = tfProf.BEOffsetMin
		prof.DecayFlatAtMin = tfProf.FlatOffsetMin
	}

	ageMin := time.Since(sig.CreatedAt).Minutes()
	riskDist := sig.EntryPrice - sig.StopLoss
	if sig.Direction == "SHORT" {
		riskDist = sig.StopLoss - sig.EntryPrice
	}
	if riskDist <= 0 {
		return
	}

	// R multiple from the live price (unrealized).
	var rMultiple float64
	if sig.Direction == "LONG" {
		rMultiple = (price - sig.EntryPrice) / riskDist
	} else {
		rMultiple = (sig.EntryPrice - price) / riskDist
	}

	decayState := sig.DecayState
	if decayState == "" {
		decayState = "NONE"
	}
	state, action := trader.EvaluateDecay(ageMin, rMultiple, decayState, prof)
	// US4 FR-014: forced flat-before-close — weekend-gap edge (or the
	// horizon) can hit before the decay state machine's checkpoints.
	if action == "" && trader.MustFlatten(time.Now(), sig.CreatedAt, prof) {
		state, action = "CLOSED", "TIME_EXIT"
	}
	switch action {
	case "":
		return
	case "BREAKEVEN":
		// Protect: stop to break-even + round-trip fees (FR-004/005).
		beStop := trader.BreakevenStopPrice(sig.EntryPrice, trader.Direction(sig.Direction), 0.001)
		// Only move the stop in the protective direction.
		moved := beStop > sig.StopLoss && sig.Direction == "LONG" || beStop < sig.StopLoss && sig.Direction == "SHORT"
		if !moved {
			return
		}
		if s.dbStore != nil {
			if err := s.dbStore.UpdateSignalStop(ctx, sig.ID, beStop); err != nil {
				log.Printf("[Decay] failed to move stop for #%d: %v", sig.ID, err)
				return
			}
			if err := s.dbStore.UpdateSignalDecay(ctx, sig.ID, state); err != nil {
				log.Printf("[Decay] failed to persist decay state for #%d: %v", sig.ID, err)
				return
			}
		}
		sig.StopLoss = beStop
		sig.DecayState = state
		log.Printf("[Decay] signal #%d %s protected: stop -> %.6f (age %.0fm, %.2fR)", sig.ID, sig.Symbol, beStop, ageMin, rMultiple)
	case "CLOSE", "TIME_EXIT":
		// Close the dead trade at the live price (never persist zero: the
		// close path settles PnL/ROI like every other exit).
		signalSvc := s.newSignalService(ctx)
		pnlUSD, roiPct, err := signalSvc.CloseSignalNow(ctx, sig, price, "TIME_EXIT")
		if err != nil {
			log.Printf("[Decay] failed to close signal #%d: %v", sig.ID, err)
			return
		}
		if s.execEngine != nil {
			if closedTrade, exited := s.execEngine.ForceClosePosition(sig.Symbol, price, "TIME_EXIT"); exited {
				log.Printf("[Decay] Closed %s position for %s: pnl=%.2f", closedTrade.Side, closedTrade.Symbol, closedTrade.RealizedPnL)
			}
		}
		_ = s.dbStore.UpdateSignalDecay(context.WithoutCancel(ctx), sig.ID, "CLOSED")
		log.Printf("[Decay] signal #%d %s closed by decay: pnl=%.2f roi=%.2f (age %.0fm, %.2fR)", sig.ID, sig.Symbol, pnlUSD, roiPct, ageMin, rMultiple)
	}
}

// livePriceFor returns the freshest cached price for a symbol.
func (s *Server) livePriceFor(ctx context.Context, symbol string) float64 {
	if s.redisClient != nil {
		if quote, err := s.redisClient.GetTicker(ctx, symbol); err == nil && quote != nil && quote.Price > 0 {
			return quote.Price
		}
	}
	if s.marketData != nil {
		if p, err := s.marketData.GetLatestPrice(symbol); err == nil && p > 0 {
			return p
		}
	}
	return 0
}

// StartSignalReconciler runs the reconciliation sweep periodically.
func (s *Server) StartSignalReconciler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Minute
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.ReconcileActiveSignals(ctx)
			}
		}
	}()
}

// CheckSignalExitForTick inspects if an active futures signal exists for the incoming tick
// and automatically executes Take Profit or Stop Loss resolution if triggered.
func (s *Server) CheckSignalExitForTick(tick cache.TickerQuote) {
	if s.dbStore == nil || tick.Price <= 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sig, err := s.dbStore.GetActiveFuturesSignalBySymbol(ctx, tick.Symbol)
	if err != nil || sig == nil || sig.Status != "ACTIVE" {
		return
	}

	signalSvc := s.newSignalService(ctx)
	resolved, exitReason, pnl, roi, err := signalSvc.CheckSignalResolution(ctx, sig, tick.Price)
	if err != nil || !resolved {
		return
	}

	// 1. Mark signal as CLOSED in database
	if sig.ID > 0 {
		_ = s.dbStore.CloseFuturesSignal(ctx, sig.ID, tick.Price, exitReason, pnl, roi)
	}

	// 1b. Close corresponding execution engine position
	if s.execEngine != nil {
		if closedTrade, exited := s.execEngine.CheckExit(sig.Symbol, tick.Price); exited {
			log.Printf("[Signal→Position] Closed %s position for %s: reason=%s pnl=%.2f",
				closedTrade.Side, closedTrade.Symbol, exitReason, closedTrade.RealizedPnL)
		}
	}

	// 2. Broadcast resolution via SSE
	if s.broadcaster != nil {
		closePayload, _ := json.Marshal(map[string]interface{}{
			"id":          sig.ID,
			"symbol":      sig.Symbol,
			"exit_price":  tick.Price,
			"exit_reason": exitReason,
			"pnl_usd":     pnl,
			"roi_pct":     roi,
		})
		s.broadcaster.Broadcast("futures_signal_closed", string(closePayload))
	}

	// 3. Broadcast resolution via Telegram Bot if active and not already resolved
	if s.telegramBot != nil && s.telegramBot.GetConfig().Enabled && !sig.TelegramResolved {
		s.telegramBot.BroadcastSignalResolution(sig, tick.Price, exitReason, pnl, roi)
		if sig.ID > 0 {
			_ = s.dbStore.MarkSignalResolved(ctx, sig.ID)
			sig.TelegramResolved = true
		}
	}

	// 4. Online fine-tuning telemetry update
	if s.sampler != nil {
		s.sampler.RecordOutcome(tradeOutcomeFromSignal(sig, pnl, roi))
	}
}

// tradeOutcomeFromSignal converts a closed signal into the learning outcome
// fed to Thompson sampling (spec 012 US7, FR-022/023). The indicator fields
// come from the decision-time snapshot recorded with the signal; rows without
// a snapshot keep HasSnapshot=false and move no statistic.
func tradeOutcomeFromSignal(sig *db.FuturesTradeSignal, pnlUSD, roiPct float64) ai.TradeOutcome {
	side := "BUY"
	if sig.Direction == "SHORT" {
		side = "SELL"
	}
	out := ai.TradeOutcome{
		Symbol:      sig.Symbol,
		Side:        side,
		Pnl:         pnlUSD,
		ReturnPct:   roiPct,
		HasSnapshot: false,
	}
	if len(sig.IndicatorSnapshot) == 0 {
		return out
	}
	var rec db.IndicatorSnapshotRecord
	if err := json.Unmarshal(sig.IndicatorSnapshot, &rec); err != nil {
		return out
	}
	out.SuperTrendTrend = rec.SuperTrend
	out.RSI = rec.RSI
	out.MACDHistogram = rec.MACDHistogram
	out.CMF = rec.CMF
	out.KaufmanER = rec.KaufmanER
	out.OBI = rec.OBI
	out.Divergence = rec.Divergence
	out.HasSnapshot = true
	return out
}

// StartBackgroundSignalScanner runs transparent background scans across the active asset universe.
func (s *Server) StartBackgroundSignalScanner(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Minute
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Run an initial scan shortly after startup (after feeds initialize)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
			s.runBackgroundScan(ctx)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runBackgroundScan(ctx)
			}
		}
	}()
}

// scanUniverse returns the symbols the autonomous scanner should evaluate:
// the screener's ACTIVE universe when available, otherwise the full catalog
// (before the first screener pass completes).
func (s *Server) scanUniverse() []string {
	if s.screener != nil {
		if qualified := s.screener.GetActiveUniverse(); len(qualified) > 0 {
			return qualified
		}
	}
	allAssets := market.GetSupportedAssets()
	symbols := make([]string, 0, len(allAssets))
	for _, a := range allAssets {
		symbols = append(symbols, a.Symbol)
	}
	return symbols
}

func (s *Server) runBackgroundScan(ctx context.Context) {
	if s.aiClient == nil {
		return
	}

	maxActive := 5
	if s.cfg != nil && s.cfg.MaxConcurrentSignals > 0 {
		maxActive = s.cfg.MaxConcurrentSignals
	}
	if s.dbStore != nil {
		activeSignals, err := s.dbStore.ListFuturesSignals(ctx, "ACTIVE", 20)
		if err == nil && len(activeSignals) >= maxActive {
			return
		}
	}

	// Scan the liquidity-qualified universe produced by the screener, not the
	// whole catalog: with 100+ instruments a full pass every 2 minutes would
	// burn AI budget on assets the screener has already rejected.
	symbols := s.scanUniverse()
	if len(symbols) == 0 {
		return
	}

	var newsHeadlines []string
	if s.newsCrawler != nil {
		latest := s.newsCrawler.GetLatestArticles()
		for i := 0; i < len(latest) && i < 15; i++ {
			newsHeadlines = append(newsHeadlines, latest[i].Title)
		}
	}

	for _, sym := range symbols {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Skip if symbol already has an active signal
		if s.dbStore != nil {
			if existing, err := s.dbStore.GetActiveFuturesSignalBySymbol(ctx, sym); err == nil && existing != nil {
				continue
			}
		}

		// spec-020 US-A3: never evaluate the same symbol concurrently with a
		// news-triggered run (and vice versa).
		unlock := s.lockSymbol(sym)
		if unlock == nil {
			log.Printf("[BackgroundScan] %s busy (news-triggered eval running) — skip", sym)
			continue
		}

		s.markEvaluated(sym)
		_, holdReason, scanErr := s.EvaluateSymbolSignal(ctx, sym, newsHeadlines)
		unlock() // evaluation finished — release the symbol (overlap guard)
		if scanErr != nil {
			log.Printf("[BackgroundScan] %s error: %v", sym, scanErr)
		} else if holdReason != "" {
			log.Printf("[BackgroundScan] %s HOLD: %s", sym, holdReason)
		}

		// Re-check the concurrency cap after each evaluation: overshoot
		// is possible between the pre-scan check and this loop.
		if s.dbStore != nil {
			if active, err := s.dbStore.ListFuturesSignals(ctx, "ACTIVE", maxActive+1); err == nil && len(active) >= maxActive {
				log.Printf("[BackgroundScan] Halting scan at concurrency cap (%d active signals).", len(active))
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// SignalSummary is the response payload for GET /api/v1/signals/summary
// (contracts/api.md §2). All ratios are computed server-side so old clients
// and the web UI stay display-agnostic.
type SignalSummary struct {
	Profile       string  `json:"profile"`
	Closed        int     `json:"closed"`
	Wins          int     `json:"wins"`
	Losses        int     `json:"losses"`
	Flat          int     `json:"flat"`
	WinRate       float64 `json:"win_rate"`
	AvgWinPct     float64 `json:"avg_win_pct"`
	AvgLossPct    float64 `json:"avg_loss_pct"`
	PayoffRatio   float64 `json:"payoff_ratio"`
	ExpectancyPct float64 `json:"expectancy_pct"`
	TotalPnlUSD   float64 `json:"total_pnl_usd"`
	StopOutRate   float64 `json:"stop_out_rate"`
	Tp1HitRate    float64 `json:"tp1_hit_rate"`
}

// SignalSummaryHandler handles GET /api/v1/signals/summary?profile=&since=
func (s *Server) SignalSummaryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	profile := r.URL.Query().Get("profile")
	since := r.URL.Query().Get("since")

	cacheKey := fmt.Sprintf("cache:signals:summary:%s:%s", profile, since)
	if s.redisClient != nil {
		if cached, err := s.redisClient.Get(r.Context(), cacheKey); err == nil && len(cached) > 0 {
			w.Write(cached)
			return
		}
	}

	summary := SignalSummary{Profile: profile}
	if s.dbStore == nil || s.dbStore.Pool == nil {
		json.NewEncoder(w).Encode(summary)
		return
	}

	var (
		wins, losses, flat, closed int
		stopLosses, tpHits         int
		sumWin, sumLoss            *float64
		sumPnl                     float64
	)
	err := s.dbStore.Pool.QueryRow(r.Context(), `
		SELECT
			count(*)::int AS closed,
			count(*) FILTER (WHERE realized_roi_pct > 0)::int AS wins,
			count(*) FILTER (WHERE realized_roi_pct < 0)::int AS losses,
			count(*) FILTER (WHERE realized_roi_pct = 0)::int AS flat,
			avg(realized_roi_pct) FILTER (WHERE realized_roi_pct > 0) AS sum_win,
			avg(realized_roi_pct) FILTER (WHERE realized_roi_pct < 0) AS sum_loss,
			coalesce(sum(realized_pnl_usd), 0) AS sum_pnl,
			count(*) FILTER (WHERE exit_reason = 'STOP_LOSS')::int AS stop_losses,
			count(*) FILTER (WHERE exit_reason LIKE 'TP%')::int AS tp_hits
		FROM futures_trade_signals
		WHERE status = 'CLOSED'
		  AND ($1 = '' OR profile = $1)
		  AND ($2 = '' OR created_at >= nullif($2, '')::timestamptz)
	`, profile, since).Scan(
		&closed, &wins, &losses, &flat,
		&sumWin, &sumLoss, &sumPnl,
		&stopLosses, &tpHits,
	)
	if err != nil {
		http.Error(w, `{"error":"failed to compute summary: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	summary.Closed = closed
	summary.Wins = wins
	summary.Losses = losses
	summary.Flat = flat
	summary.TotalPnlUSD = sumPnl
	if sumWin != nil {
		summary.AvgWinPct = *sumWin
	}
	if sumLoss != nil {
		summary.AvgLossPct = *sumLoss
	}
	if closed > 0 {
		summary.WinRate = float64(wins) / float64(closed)
		summary.StopOutRate = float64(stopLosses) / float64(closed)
		summary.Tp1HitRate = float64(tpHits) / float64(closed)
		if wins > 0 && losses > 0 && summary.AvgLossPct != 0 {
			summary.PayoffRatio = summary.AvgWinPct / absFloat(summary.AvgLossPct)
		}
		if wins > 0 || losses > 0 {
			summary.ExpectancyPct = (float64(wins)*summary.AvgWinPct +
				float64(losses)*summary.AvgLossPct) / float64(closed)
		}
	}
	data, err := json.Marshal(summary)
	if err != nil {
		http.Error(w, `{"error":"failed to marshal summary"}`, http.StatusInternalServerError)
		return
	}
	if s.redisClient != nil {
		_ = s.redisClient.Set(r.Context(), cacheKey, data, 30*time.Second)
	}
	w.Write(data)
}

// absFloat returns the absolute value of v.
func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// ListEntryFilterLogsHandler handles GET /api/v1/signals/filters?limit=&rule=&symbol=
func (s *Server) ListEntryFilterLogsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	limit := 50
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	rule := r.URL.Query().Get("rule")
	symbol := r.URL.Query().Get("symbol")

	if s.dbStore == nil {
		json.NewEncoder(w).Encode([]db.EntryFilterLog{})
		return
	}
	logs, err := s.dbStore.ListEntryFilterLogs(r.Context(), limit, rule, symbol)
	if err != nil {
		http.Error(w, `{"error":"failed to fetch entry filter log: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(logs)
}

// ListRiskProfilesHandler handles GET /api/v1/risk-profiles (contracts/api.md §4).
func (s *Server) ListRiskProfilesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.dbStore == nil {
		json.NewEncoder(w).Encode([]db.RiskProfileRow{})
		return
	}
	profiles, err := s.dbStore.ListRiskProfiles(r.Context())
	if err != nil {
		http.Error(w, `{"error":"failed to fetch risk profiles: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(profiles)
}
