package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/market"
	"github.com/rqzbeh/simple-trader/internal/server"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

func main() {
	log.Println("==========================================================")
	log.Println("Simple-Trader v2.0 • Autonomous Go & AI Engine Starting...")
	log.Println("==========================================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Load Configuration
	cfg, err := config.Load()
	if err != nil {
		log.Printf("[WARN] Failed to load full env config, using defaults: %v", err)
	}

	// 2. Initialize Database Store (optional fallback for non-DB container boot)
	var store *db.Store
	if cfg != nil && cfg.DatabaseURL != "" {
		dbStore, err := db.NewStore(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Printf("[WARN] PostgreSQL not reachable at %s: %v. Running in-memory mode.", cfg.DatabaseURL, err)
		} else {
			defer dbStore.Close()
			store = dbStore
			log.Printf("[INFO] Connected to PostgreSQL 16 Store successfully.")

			// Run migrations idempotently
			for _, migPath := range []string{"migrations", "/app/migrations", "internal/db/migrations"} {
				if _, err := os.Stat(migPath); err == nil {
					if err := store.RunMigrations(ctx, migPath); err != nil {
						log.Printf("[WARN] Error executing migrations from %s: %v", migPath, err)
					} else {
						log.Printf("[INFO] Migrations successfully verified from %s.", migPath)
						break
					}
				}
			}
		}
	}

	// 3. Initialize Redis In-Memory Cache & Pub/Sub
	var rCache *cache.Client
	if cfg != nil && cfg.RedisURL != "" {
		rClient, err := cache.NewClient(ctx, cfg.RedisURL)
		if err != nil {
			log.Printf("[WARN] Redis not reachable at %s: %v. Running without distributed cache.", cfg.RedisURL, err)
		} else {
			defer rClient.Close()
			rCache = rClient
			log.Printf("[INFO] Connected to Redis 7 Pub/Sub successfully.")
		}
	}

	// 4. Initialize AI Engine & Client
	aiCfg := ai.ClientConfig{
		BaseURL:         cfg.AIBaseURL,
		ModelID:         cfg.AIModelID,
		APIKey:          cfg.AIAPIKey,
		Temperature:     cfg.AITemperature,
		TimeoutSec:      cfg.AITimeoutSeconds,
		ReasoningEffort: cfg.AIReasoningEffort,
	}
	aiClient := ai.NewClient(aiCfg)
	_ = ai.NewWeightEngine()

	// 5. Initialize Trading & Risk Allocator & Execution Engine
	initialCap := 10000.0
	if cfg != nil && cfg.InitialCapital > 0 {
		initialCap = cfg.InitialCapital
	}
	if store != nil {
		// Ground initial capital strictly in the PostgreSQL investor ledger
		_, _, netCap, activeInvestors, err := store.GetTotalInvestorCapital(ctx)
		if err == nil {
			if activeInvestors == 0 {
				_, _ = store.EnsureDefaultInvestorProfile(ctx, "General Partner / Treasury", "Genesis Capital Allocation & Liquidity Seed", initialCap)
				_, _, netCap, _, _ = store.GetTotalInvestorCapital(ctx)
			}
			if netCap > 0 {
				initialCap = netCap
				log.Printf("[INFO] Master portfolio equity grounded in PostgreSQL investor ledger: $%.2f", initialCap)
			}
		}
	}

	allocatorConfig := trader.AllocatorConfig{
		TotalCapital:       initialCap,
		CoreTargetPct:      cfg.CoreTargetPct,
		AlphaTargetPct:     cfg.AlphaTargetPct,
		MaxRiskPerTradePct: cfg.MaxRiskPerTradePct,
	}
	allocator := trader.NewAllocator(allocatorConfig)
	execEngine := trader.NewExecutionEngine(initialCap)

	// 6. Initialize HTTP & SSE Broadcaster Server
	srv := server.NewServer(cfg, store, rCache, aiClient, allocator, execEngine)

	// 6b. Start Autonomous News Crawler, Dynamic Crypto Screener, and Transparent Background Signal Scanner
	if srv.NewsCrawler() != nil {
		srv.NewsCrawler().Start(ctx)
		log.Println("[INFO] Autonomous News Crawler started (polling financial & crypto RSS feeds).")
	}
	if srv.Screener() != nil {
		srv.Screener().Start(ctx)
		log.Println("[INFO] Dynamic Liquid Crypto Screener started (evaluating $50M volume / 10bps spread).")
	}
	// Transparent background scanning across all assets every 2 minutes
	srv.StartBackgroundSignalScanner(ctx, 2*time.Minute)
	log.Println("[INFO] Transparent Background Signal Scanner started (evaluating news catalysts across full universe).")

	// 7. Start Market Live Ticker Feed from Online Exchange APIs (Binance)
	liveFeed := market.NewLiveMarketFeed(market.GetSupportedAssets())
	ticks := liveFeed.Subscribe(ctx, 1*time.Second)
	log.Println("[INFO] Real-time live exchange market feed active. Ingesting online fluctuating ticks...")

	// 7b. Initialize Circuit Breaker & Autonomous Trading Daemon
	circuit := trader.NewCircuitBreaker(cfg.InitialCapital, cfg.MaxDrawdownLimitPct)
	strategyEvaluator := trader.NewAIStrategyEvaluator(aiClient, srv.NewsCrawler())
	daemonCfg := trader.DaemonConfig{
		TickInterval: 2 * time.Second,
		Symbols:      []string{"BTC/USDT", "ETH/USDT", "SOL/USDT", "PAXG/USDT", "BNB/USDT", "XRP/USDT", "LINK/USDT", "EUR/USDT"},
	}
	daemon := trader.NewTradingDaemon(daemonCfg, execEngine, allocator, circuit, srv.MarketData(), strategyEvaluator)

	// Ingest live online ticks into server, cache, SSE, and process daemon cycles
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tick, ok := <-ticks:
				if !ok {
					return
				}
				// Ingest tick into thread-safe quote store, redis, and SSE stream
				srv.IngestTick(tick)

				// Run continuous autonomous daemon evaluation
				daemon.ProcessTick(ctx)
			}
		}
	}()

	// 8. Start HTTP Server in background
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      srv.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		log.Printf("[INFO] Simple-Trader API & SSE Server listening on :%s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] HTTP server error: %v", err)
		}
	}()

	// 9. Graceful Shutdown listener
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("[INFO] Shutting down Simple-Trader gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] HTTP server shutdown error: %v", err)
	}

	log.Println("[INFO] Simple-Trader stopped.")
}
