package main

import (
	"context"
	"encoding/json"
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
		BaseURL: cfg.AIBaseURL,
		ModelID: cfg.AIModelID,
		APIKey:  cfg.AIAPIKey,
	}
	aiClient := ai.NewClient(aiCfg)
	_ = ai.NewWeightEngine()

	// 5. Initialize Trading & Risk Allocator & Execution Engine
	allocatorConfig := trader.AllocatorConfig{
		TotalCapital:       cfg.InitialCapital,
		CoreTargetPct:      cfg.CoreTargetPct,
		AlphaTargetPct:     cfg.AlphaTargetPct,
		MaxRiskPerTradePct: cfg.MaxRiskPerTradePct,
	}
	allocator := trader.NewAllocator(allocatorConfig)
	execEngine := trader.NewExecutionEngine(cfg.InitialCapital)

	// 6. Initialize HTTP & SSE Broadcaster Server
	srv := server.NewServer(cfg, store, rCache, aiClient, allocator, execEngine)

	// 7. Start Market Simulated Ticker Feed Generator
	go func() {
		feed := market.NewSimulatedFeed()
		ticks := feed.Subscribe(ctx)
		log.Println("[INFO] Market ingestion & tick feed active. Broadcasting ticks to SSE...")
		for {
			select {
			case <-ctx.Done():
				return
			case tick, ok := <-ticks:
				if !ok {
					return
				}
				tickJSON, err := json.Marshal(tick)
				if err == nil {
					srv.Broadcaster().Broadcast("tick", string(tickJSON))
				}
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
