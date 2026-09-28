package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/backtest"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/market"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

func (s *Server) setupRoutes() {
	r := s.router

	// Standard middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Cross-Origin Resource Sharing (CORS) for PWA Frontend
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "healthy",
			"version":  "3.0.0-decision-core",
			"model_id": s.cfg.AIModelID,
		})
	})

	s.registerSystemRoutes(r)
	s.registerMarketRoutes(r)
	s.registerInvestorRoutes(r)
	s.registerSignalRoutes(r)
	s.registerAuthRoutes(r)
	s.registerStaticRoutes(r)
}

func (s *Server) registerSystemRoutes(r chi.Router) {
	// System Runtime Configuration Telemetry (binds live .env to UI)
	r.Get("/api/v1/system/config", s.handleGetSystemConfig)
	r.Put("/api/v1/system/config", s.handlePutSystemConfig)
	r.Get("/api/v1/system/stats", s.handleSystemStats)

	// Real-Time Server-Sent Events (SSE)
	r.Get("/api/v1/events", s.broadcaster.ServeHTTP)
	r.Get("/api/admin/shadow/report", s.handleShadowReport)

	// Dynamic Indicator Weights
	r.Get("/api/v1/weights", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		weights := make(map[string]float64)
		if s.sampler != nil {
			weights = s.sampler.GetWeights()
		}
		if len(weights) == 0 {
			weights = map[string]float64{
				"RSI":            1.0,
				"MACD":           1.0,
				"SUPERTREND":     1.0,
				"MICROSTRUCTURE": 1.0,
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"weights": weights,
		})
	})

	r.Post("/api/v1/weights", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.sampler == nil {
			http.Error(w, `{"error":"weight sampler not configured"}`, http.StatusServiceUnavailable)
			return
		}
		var req struct {
			Weights map[string]float64 `json:"weights"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Weights) == 0 {
			http.Error(w, `{"error":"body must contain a non-empty weights map"}`, http.StatusBadRequest)
			return
		}
		s.sampler.SetWeights(req.Weights)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"weights": s.sampler.GetWeights(),
		})
	})

	// Continuous Fine-Tuning JSONL Export
	r.Get("/api/v1/learning/dataset.jsonl", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		if s.dbStore == nil {
			http.Error(w, `{"error":"database not available for training data export"}`, http.StatusServiceUnavailable)
			return
		}
		signals, err := s.dbStore.ListFuturesSignals(r.Context(), "CLOSED", 100)
		if err != nil || len(signals) == 0 {
			http.Error(w, `{"error":"no closed signals available for training"}`, http.StatusNotFound)
			return
		}
		for _, sig := range signals {
			pair := ai.FineTunePair{
				SystemPrompt:      "You are Simple-Trader AI strategy core. Analyze market conditions and provide trading decisions.",
				UserPrompt:        fmt.Sprintf("Analyze %s at $%.2f with direction=%s, leverage=%d, R:R=%.2f", sig.Symbol, sig.EntryPrice, sig.Direction, sig.Leverage, sig.RiskRewardRatio),
				AssistantResponse: fmt.Sprintf(`{"decision":"%s","confidence":%.2f,"reasoning":"%s"}`, sig.Direction, sig.RiskRewardRatio/5.0, sig.CatalystHeadline),
			}
			line, err := pair.ToJSONL()
			if err == nil {
				w.Write([]byte(line + "\n"))
			}
		}
	})

	// Economic Calendar & Macro Status (FR-005)
	r.Get("/api/v1/calendar", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		events := []market.MacroEvent{}
		if s.calendar != nil {
			events = s.calendar.GetEvents()
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"events": events,
		})
	})

	// Interactive Vectorized Backtest & Monte Carlo Engine (FR-008, SC-004)
	r.Post("/api/v1/backtest/run", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req struct {
			Symbol         string  `json:"symbol"`
			InitialCapital float64 `json:"initial_capital"`
			BarsCount      int     `json:"bars_count"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Symbol == "" || req.Symbol == "BTC/USD" {
			req.Symbol = "BTC/USDT"
		}
		if req.InitialCapital <= 0 {
			req.InitialCapital = 100000.0
		}
		if req.BarsCount <= 0 {
			req.BarsCount = 1000
		}

		if s.candleDownloader == nil {
			http.Error(w, `{"error":"historical market data downloader not initialized"}`, http.StatusInternalServerError)
			return
		}

		candles, err := s.candleDownloader.FetchHistoricalKlines(r.Context(), req.Symbol, "1h", req.BarsCount)
		if err != nil {
			http.Error(w, `{"error":"failed to download historical candles: `+err.Error()+`"}`, http.StatusBadGateway)
			return
		}

		backtestCandles := make([]backtest.Candle, len(candles))
		for i, c := range candles {
			backtestCandles[i] = backtest.Candle{
				Timestamp: c.OpenTime,
				Open:      c.Open,
				High:      c.High,
				Low:       c.Low,
				Close:     c.Close,
				Volume:    c.Volume,
			}
		}

		backtestWeights := map[string]float64{"RSI": 1.0, "MACD": 1.0, "SUPERTREND": 1.0, "MICROSTRUCTURE": 1.0}
		if s.sampler != nil {
			backtestWeights = s.sampler.GetWeights()
		}
		cfg := backtest.BacktestConfig{
			Symbol:            req.Symbol,
			InitialCapital:    req.InitialCapital,
			Friction:          trader.DefaultFrictionModel(),
			Kelly:             trader.DefaultKellyConfig(),
			IndicatorsWeights: backtestWeights,
			RiskFreeRate:      0.04,
		}
		engine := backtest.NewVectorizedEngine(cfg)
		btResult := engine.Run(backtestCandles)
		mcResult := backtest.RunMonteCarlo(btResult.Trades, req.InitialCapital, 1000, 42)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"backtest":    btResult,
			"monte_carlo": mcResult,
		})
	})
}

func (s *Server) registerMarketRoutes(r chi.Router) {
	// Market Assets (Enriched with latest live cached ticker prices)
	r.Get("/api/v1/assets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assets := market.GetSupportedAssets()
		type enrichedAsset struct {
			market.AssetDefinition
			Price     float64 `json:"price"`
			Change24h float64 `json:"change24h"`
			High24h   float64 `json:"high24h"`
			Low24h    float64 `json:"low24h"`
			Volume    float64 `json:"volume"`
		}
		enriched := make([]enrichedAsset, len(assets))
		for i, a := range assets {
			enriched[i] = enrichedAsset{AssetDefinition: a}
			if s.marketData != nil {
				if q, ok := s.marketData.GetQuote(a.Symbol); ok {
					enriched[i].Price = q.Price
					enriched[i].Change24h = q.Change24h
					enriched[i].High24h = q.High24h
					enriched[i].Low24h = q.Low24h
					enriched[i].Volume = q.Volume
				}
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"assets": enriched,
		})
	})

	// Real Exchange Candlestick Klines (Authentic Binance Klines)
	r.Get("/api/v1/klines", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			symbol = "BTC/USDT"
		}
		interval := r.URL.Query().Get("interval")
		if interval == "" {
			interval = "1h"
		}
		limit := 48
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 && parsed <= 1000 {
				limit = parsed
			}
		}

		if s.candleDownloader == nil {
			http.Error(w, `{"error":"historical candle downloader uninitialized"}`, http.StatusInternalServerError)
			return
		}

		candles, err := s.candleDownloader.FetchHistoricalKlines(r.Context(), symbol, interval, limit)
		if err != nil {
			http.Error(w, `{"error":"failed to fetch authentic exchange klines: `+err.Error()+`"}`, http.StatusBadGateway)
			return
		}

		type FormattedCandle struct {
			Time   int64   `json:"time"`
			Open   float64 `json:"open"`
			High   float64 `json:"high"`
			Low    float64 `json:"low"`
			Close  float64 `json:"close"`
			Volume float64 `json:"volume"`
		}
		formatted := make([]FormattedCandle, len(candles))
		for i, c := range candles {
			formatted[i] = FormattedCandle{
				Time:   c.OpenTime.Unix(),
				Open:   c.Open,
				High:   c.High,
				Low:    c.Low,
				Close:  c.Close,
				Volume: c.Volume,
			}
		}

		json.NewEncoder(w).Encode(formatted)
	})

	// Live News Stream & Sentiment (FR-004)
	r.Get("/api/v1/news/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var articles []db.NewsArticle
		if s.newsCrawler != nil {
			articles = s.newsCrawler.GetLatestArticles()
		}
		if articles == nil {
			articles = []db.NewsArticle{}
		}

		var sentiment market.NewsSentimentReport
		if s.newsCrawler != nil {
			rep, err := s.newsCrawler.GetAggregateSentiment()
			if err != nil {
				http.Error(w, "component=news-classifier: "+err.Error(), http.StatusBadGateway)
				return
			}
			sentiment = rep
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"articles":  articles,
			"sentiment": sentiment,
		})
	})

	// Dynamic Liquid Crypto Screener (FR-006)
	r.Get("/api/v1/market/screener", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var assets []db.ScreenedAsset
		var universe []string
		if s.screener != nil {
			assets = s.screener.GetScreenedAssets()
			universe = s.screener.GetActiveUniverse()
		}
		if assets == nil {
			assets = []db.ScreenedAsset{}
		}
		if universe == nil {
			universe = []string{}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"assets":          assets,
			"active_universe": universe,
		})
	})

	// Dynamic Macroeconomic Regime & 3-Tier Allocation (US2, FR-004)
	r.Get("/api/v1/macro/regime", s.GetMacroRegimeHandler)
	r.Post("/api/v1/macro/regime", s.UpdateMacroRegimeHandler)
}

func (s *Server) registerInvestorRoutes(r chi.Router) {
	// Investor Capital Ledger (US1, FR-009, FR-010)
	r.Get("/api/v1/investors", s.ListInvestorsHandler)
	r.Post("/api/v1/investors", s.CreateInvestorHandler)
	r.Get("/api/v1/investors/{id}", s.GetInvestorHandler)
	r.Post("/api/v1/investors/{id}/deposit", s.RecordDepositHandler)
	r.Post("/api/v1/investors/{id}/withdraw", s.RecordWithdrawalHandler)

	// 3-Tier Liquidity Allocation (US2, FR-002, FR-003)
	r.Get("/api/v1/allocator/tiers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.allocator == nil {
			json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		json.NewEncoder(w).Encode(s.allocator.Get3TierBreakdown())
	})
}

func (s *Server) registerSignalRoutes(r chi.Router) {
	// Live Open Paper Trading Positions
	r.Get("/api/v1/positions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.execEngine == nil {
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}
		trades := s.execEngine.GetOpenTrades()
		if trades == nil {
			trades = []*db.Trade{}
		}
		json.NewEncoder(w).Encode(trades)
	})

	// Live Mark-to-Market Portfolio Summary
	r.Get("/api/v1/portfolio/summary", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Redis cache-aside (10s TTL, Constitution Principle II)
		if s.redisClient != nil {
			if cached, err := s.redisClient.Get(r.Context(), "cache:portfolio:summary"); err == nil && len(cached) > 0 {
				w.Write(cached)
				return
			}
		}

		initialEquity := s.cfg.InitialCapital
		targetCorePct := s.cfg.CoreTargetPct
		targetAlphaPct := s.cfg.AlphaTargetPct
		if initialEquity <= 0 {
			initialEquity = 10000.0
		}
		if targetCorePct <= 0 {
			targetCorePct = 0.50
		}
		if targetAlphaPct <= 0 {
			targetAlphaPct = 0.50
		}

		// Root initial capital in PostgreSQL investor ledger if available
		if s.dbStore != nil {
			_, _, netCapital, activeInvestors, err := s.dbStore.GetTotalInvestorCapital(r.Context())
			if err == nil {
				if activeInvestors == 0 {
					_, _ = s.dbStore.EnsureDefaultInvestorProfile(r.Context(), "General Partner / Treasury", "Genesis Capital Allocation & Liquidity Seed", initialEquity)
					_, _, netCapital, _, _ = s.dbStore.GetTotalInvestorCapital(r.Context())
				}
				if netCapital > 0 {
					initialEquity = netCapital
				}
			}
		}

		totalEquity := initialEquity
		cash := initialEquity
		coreEquity := initialEquity * targetCorePct
		alphaEquity := initialEquity * targetAlphaPct

		if s.execEngine != nil {
			cash = s.execEngine.GetCash()
			totalEquity = s.execEngine.GetTotalEquity()
			initialEquity = s.execEngine.GetInitialEquity()
			openTrades := s.execEngine.GetOpenTrades()
			if len(openTrades) > 0 {
				cEq, aEq := s.execEngine.GetBucketEquities()
				if cEq > 0 {
					coreEquity = cEq
				}
				if aEq > 0 {
					alphaEquity = aEq
				}
			}
		}

		if s.allocator != nil {
			breakdown := s.allocator.Get3TierBreakdown()
			if tcp, ok := breakdown["tier2_target_pct"].(float64); ok && tcp > 0 {
				targetCorePct = tcp
			}
			if tap, ok := breakdown["tier3_target_pct"].(float64); ok && tap > 0 {
				targetAlphaPct = tap
			}
		}

		drawdownPct := 0.0
		peakEquity := totalEquity
		if initialEquity > peakEquity {
			peakEquity = initialEquity
		}
		if peakEquity > 0 && totalEquity < peakEquity {
			drawdownPct = ((peakEquity - totalEquity) / peakEquity) * 100.0
		}

		data, err := json.Marshal(map[string]interface{}{
			"totalEquity":          totalEquity,
			"initialEquity":        initialEquity,
			"coreEquity":           coreEquity,
			"alphaEquity":          alphaEquity,
			"targetCorePct":        targetCorePct,
			"targetAlphaPct":       targetAlphaPct,
			"cash":                 cash,
			"peakEquity":           peakEquity,
			"drawdownPct":          drawdownPct,
			"circuitBreakerHalted": false,
		})
		if err != nil {
			http.Error(w, `{"error":"failed to marshal portfolio summary"}`, http.StatusInternalServerError)
			return
		}

		if s.redisClient != nil {
			_ = s.redisClient.Set(r.Context(), "cache:portfolio:summary", data, 10*time.Second)
		}
		w.Write(data)
	})

	// Live AI Trade Decision Engine (OmniRoute / OpenAI Chat Completions)
	r.Post("/api/v1/trade/decide", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.aiClient == nil {
			http.Error(w, `{"error":"ai client not configured"}`, http.StatusServiceUnavailable)
			return
		}

		var req ai.DecisionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid request format: `+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		if req.Symbol == "" {
			req.Symbol = "BTC/USD"
		}
		if req.Bucket == "" {
			req.Bucket = "ALPHA"
		}
		if len(req.NewsHeadlines) == 0 && s.newsCrawler != nil {
			latest := s.newsCrawler.GetLatestArticles()
			for i := 0; i < len(latest) && i < 5; i++ {
				req.NewsHeadlines = append(req.NewsHeadlines, latest[i].Title)
			}
		}

		decision, err := s.aiClient.Analyze(r.Context(), req)
		if err != nil {
			http.Error(w, `{"error":"ai analysis failed: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(decision)
	})

	// Two-Sided Futures Trade Signals & News-First Execution (US1)
	r.Get("/api/v1/signals/futures", s.ListFuturesSignalsHandler)
	r.Post("/api/v1/signals/futures/decide", s.GenerateFuturesSignalHandler)
	r.Post("/api/v1/signals/futures/decide-all", s.GenerateAllFuturesSignalsHandler)
	r.Post("/api/v1/signals/futures/{id}/close", s.CloseFuturesSignalHandler)

	// Signal optimization (spec 012): performance summary + entry-filter audit
	r.Get("/api/v1/signals/summary", s.SignalSummaryHandler)
	r.Get("/api/v1/signals/filters", s.ListEntryFilterLogsHandler)
	r.Get("/api/v1/risk-profiles", s.ListRiskProfilesHandler)
	r.Get("/api/v1/commodities/status", s.ListCommoditiesStatusHandler)
}

func (s *Server) registerAuthRoutes(r chi.Router) {
	// Telegram Signals Bot Integration (US3, FR-007)
	r.Get("/api/v1/telegram/config", s.GetTelegramConfigHandler)
	r.Post("/api/v1/telegram/config", s.UpdateTelegramConfigHandler)
	r.Post("/api/v1/telegram/test", s.TestTelegramHandler)

	// Administrative Authentication & Session Security (US4)
	r.Post("/api/v1/auth/login", s.LoginHandler)
	r.Post("/api/v1/auth/logout", s.LogoutHandler)
	r.Get("/api/v1/auth/session", s.SessionHandler)
}

func (s *Server) registerStaticRoutes(r chi.Router) {
	workDir, _ := os.Getwd()
	distPaths := []string{
		filepath.Join(workDir, "web", "dist"),
		filepath.Join(workDir, "dist"),
		"/app/web/dist",
		"/app/dist",
	}
	for _, p := range distPaths {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			fs := http.FileServer(http.Dir(p))
			r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
				cleanPath := filepath.Clean(req.URL.Path)
				targetPath := filepath.Join(p, cleanPath)
				if fi, err := os.Stat(targetPath); (os.IsNotExist(err) || fi.IsDir()) && !strings.HasPrefix(req.URL.Path, "/api") {
					http.ServeFile(w, req, filepath.Join(p, "index.html"))
					return
				}
				fs.ServeHTTP(w, req)
			})
			break
		}
	}
}
