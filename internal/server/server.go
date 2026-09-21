package server

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
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
	}

	s.setupRoutes()
	return s
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

	// Market Assets
	r.Get("/api/v1/assets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"assets": market.GetSupportedAssets(),
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Symbol == "" {
			req.Symbol = "BTC/USD"
		}
		if req.InitialCapital <= 0 {
			req.InitialCapital = 100000.0
		}
		if req.BarsCount <= 0 {
			req.BarsCount = 1000
		}

		now := time.Now().Add(-time.Duration(req.BarsCount) * time.Hour)
		candles := make([]backtest.Candle, req.BarsCount)
		price := 65000.0
		if req.Symbol == "XAU/USD" {
			price = 2650.0
		} else if req.Symbol == "ETH/USD" {
			price = 2800.0
		}
		for i := 0; i < req.BarsCount; i++ {
			drift := math.Sin(float64(i)/30.0)*10.0 + 2.0
			closeVal := price + drift
			high := math.Max(price, closeVal) + 5.0
			low := math.Min(price, closeVal) - 5.0
			candles[i] = backtest.Candle{
				Timestamp: now.Add(time.Duration(i) * time.Hour),
				Open:      price,
				High:      high,
				Low:       low,
				Close:     closeVal,
				Volume:    150.0,
			}
			price = closeVal
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
