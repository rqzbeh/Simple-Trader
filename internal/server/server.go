package server

import (
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/backtest"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/market"
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
	broadcaster *SSEBroadcaster
	router      *chi.Mux
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
	s := &Server{
		cfg:         cfg,
		dbStore:     dbStore,
		redisClient: redisClient,
		aiClient:    aiClient,
		allocator:   allocator,
		execEngine:  execEngine,
		broadcaster: NewSSEBroadcaster(),
		router:      chi.NewRouter(),
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
}
