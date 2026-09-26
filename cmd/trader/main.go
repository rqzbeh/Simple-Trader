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

			// Run migrations idempotently, bounded in time: a wedged database
			// must degrade to serving traffic, never block startup forever.
			migCtx, migCancel := context.WithTimeout(ctx, 60*time.Second)
			for _, migPath := range []string{"migrations", "/app/migrations", "internal/db/migrations"} {
				if _, err := os.Stat(migPath); err == nil {
					if err := store.RunMigrations(migCtx, migPath); err != nil {
						log.Printf("[WARN] Error executing migrations from %s: %v", migPath, err)
					} else {
						log.Printf("[INFO] Migrations successfully verified from %s.", migPath)
						break
					}
				}
			}
			migCancel()

			// Validate + persist effective risk profiles (spec 012, FR-019).
			// Invalid config logs loudly but never blocks startup.
			if err := trader.ValidateRiskProfiles(); err != nil {
				log.Printf("[WARN] Risk profile validation failed: %v", err)
			} else if store != nil {
				for _, name := range []string{"CRYPTO", "COMMODITY"} {
					p := config.GetRiskProfile(name)
					windows := make([]db.BlackoutWindow, 0, len(p.BlackoutWindows))
					for _, w := range p.BlackoutWindows {
						windows = append(windows, db.BlackoutWindow{Name: w.Name, BeforeMin: w.BeforeMin, AfterMin: w.AfterMin})
					}
					row := db.RiskProfileRow{
						Name:                 p.Name,
						HorizonMin:           p.HorizonMin,
						HorizonMax:           p.HorizonMax,
						DecayBreakevenAtMin:  p.DecayBreakevenAtMin,
						DecayFlatAtMin:       p.DecayFlatAtMin,
						SLAtrMult:            p.SLAtrMult,
						SLSwingOffset:        p.SLSwingOffset,
						SLMinPct:             p.SLMinPct,
						SLMaxPct:             p.SLMaxPct,
						TP1AtrMult:           p.TP1AtrMult,
						TP2AtrMult:           p.TP2AtrMult,
						TP1CloseFrac:         p.TP1CloseFrac,
						TargetHourlyVolPct:   p.TargetHourlyVolPct,
						LiqBufferMin:         p.LiqBufferMin,
						RiskPerTradePct:      p.RiskPerTradePct,
						FreshnessHalfLifeMin: p.FreshnessHalfLifeMin,
						WeekendFlat:          p.WeekendFlat,
						BlackoutCalendar:     windows,
					}
					if err := store.UpsertRiskProfile(ctx, row); err != nil {
						log.Printf("[WARN] Failed to seed risk profile %s: %v", name, err)
					}
				}
				log.Printf("[INFO] Risk profiles seeded (CRYPTO, COMMODITY).")
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
		BaseURL:            cfg.AIBaseURL,
		ModelID:            cfg.AIModelID,
		APIKey:             cfg.AIAPIKey,
		Temperature:        cfg.AITemperature,
		TimeoutSec:         cfg.AITimeoutSeconds,
		ReasoningEffort:    cfg.AIReasoningEffort,
		DefaultLeverage:    cfg.DefaultLeverage,
		MinStopLossPct:     cfg.MinStopLossPct,
		MaxStopLossPct:     cfg.MaxStopLossPct,
		MinTakeProfitPct:   cfg.MinTakeProfitPct,
		MaxTakeProfitPct:   cfg.MaxTakeProfitPct,
		MinRiskRewardRatio: cfg.MinRiskRewardRatio,
	}
	aiClient := ai.NewClient(aiCfg)
	_ = ai.NewWeightEngine()

	// 5. Initialize Trading & Risk Allocator & Execution Engine
	initialCap := cfg.InitialCapital
	if initialCap <= 0 {
		initialCap = 10000.0
	}
	if store != nil {
		// Ground initial capital strictly in the PostgreSQL investor ledger.
		// Bounded so a stalled database degrades to the configured initial
		// capital instead of freezing startup.
		ledgerCtx, ledgerCancel := context.WithTimeout(ctx, 10*time.Second)
		_, _, netCap, activeInvestors, err := store.GetTotalInvestorCapital(ledgerCtx)
		ledgerCancel()
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
		Kelly: trader.NewKellyConfig(
			cfg.KellyFraction,
			cfg.MinRiskPerTradePct,
			cfg.MaxRiskPerTradePct,
			0.52,
			1.60,
		),
	}
	allocator := trader.NewAllocator(allocatorConfig)
	execEngine := trader.NewExecutionEngine(initialCap)
	if cfg != nil {
		impact := cfg.ImpactFactor
		if impact <= 0 {
			impact = 0.05
		}
		execEngine.SetFrictionModel(trader.NewFrictionModel(cfg.MakerFeeRate, cfg.TakerFeeRate, impact, cfg.MaxSlippagePct))
	}

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

	// Synchronous initial fetch to populate prices before serving clients
	if initialQuotes, err := liveFeed.FetchAllLiveTicks(ctx); err == nil {
		for _, q := range initialQuotes {
			srv.IngestTick(q)
		}
		log.Printf("[INFO] Initial price fetch complete: %d assets priced. Positions are signal-driven only.", len(initialQuotes))
	} else {
		log.Printf("[WARN] Initial price fetch failed: %v. Prices will be populated from live feed.", err)
	}

	// Reconcile active signals against live prices BEFORE rehydration: exits
	// missed while the backend was down (TP/SL crossings, expired signals) are
	// resolved here so rehydration only restores genuinely live positions.
	srv.ReconcileActiveSignals(ctx)
	srv.StartSignalReconciler(ctx, 1*time.Minute)
	log.Println("[INFO] Signal reconciler active (level crossings + time exits every 1m).")

	// Rehydrate existing active signals from database into execution engine positions
	if store != nil {
		activeSignals, err := store.ListFuturesSignals(ctx, "ACTIVE", 50)
		if err == nil && len(activeSignals) > 0 {
			rehydrated := 0
			for i := range activeSignals {
				sig := &activeSignals[i]
				if trade, err := execEngine.OpenPositionFromSignal(sig); err == nil && trade != nil {
					rehydrated++
				}
			}
			if rehydrated > 0 {
				log.Printf("[INFO] Rehydrated %d active signals into execution engine positions.", rehydrated)
			}
		}
	}

	ticks := liveFeed.Subscribe(ctx, 3*time.Second)
	log.Println("[INFO] Real-time live exchange market feed active. Ingesting online fluctuating ticks...")

	// 7b. Initialize Circuit Breaker & the position guardian.
	// The daemon monitors existing positions (SL/TP/liquidation exits) and
	// circuit-breaker equity only. Entries come exclusively from the news-gated
	// signal pipeline, so a position can never exist without a catalyst record.
	circuit := trader.NewCircuitBreaker(cfg.InitialCapital, cfg.MaxDrawdownLimitPct)
	allAssets := market.GetSupportedAssets()
	daemonSymbols := make([]string, len(allAssets))
	for i, a := range allAssets {
		daemonSymbols[i] = a.Symbol
	}
	daemonCfg := trader.DaemonConfig{
		TickInterval: 2 * time.Second,
		Symbols:      daemonSymbols,
	}
	daemon := trader.NewTradingDaemon(daemonCfg, execEngine, allocator, circuit, srv.MarketData())

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
	// WriteTimeout MUST be zero: the SSE stream (/api/v1/events) and batch
	// scans both write long after the request arrives. With WriteTimeout > 0
	// Go arms a socket write deadline when the headers are read, so any handler
	// still running past it silently loses its response — a 70s scan returned
	// an empty reply at exactly the handler's finish time. Handler-level
	// contexts bound each request instead.
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      srv.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
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
