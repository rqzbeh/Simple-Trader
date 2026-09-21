package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if bucket == "" {
		bucket = "ALPHA"
	}

	// Ingest latest breaking news headlines from crawler if not provided
	if len(headlines) == 0 && s.newsCrawler != nil {
		latest := s.newsCrawler.GetLatestArticles()
		for i := 0; i < len(latest) && i < 5; i++ {
			headlines = append(headlines, latest[i].Title)
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

	snap := cache.IndicatorSnapshot{
		Symbol:          symbol,
		RSI:             55.0,
		SuperTrend:      "BULL",
		Histogram:       1.5,
		ConfluenceScore: 0.80,
	}

	if s.redisClient != nil {
		if ind, err := s.redisClient.GetIndicatorSnapshot(ctx, symbol); err == nil && ind != nil {
			snap = *ind
		}
	}

	// Portfolio equity and alpha capital
	totalEquity := 10000.0
	availableAlphaCapital := 5000.0
	if s.cfg != nil {
		if s.cfg.InitialCapital > 0 {
			totalEquity = s.cfg.InitialCapital
		}
		if s.cfg.AlphaTargetPct > 0 {
			availableAlphaCapital = totalEquity * s.cfg.AlphaTargetPct
		}
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

	signalSvc := trader.NewSignalService(s.dbStore, s.aiClient)
	sig, err := signalSvc.EvaluateMarketSignal(
		ctx,
		symbol,
		bucket,
		quote,
		snap,
		nil,
		headlines,
		totalEquity,
		availableAlphaCapital,
	)
	if err != nil {
		return nil, fmt.Errorf("signal generation failed: %w", err)
	}

	if sig == nil {
		return nil, nil // HOLD
	}

	// Broadcast signal via SSE
	if s.broadcaster != nil {
		if sigBytes, err := json.Marshal(sig); err == nil {
			s.broadcaster.Broadcast("futures_signal", string(sigBytes))
		}
	}

	// Broadcast signal via Telegram Bot if active
	if s.telegramBot != nil && s.telegramBot.GetConfig().Enabled {
		s.telegramBot.BroadcastSignalEntry(sig)
		if s.dbStore != nil && sig.ID > 0 {
			_ = s.dbStore.MarkSignalDispatched(ctx, sig.ID)
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
	if len(symbols) == 0 {
		if s.screener != nil {
			symbols = s.screener.GetActiveUniverse()
		}
		if len(symbols) == 0 {
			symbols = []string{
				"BTC/USDT", "ETH/USDT", "SOL/USDT", "PAXG/USDT",
				"BNB/USDT", "XRP/USDT", "LINK/USDT", "EUR/USDT",
			}
		}
	}

	bucket := req.Bucket
	if bucket == "" {
		bucket = "ALPHA"
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
	sem := make(chan struct{}, 4) // concurrency limit

	for i, sym := range symbols {
		wg.Add(1)
		go func(idx int, symbol string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			b := bucket
			if symbol == "PAXG/USDT" || symbol == "XAG/USDT" {
				b = "CORE"
			}

			sig, err := s.EvaluateSymbolSignal(r.Context(), symbol, b, newsHeadlines)
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

	signalSvc := trader.NewSignalService(s.dbStore, s.aiClient)
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

	signalSvc := trader.NewSignalService(s.dbStore, s.aiClient)
	resolved, exitReason, pnl, roi, err := signalSvc.CheckSignalResolution(ctx, sig, tick.Price)
	if err != nil || !resolved {
		return
	}

	// 1. Broadcast resolution via SSE
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

	// 2. Broadcast resolution via Telegram Bot if active
	if s.telegramBot != nil && s.telegramBot.GetConfig().Enabled {
		s.telegramBot.BroadcastSignalResolution(sig, tick.Price, exitReason, pnl, roi)
		if sig.ID > 0 {
			_ = s.dbStore.MarkSignalResolved(ctx, sig.ID)
		}
	}

	// 3. Online fine-tuning telemetry update
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

	symbols := []string{
		"BTC/USDT", "ETH/USDT", "SOL/USDT", "PAXG/USDT",
		"BNB/USDT", "XRP/USDT", "LINK/USDT", "EUR/USDT",
	}
	if s.screener != nil {
		if univ := s.screener.GetActiveUniverse(); len(univ) > 0 {
			symbols = univ
		}
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

		bucket := "ALPHA"
		if sym == "PAXG/USDT" || sym == "XAG/USDT" {
			bucket = "CORE"
		}

		_, _ = s.EvaluateSymbolSignal(ctx, sym, bucket, newsHeadlines)
		time.Sleep(500 * time.Millisecond)
	}
}
