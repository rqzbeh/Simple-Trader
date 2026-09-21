package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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

	// Ingest latest breaking news headlines from crawler if not provided
	if len(req.NewsHeadlines) == 0 && s.newsCrawler != nil {
		latest := s.newsCrawler.GetLatestArticles()
		for i := 0; i < len(latest) && i < 5; i++ {
			req.NewsHeadlines = append(req.NewsHeadlines, latest[i].Title)
		}
	}

	// Fetch current market price
	currentPrice := 0.0
	if s.redisClient != nil {
		if quote, err := s.redisClient.GetTicker(r.Context(), req.Symbol); err == nil && quote != nil && quote.Price > 0 {
			currentPrice = quote.Price
		}
	}
	if currentPrice <= 0 && s.marketData != nil {
		if p, err := s.marketData.GetLatestPrice(req.Symbol); err == nil && p > 0 {
			currentPrice = p
		}
	}
	if currentPrice <= 0 {
		fetcher := market.NewBinanceFetcher()
		cleanSym := strings.ReplaceAll(req.Symbol, "/", "")
		if q, err := fetcher.FetchTicker(r.Context(), cleanSym); err == nil && q != nil && q.Price > 0 {
			currentPrice = q.Price
			if s.marketData != nil {
				s.marketData.UpdateQuote(*q)
			}
		}
	}
	if currentPrice <= 0 {
		http.Error(w, `{"error":"live market price unavailable for `+req.Symbol+`"}`, http.StatusServiceUnavailable)
		return
	}

	quote := cache.TickerQuote{
		Symbol: req.Symbol,
		Price:  currentPrice,
	}

	snap := cache.IndicatorSnapshot{
		Symbol:          req.Symbol,
		RSI:             55.0,
		SuperTrend:      "BULL",
		Histogram:       1.5,
		ConfluenceScore: 0.80,
	}

	if s.redisClient != nil {
		if ind, err := s.redisClient.GetIndicatorSnapshot(r.Context(), req.Symbol); err == nil && ind != nil {
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
		r.Context(),
		req.Symbol,
		req.Bucket,
		quote,
		snap,
		nil,
		req.NewsHeadlines,
		totalEquity,
		availableAlphaCapital,
	)
	if err != nil {
		http.Error(w, `{"error":"signal generation failed: `+err.Error()+`"}`, http.StatusInternalServerError)
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
			_ = s.dbStore.MarkSignalDispatched(r.Context(), sig.ID)
		}
	}

	json.NewEncoder(w).Encode(sig)
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
