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
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/market"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

// ListFuturesSignalsHandler handles GET /api/v1/signals/futures
func (s *Server) newSignalService() *trader.SignalService {
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
		sigCfg = trader.DefaultSignalConfig()
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
	// Serialise the final cap check so decide-all and the background scanner
	// cannot both pass an open-slot read and overshoot MAX_CONCURRENT_SIGNALS.
	svc.SetSlotGuard(func() error {
		s.signalSlotMu.Lock()
		defer s.signalSlotMu.Unlock()
		maxActive := 5
		if s.cfg != nil && s.cfg.MaxConcurrentSignals > 0 {
			maxActive = s.cfg.MaxConcurrentSignals
		}
		if s.dbStore == nil {
			return nil
		}
		active, err := s.dbStore.ListFuturesSignals(context.Background(), "ACTIVE", 100)
		if err != nil {
			return nil
		}
		if len(active) >= maxActive {
			return fmt.Errorf("%w (%d/%d)", trader.ErrConcurrentCap, len(active), maxActive)
		}
		return nil
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

	if s.dbStore == nil {
		json.NewEncoder(w).Encode([]db.FuturesTradeSignal{})
		return
	}

	signals, err := s.dbStore.ListFuturesSignals(r.Context(), status, limit)
	if err != nil {
		http.Error(w, `{"error":"failed to fetch futures signals: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if signals == nil {
		signals = []db.FuturesTradeSignal{}
	}

	json.NewEncoder(w).Encode(signals)
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

// EvaluateSymbolSignal evaluates a single symbol against breaking news catalysts and technical confluence.
// If high conviction is detected, it persists the signal, broadcasts via SSE and Telegram, and returns the signal.
func (s *Server) EvaluateSymbolSignal(ctx context.Context, symbol, bucket string, headlines []string) (*db.FuturesTradeSignal, string, error) {
	if s.aiClient == nil {
		return nil, "AI client not configured", errors.New("ai client not configured")
	}

	if symbol == "" || symbol == "BTC/USD" {
		symbol = "BTC/USDT"
	}
	bucket = market.GetBucket(symbol)

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
	if len(headlines) == 0 {
		return nil, "No asset-relevant news headline for " + symbol, nil
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

	signalSvc := s.newSignalService()
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
			if decision.Reasoning != "" {
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

	sig, holdReason, err := s.EvaluateSymbolSignal(r.Context(), req.Symbol, req.Bucket, req.NewsHeadlines)
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

			b := market.GetBucket(symbol)

			evalCtx, evalCancel := context.WithTimeout(batchCtx, 20*time.Second)
			defer evalCancel()

			sig, holdReason, err := s.EvaluateSymbolSignal(evalCtx, symbol, b, newsHeadlines)
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

	signalSvc := s.newSignalService()
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
		side := "BUY"
		if targetSig.Direction == "SHORT" {
			side = "SELL"
		}
		s.sampler.RecordOutcome(ai.TradeOutcome{
			Symbol:    targetSig.Symbol,
			Side:      side,
			Pnl:       pnl,
			ReturnPct: roi,
		})
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
			_ = s.dbStore.CloseFuturesSignal(ctx, sig.ID, price, exitReason, 0, 0)
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
		signalSvc := s.newSignalService()
		resolved, exitReason, pnl, roi, err := signalSvc.CheckSignalResolution(ctx, sig, price)
		if err != nil || !resolved {
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

	signalSvc := s.newSignalService()
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
		side := "BUY"
		if sig.Direction == "SHORT" {
			side = "SELL"
		}
		s.sampler.RecordOutcome(ai.TradeOutcome{
			Symbol:    sig.Symbol,
			Side:      side,
			Pnl:       pnl,
			ReturnPct: roi,
		})
	}
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

		bucket := market.GetBucket(sym)

		_, holdReason, scanErr := s.EvaluateSymbolSignal(ctx, sym, bucket, newsHeadlines)
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
