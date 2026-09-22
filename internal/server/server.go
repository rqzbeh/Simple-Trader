package server

import (
	"context"
	"encoding/json"
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
	"github.com/rqzbeh/simple-trader/internal/auth"
	"github.com/rqzbeh/simple-trader/internal/backtest"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/market"
	"github.com/rqzbeh/simple-trader/internal/telegram"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

// Server encapsulates HTTP routes, middlewares, and services.
type Server struct {
	cfg         *config.Config
	dbStore     *db.Store
	redisClient *cache.Client
	aiClient    *ai.Client
	allocator   *trader.Allocator
	execEngine  *trader.ExecutionEngine
	newsCrawler   *market.NewsCrawler
	screener      *market.DynamicCryptoScreener
	telegramBot   *telegram.BotClient
	authenticator *auth.Authenticator
	broadcaster   *SSEBroadcaster
	sampler       *ai.ThompsonSampler
	gpuTrainer    *ai.GPUTrainer
	realDataPipeline *ai.RealDataPipeline
	router        *chi.Mux
	marketData    *trader.LiveMarketData
	candleDownloader market.HistoricalKlineProvider
}

// NewServer configures routes and dependency injection.
func NewServer(
	cfg *config.Config,
	dbStore *db.Store,
	redisClient *cache.Client,
	aiClient *ai.Client,
	allocator *trader.Allocator,
	execEngine *trader.ExecutionEngine,
) *Server {
	crawler := market.NewNewsCrawler(market.DefaultNewsFeedConfig(), redisClient, dbStore)
	binanceFetcher := market.NewBinanceFetcher()
	screener := market.NewDynamicCryptoScreener(market.DefaultScreenerConfig(), binanceFetcher, redisClient, dbStore)

	tgBot := telegram.NewBotClient(telegram.BotConfig{
		BotToken: cfg.TelegramBotToken,
		ChatID:   cfg.TelegramChatID,
		Enabled:  cfg.TelegramBotToken != "" && cfg.TelegramChatID != "",
	})

	var authenticator *auth.Authenticator
	if cfg.AdminPassword != "" {
		authenticator, _ = auth.NewAuthenticator(cfg.AdminPassword, redisClient)
	}

	sampler := ai.NewThompsonSampler(0)
	gpuTrainer := ai.NewGPUTrainer("", "", sampler)
	realDataPipeline := ai.NewRealDataPipeline(nil, sampler)
	marketData := trader.NewLiveMarketData()
	candleDownloader := market.NewBinanceHistoricalDownloader()
	if execEngine != nil {
		execEngine.SetPriceProvider(marketData)
	}

	s := &Server{
		cfg:              cfg,
		dbStore:          dbStore,
		redisClient:      redisClient,
		aiClient:         aiClient,
		allocator:        allocator,
		execEngine:       execEngine,
		newsCrawler:      crawler,
		screener:         screener,
		telegramBot:      tgBot,
		authenticator:    authenticator,
		sampler:          sampler,
		gpuTrainer:       gpuTrainer,
		realDataPipeline: realDataPipeline,
		broadcaster:      NewSSEBroadcaster(),
		router:           chi.NewRouter(),
		marketData:       marketData,
		candleDownloader: candleDownloader,
	}

	s.setupRoutes()
	return s
}

// SetCandleDownloader injects a historical candle provider (useful for testing or alternative exchanges).
func (s *Server) SetCandleDownloader(d market.HistoricalKlineProvider) {
	s.candleDownloader = d
}

// MarketData returns the active live market data provider.
func (s *Server) MarketData() *trader.LiveMarketData {
	return s.marketData
}

// IngestTick updates the in-memory quote store, Redis (if active), broadcasts the tick to SSE,
// and checks active futures signals for autonomous Take Profit / Stop Loss resolution.
func (s *Server) IngestTick(tick cache.TickerQuote) {
	if s.marketData != nil {
		s.marketData.UpdateQuote(tick)
	}
	if s.redisClient != nil {
		_ = s.redisClient.SetTicker(context.Background(), tick.Symbol, &tick, 2*time.Minute)
	}
	if tickJSON, err := json.Marshal(tick); err == nil {
		s.broadcaster.Broadcast("tick", string(tickJSON))
	}

	// Autonomously evaluate active futures signals against streaming ticks
	go s.CheckSignalExitForTick(tick)
}

// Router returns the initialized chi router.
func (s *Server) Router() *chi.Mux {
	return s.router
}

// Broadcaster returns the SSE broadcaster.
func (s *Server) Broadcaster() *SSEBroadcaster {
	return s.broadcaster
}

// NewsCrawler returns the news crawler instance.
func (s *Server) NewsCrawler() *market.NewsCrawler {
	return s.newsCrawler
}

// Screener returns the crypto screener instance.
func (s *Server) Screener() *market.DynamicCryptoScreener {
	return s.screener
}

// Authenticator returns the authenticator instance.
func (s *Server) Authenticator() *auth.Authenticator {
	return s.authenticator
}

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

	// Health and Status
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "healthy",
			"version":  "2.0.0-pure-go",
			"model_id": s.cfg.AIModelID,
		})
	})

	// System Runtime Configuration Telemetry (binds live .env to UI)
	r.Get("/api/v1/system/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		maskedKey := ""
		if len(s.cfg.AIAPIKey) > 8 {
			maskedKey = s.cfg.AIAPIKey[:4] + "..." + s.cfg.AIAPIKey[len(s.cfg.AIAPIKey)-4:]
		} else if len(s.cfg.AIAPIKey) > 0 {
			maskedKey = "***"
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"ai_base_url":             s.cfg.AIBaseURL,
			"ai_model_id":             s.cfg.AIModelID,
			"ai_reasoning_effort":     s.cfg.AIReasoningEffort,
			"ai_api_key_configured":   s.cfg.AIAPIKey != "",
			"ai_api_key_masked":       maskedKey,
			"initial_capital":         s.cfg.InitialCapital,
			"core_target_pct":         s.cfg.CoreTargetPct,
			"alpha_target_pct":        s.cfg.AlphaTargetPct,
			"telegram_bot_configured": s.cfg.TelegramBotToken != "" && s.cfg.TelegramChatID != "",
			"telegram_chat_id":        s.cfg.TelegramChatID,
		})
	})

	// Real-Time Server-Sent Events (SSE)
	r.Get("/api/v1/events", s.broadcaster.ServeHTTP)

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

		initialEquity := 10000.0
		targetCorePct := 0.50
		targetAlphaPct := 0.50
		if s.cfg != nil {
			if s.cfg.InitialCapital > 0 {
				initialEquity = s.cfg.InitialCapital
			}
			if s.cfg.CoreTargetPct > 0 {
				targetCorePct = s.cfg.CoreTargetPct
			}
			if s.cfg.AlphaTargetPct > 0 {
				targetAlphaPct = s.cfg.AlphaTargetPct
			}
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
		coreEquity := totalEquity * targetCorePct
		alphaEquity := totalEquity * targetAlphaPct

		if s.execEngine != nil {
			totalEquity = s.execEngine.GetTotalEquity()
			cash = s.execEngine.GetCash()
			initialEquity = s.execEngine.GetInitialEquity()
		}

		if s.allocator != nil {
			breakdown := s.allocator.Get3TierBreakdown()
			if te, ok := breakdown["total_equity"].(float64); ok && te > 0 {
				totalEquity = te
			}
			if c, ok := breakdown["tier1_cash"].(float64); ok && c > 0 {
				cash = c
			}
			if ce, ok := breakdown["tier2_core"].(float64); ok && ce > 0 {
				coreEquity = ce
			}
			if ae, ok := breakdown["tier3_tactical"].(float64); ok && ae > 0 {
				alphaEquity = ae
			}
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

		json.NewEncoder(w).Encode(map[string]interface{}{
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
	})

	// Dynamic Indicator Weights
	r.Get("/api/v1/weights", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return current active indicator weights
		weights := map[string]float64{
			"RSI":            1.15,
			"MACD":           1.20,
			"SUPERTREND":     1.45,
			"MICROSTRUCTURE": 1.50,
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"weights": weights,
		})
	})

	// Continuous Fine-Tuning JSONL Export
	r.Get("/api/v1/learning/dataset.jsonl", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		// Export high-quality training pairs
		pair := ai.FineTunePair{
			SystemPrompt: "You are Simple-Trader AI strategy core.",
			UserPrompt:   "Analyze BTC/USD at $68,500 with SuperTrend=BULL.",
			AssistantResponse: `{"decision":"BUY","confidence":0.88,"reasoning":"Confirmed trend breakout."}`,
		}
		line, _ := pair.ToJSONL()
		w.Write([]byte(line + "\n"))
	})

	// Economic Calendar & Macro Status (FR-005)
	r.Get("/api/v1/calendar", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		cal := market.NewEconomicCalendar(15 * time.Minute)
		cal.AddEvents(
			market.MacroEvent{
				ID:          "FOMC-RATE-DECISION",
				Title:       "FOMC Federal Funds Rate Decision",
				Currency:    "USD",
				Impact:      market.ImpactHigh,
				ScheduledAt: time.Now().Add(45 * time.Minute),
			},
			market.MacroEvent{
				ID:          "US-CPI-YOY",
				Title:       "US CPI Inflation Rate (YoY)",
				Currency:    "USD",
				Impact:      market.ImpactHigh,
				ScheduledAt: time.Now().Add(4 * time.Hour),
			},
		)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"events": cal.GetEvents(),
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
			http.Error(w, `{"error":"historical candle downloader uninitialized"}`, http.StatusInternalServerError)
			return
		}

		histCandles, err := s.candleDownloader.FetchHistoricalKlines(r.Context(), req.Symbol, "1h", req.BarsCount)
		if err != nil || len(histCandles) == 0 {
			errMsg := "unable to fetch live historical exchange klines for backtest"
			if err != nil {
				errMsg += ": " + err.Error()
			}
			http.Error(w, `{"error":"`+errMsg+`"}`, http.StatusBadGateway)
			return
		}

		candles := make([]backtest.Candle, len(histCandles))
		for i, hc := range histCandles {
			candles[i] = backtest.Candle{
				Timestamp: hc.OpenTime,
				Open:      hc.Open,
				High:      hc.High,
				Low:       hc.Low,
				Close:     hc.Close,
				Volume:    hc.Volume,
			}
		}
		cfg := backtest.BacktestConfig{
			Symbol:            req.Symbol,
			InitialCapital:    req.InitialCapital,
			Friction:          trader.DefaultFrictionModel(),
			Kelly:             trader.DefaultKellyConfig(),
			IndicatorsWeights: map[string]float64{"RSI": 1.0, "MACD": 1.0, "SUPERTREND": 1.2, "MICROSTRUCTURE": 1.5},
			RiskFreeRate:      0.04,
		}
		engine := backtest.NewVectorizedEngine(cfg)
		btResult := engine.Run(candles)
		mcResult := backtest.RunMonteCarlo(btResult.Trades, req.InitialCapital, 1000, 42)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"backtest":    btResult,
			"monte_carlo": mcResult,
		})
	})

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
			sentiment = s.newsCrawler.GetAggregateSentiment()
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

	// Dynamic Macroeconomic Regime & 3-Tier Allocation (US2, FR-004)
	r.Get("/api/v1/macro/regime", s.GetMacroRegimeHandler)
	r.Post("/api/v1/macro/regime", s.UpdateMacroRegimeHandler)

	// Telegram Signals Bot Integration (US3, FR-007)
	r.Get("/api/v1/telegram/config", s.GetTelegramConfigHandler)
	r.Post("/api/v1/telegram/config", s.UpdateTelegramConfigHandler)
	r.Post("/api/v1/telegram/test", s.TestTelegramHandler)

	// Administrative Authentication & Session Security (US4)
	r.Post("/api/v1/auth/login", s.LoginHandler)
	r.Post("/api/v1/auth/logout", s.LogoutHandler)
	r.Get("/api/v1/auth/session", s.SessionHandler)

	// Real-Data Machine Learning Training & Model Telemetry (US6)
	r.Post("/api/v1/ml/train", s.TrainMLHandler)
	r.Get("/api/v1/ml/status", s.GetMLStatusHandler)
	r.Get("/api/v1/ml/runs", s.ListMLRunsHandler)

	// 8. Serve static frontend PWA assets if built (allows direct access or reverse-proxy from host Nginx)
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
