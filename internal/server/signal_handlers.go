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
	return trader.NewSignalService(s.dbStore, s.aiClient, sigCfg)
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

// EvaluateSymbolSignal evaluates a single symbol against breaking news catalysts and technical confluence.
// If high conviction is detected, it persists the signal, broadcasts via SSE and Telegram, and returns the signal.
func (s *Server) EvaluateSymbolSignal(ctx context.Context, symbol, bucket string, headlines []string) (*db.FuturesTradeSignal, error) {
	if s.aiClient == nil {
		return nil, errors.New("ai client not configured")
	}

	if symbol == "" || symbol == "BTC/USD" {
		symbol = "BTC/USDT"
	}
	bucket = market.GetBucket(symbol)

	// Ingest latest breaking news headlines from crawler if not provided
	if len(headlines) == 0 && s.newsCrawler != nil {
		latest := s.newsCrawler.GetLatestArticles()
		for i := 0; i < len(latest) && i < 5; i++ {
			headlines = append(headlines, latest[i].Title)
		}
	}

	// Macroeconomic Calendar Halt Guard (FR-005)
	// If a high-impact release is pending within the halt window, halt opening new trades to protect capital
	if s.calendar != nil {
		if halted, reason := s.calendar.IsSymbolHalted(symbol, time.Now()); halted {
			log.Printf("[MACRO HALT] Signal generation halted for %s due to high-impact macro event: %s", symbol, reason)
			return nil, nil // Preserves capital in HOLD
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
		return nil, fmt.Errorf("live market price unavailable for %s", symbol)
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
		if aac, ok := allocBreakdown["tier3_tactical"].(float64); ok && aac > 0 {
			availableAlphaCapital = aac
		}
	}

	// 1. Calculate reserved capital across all currently ACTIVE signals and open positions
	reservedCapital := 0.0
	activeSignalsCount := 0
	if s.dbStore != nil {
		activeSignals, err := s.dbStore.ListFuturesSignals(ctx, "ACTIVE", 50)
		if err == nil && activeSignals != nil {
			activeSignalsCount = len(activeSignals)
			for _, as := range activeSignals {
				if as.Symbol != symbol {
					reservedCapital += as.AllocatedCapitalUSD
				}
			}
		}
	}
	if s.execEngine != nil {
		for _, tr := range s.execEngine.GetOpenTrades() {
			if tr != nil && tr.Symbol != symbol {
				lev := tr.Leverage
				if lev < 1 {
					lev = 1
				}
				margin := (tr.PositionSize * tr.EntryPrice) / float64(lev)
				reservedCapital += margin
			}
		}
	}

	// 2. Concurrency & Correlated Commodity Exposure Gating
	if s.dbStore != nil {
		existing, err := s.dbStore.GetActiveFuturesSignalBySymbol(ctx, symbol)
		if err == nil && existing != nil {
			// Signal already active for this symbol; return existing without duplicate notifications
			return existing, nil
		}

		// Correlated Commodity Guard: prevent concurrent active signals across correlated assets in same exposure group
		activeSignals, err := s.dbStore.ListFuturesSignals(ctx, "ACTIVE", 50)
		if err == nil && activeSignals != nil {
			for _, as := range activeSignals {
				if market.AreCorrelatedCommodities(as.Symbol, symbol) {
					// Correlated commodity (e.g. PAXG & XAU) already active; prevent duplicate risk and cash splitting
					return nil, nil // HOLD
				}
			}
		}

		maxActive := 5
		if s.cfg != nil && s.cfg.MaxConcurrentSignals > 0 {
			maxActive = s.cfg.MaxConcurrentSignals
		}
		if activeSignalsCount >= maxActive {
			// Capital guard: dynamically bounded active signals permitted
			return nil, nil // HOLD
		}
	}

	if s.execEngine != nil {
		for _, tr := range s.execEngine.GetOpenTrades() {
			if tr != nil && market.AreCorrelatedCommodities(tr.Symbol, symbol) {
				// Correlated commodity already open in execution engine
				return nil, nil // HOLD
			}
		}
	}

	// 3. Dynamic capital reservation
	unreservedAlphaCapital := availableAlphaCapital - reservedCapital
	minRequiredUnreserved := totalEquity * 0.05
	if unreservedAlphaCapital < minRequiredUnreserved {
		return nil, nil // HOLD - capital fully reserved in active trades
	}

	signalSvc := s.newSignalService()
	sig, err := signalSvc.EvaluateMarketSignal(
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
		return nil, fmt.Errorf("signal generation failed: %w", err)
	}

	if sig == nil {
		return nil, nil // HOLD
	}

	// Broadcast signal via SSE and Telegram ONLY if brand-new and not yet dispatched
	if !sig.TelegramDispatched {
		// Wire signal into execution engine as a live position
		if s.execEngine != nil && sig.ID > 0 {
			if trade, err := s.execEngine.OpenPositionFromSignal(sig); err == nil && trade != nil {
				// Broadcast the new trade via SSE so the frontend positions table updates
				if s.broadcaster != nil {
					if tradeBytes, err := json.Marshal(trade); err == nil {
						s.broadcaster.Broadcast("trade", string(tradeBytes))
					}
				}
				log.Printf("[Signal→Position] Opened %s position for %s at $%.2f (margin=$%.2f, lev=%dx)",
					sig.Direction, sig.Symbol, sig.EntryPrice, sig.AllocatedCapitalUSD, sig.Leverage)
			} else if err != nil {
				log.Printf("[Signal→Position] Failed to open position for %s: %v", sig.Symbol, err)
			}
		}

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

	return sig, nil
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

	sig, err := s.EvaluateSymbolSignal(r.Context(), req.Symbol, req.Bucket, req.NewsHeadlines)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if sig == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "HOLD",
			"message": "No high-conviction breaking news catalyst detected. Capital preserved in HOLD.",
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
		allAssets := market.GetSupportedAssets()
		switch bucket {
		case "CORE":
			for _, a := range allAssets {
				if a.Bucket == "CORE" {
					symbols = append(symbols, a.Symbol)
				}
			}
		case "ALPHA":
			for _, a := range allAssets {
				if a.Bucket == "ALPHA" {
					symbols = append(symbols, a.Symbol)
				}
			}
		default: // "ALL" or unrecognized
			for _, a := range allAssets {
				symbols = append(symbols, a.Symbol)
			}
		}
	}

	// Ingest latest breaking news headlines once for the batch
	var newsHeadlines []string
	if s.newsCrawler != nil {
		latest := s.newsCrawler.GetLatestArticles()
		for i := 0; i < len(latest) && i < 8; i++ {
			newsHeadlines = append(newsHeadlines, latest[i].Title)
		}
	}

	results := make([]AssetScanResult, len(symbols))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 12) // concurrency limit

	for i, sym := range symbols {
		wg.Add(1)
		go func(idx int, symbol string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			b := market.GetBucket(symbol)

			evalCtx, evalCancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer evalCancel()

			sig, err := s.EvaluateSymbolSignal(evalCtx, symbol, b, newsHeadlines)
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
					Message: "No breaking catalyst found; capital preserved.",
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
	_, _, pnl, roi, err := signalSvc.CheckSignalResolution(r.Context(), targetSig, req.ExitPrice)
	if err != nil {
		http.Error(w, `{"error":"failed to resolve signal: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
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

	allAssets := market.GetSupportedAssets()
	symbols := make([]string, len(allAssets))
	for i, a := range allAssets {
		symbols[i] = a.Symbol
	}

	var newsHeadlines []string
	if s.newsCrawler != nil {
		latest := s.newsCrawler.GetLatestArticles()
		for i := 0; i < len(latest) && i < 8; i++ {
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

		_, _ = s.EvaluateSymbolSignal(ctx, sym, bucket, newsHeadlines)
		time.Sleep(500 * time.Millisecond)
	}
}
