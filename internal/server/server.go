package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/auth"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/indicators"
	"github.com/rqzbeh/simple-trader/internal/market"
	"github.com/rqzbeh/simple-trader/internal/telegram"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

// Server encapsulates HTTP routes, middlewares, and services.
type Server struct {
	cfg               *config.Config
	dbStore           *db.Store
	redisClient       *cache.Client
	aiClient          *ai.Client
	allocator         *trader.Allocator
	execEngine        *trader.ExecutionEngine
	newsCrawler       *market.NewsCrawler
	catalystClusterer *market.Clusterer // spec 012 US3: syndicated-headline clustering (FR-008)
	screener          *market.DynamicCryptoScreener
	telegramBot       *telegram.BotClient
	authenticator     *auth.Authenticator
	broadcaster       *SSEBroadcaster
	sampler           *ai.ThompsonSampler
	signalSlotMu      sync.Mutex // serialises the MAX_CONCURRENT_SIGNALS check
	router            *chi.Mux
	marketData        *trader.LiveMarketData
	candleDownloader  market.HistoricalKlineProvider
	calendar          *market.EconomicCalendar
	newsClassifier    market.NewsClassifier // spec-013: core headline classifier (explicit-error)
	decisionRouter    *trader.DecisionRouter
	shadow            *trader.ShadowOrchestrator
	earlyExitManager  *trader.EarlyExitManager
	startTime         time.Time
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
	coreClassify := coreNewsClassifier(aiClient)
	crawler.SetClassifier(coreClassify)

	// Decision core wiring (spec-013 T040/T045): threshold parsed at boot —
	// missing/invalid is a startup error (FR-016), no silent default.
	var decisionRouter *trader.DecisionRouter
	if os.Getenv("JEV_DISABLE") == "" {
		thr, terr := cfg.RoutingThreshold()
		if terr != nil {
			// spec-017 FR-402: no continue-with-degraded-core — boot aborts.
			log.Fatalf("[FATAL] decision core not configured: %v", terr)
		} else {
			decisionRouter = &trader.DecisionRouter{
				Jev:       ai.NewJevClient(os.Getenv("JEV_BASE_URL"), os.Getenv("TYPESAFE_API_KEY"), 12*time.Second),
				Threshold: thr,
				Escalate: func(ctx context.Context, payload interface{}) (trader.DecisionOutcome, error) {
					// 9Router escalation: slow brain answers when Jev low-confidence (FR-003).
					// Payload MUST be the real entry request (or a StateObject converted
					// from it) — an empty dummy body made the LLM answer HOLD for every
					// escalated candidate (2026-09-29 defect). EscalationPayload rejects
					// anything else instead of judging an empty context.
					req, perr := trader.EscalationPayload(payload)
					if perr != nil {
						return trader.DecisionOutcome{}, perr
					}
					resp, err := aiClient.Analyze(ctx, req)
					if err != nil {
						return trader.DecisionOutcome{}, fmt.Errorf("escalation failed: %w", err)
					}
					out := trader.DecisionOutcome{Confidence: resp.Confidence}
					switch resp.Decision {
					case "BUY":
						out.Choice = "LONG"
					case "SELL":
						out.Choice = "SHORT"
					default:
						out.Choice = "NO_TRADE"
					}
					return out, nil
				},
			}
		}
	}
	shadow := &trader.ShadowOrchestrator{Router: decisionRouter, Store: dbStore}
	shadow.Start(2, 64)
	shadow.SetEnabled("entry", os.Getenv("SHADOW_ENTRY") != "false")
	shadow.SetEnabled("exit", os.Getenv("SHADOW_EXIT") != "false")
	shadow.SetEnabled("news", os.Getenv("SHADOW_NEWS") != "false")
	binanceFetcher := market.NewBinanceFetcher()
	screenerCfg := market.DefaultScreenerConfig()
	// spec-017: liquidity thresholds come from required env, not code samples.
	screenerCfg.Min24hVolume = cfg.ScreenerMin24hVolume
	screenerCfg.MaxSpreadBps = cfg.ScreenerMaxSpreadBps
	screener := market.NewDynamicCryptoScreener(screenerCfg, binanceFetcher, redisClient, dbStore)

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
	marketData := trader.NewLiveMarketData()
	candleDownloader := market.NewBinanceHistoricalDownloader()
	if execEngine != nil {
		execEngine.SetPriceProvider(marketData)
	}

	// spec-017: value comes from required env CALENDAR_HALT_MINUTES (0 = disabled).
	var haltWindow time.Duration
	calURL := ""
	if cfg != nil {
		haltWindow = time.Duration(cfg.CalendarHaltMinutes) * time.Minute
		calURL = cfg.EconomicCalendarURL
	}
	calendar := market.NewEconomicCalendar(haltWindow)

	// Asynchronously fetch authentic macroeconomic releases from the institutional calendar feed
	go func() {
		calCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = calendar.RefreshFromLiveFeed(calCtx, calURL)
		cancel()

		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			refreshCtx, refreshCancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = calendar.RefreshFromLiveFeed(refreshCtx, calURL)
			refreshCancel()
		}
	}()

	s := &Server{
		startTime:         time.Now(),
		cfg:               cfg,
		dbStore:           dbStore,
		redisClient:       redisClient,
		aiClient:          aiClient,
		allocator:         allocator,
		execEngine:        execEngine,
		newsCrawler:       crawler,
		newsClassifier:    coreClassify,
		decisionRouter:    decisionRouter,
		shadow:            shadow,
		catalystClusterer: market.NewClusterer(15 * time.Minute),
		screener:          screener,
		telegramBot:       tgBot,
		authenticator:     authenticator,
		sampler:           sampler,
		broadcaster:       NewSSEBroadcaster(),
		router:            chi.NewRouter(),
		marketData:        marketData,
		candleDownloader:  candleDownloader,
		calendar:          calendar,
	}

	// Register initial SSE hydration provider so newly connected dashboards receive all cached live asset prices instantly
	s.broadcaster.SetInitialPayloadProvider(func() []string {
		var payloads []string
		if s.marketData != nil {
			for _, q := range s.marketData.GetAllQuotes() {
				if q.Price > 0 {
					if data, err := json.Marshal(q); err == nil {
						payloads = append(payloads, fmt.Sprintf("event: tick\ndata: %s\n\n", string(data)))
					}
				}
			}
		}
		return payloads
	})

	if redisClient != nil {
		s.broadcaster.AttachRedis(context.Background(), redisClient, "events:stream")
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

// CalculateAndCacheIndicatorSnapshot downloads authentic historical candles,
// computes the full institutional indicator suite (RSI, MACD, SuperTrend, Bollinger, ATR,
// Garman-Klass, Parkinson, Kaufman ER, CMF, Regime, Confluence), and caches the snapshot.
func (s *Server) CalculateAndCacheIndicatorSnapshot(ctx context.Context, symbol string) (*cache.IndicatorSnapshot, error) {
	if s.candleDownloader == nil {
		return nil, fmt.Errorf("historical candle downloader uninitialized")
	}

	candles, err := s.candleDownloader.FetchHistoricalKlines(ctx, symbol, "1h", 60)
	if err != nil || len(candles) == 0 {
		return nil, fmt.Errorf("failed to fetch authentic candles for %s: %w", symbol, err)
	}

	dbCandles := make([]db.Candle, len(candles))
	for i, c := range candles {
		dbCandles[i] = db.Candle{
			Symbol:    symbol,
			Timeframe: "1h",
			OpenTime:  c.OpenTime,
			Open:      c.Open,
			High:      c.High,
			Low:       c.Low,
			Close:     c.Close,
			Volume:    c.Volume,
		}
	}

	var weights map[string]float64
	if s.sampler != nil {
		weights = s.sampler.GetWeights()
	}

	snap := indicators.BuildSnapshot(symbol, dbCandles, weights)
	cacheSnap := &cache.IndicatorSnapshot{
		Symbol:          snap.Symbol,
		RSI:             snap.RSI,
		MACD:            snap.MACD,
		Signal:          snap.MACDSignal,
		Histogram:       snap.MACDHistogram,
		UpperBand:       snap.UpperBand,
		MiddleBand:      snap.MiddleBand,
		LowerBand:       snap.LowerBand,
		SuperTrend:      snap.SuperTrendTrend,
		ConfluenceScore: snap.ConfluenceScore,
		Regime:          string(snap.Regime),
		OBI:             snap.OBI,
		CVD:             snap.CVD,
		Divergence:      string(snap.Divergence),
		VolRatio:        snap.VolRatio,
		GarmanKlass:     snap.GarmanKlass,
		Parkinson:       snap.Parkinson,
		KaufmanER:       snap.KaufmanER,
		CMF:             snap.CMF,
		NATR:            snap.NATR,
		UpdatedAt:       time.Now().Unix(),
	}

	if s.redisClient != nil {
		_ = s.redisClient.SetIndicatorSnapshot(ctx, symbol, cacheSnap, 5*time.Minute)
	}

	return cacheSnap, nil
}

// GetIndicatorSnapshot retrieves the cached indicator snapshot or dynamically computes it.
func (s *Server) GetIndicatorSnapshot(ctx context.Context, symbol string) (*cache.IndicatorSnapshot, error) {
	if s.redisClient != nil {
		if cached, err := s.redisClient.GetIndicatorSnapshot(ctx, symbol); err == nil && cached != nil {
			return cached, nil
		}
	}
	return s.CalculateAndCacheIndicatorSnapshot(ctx, symbol)
}

// IngestTick updates the in-memory quote store, Redis (if active), broadcasts the tick to SSE,
// and checks active futures signals for autonomous Take Profit / Stop Loss resolution.
func (s *Server) IngestTick(tick cache.TickerQuote) {
	if s.marketData != nil {
		s.marketData.UpdateQuote(tick)
	}
	if s.redisClient != nil {
		tickCtx, tickCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = s.redisClient.SetTicker(tickCtx, tick.Symbol, &tick, 2*time.Minute)
		tickCancel()
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

// Calendar returns the active economic calendar manager.
func (s *Server) Calendar() *market.EconomicCalendar {
	return s.calendar
}

// StartEarlyExitWorker starts the news-driven early exit evaluation loop (spec-014).
func (s *Server) StartEarlyExitWorker(ctx context.Context, interval time.Duration) {
	if s.decisionRouter == nil || s.decisionRouter.Jev == nil || s.execEngine == nil || s.catalystClusterer == nil {
		return
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	eeCfg := config.EarlyExitConfig{
		Enabled:     true,
		MinHoldMin:  30,
		MaxPerDay:   3,
		CooldownMin: 60,
		ConfFloor:   0.75,
	}
	if s.cfg != nil {
		eeCfg = s.cfg.EarlyExit
	}

	mgr := trader.NewEarlyExitManager(
		eeCfg,
		s.execEngine,
		s.dbStore,
		s.telegramBot,
		s.decisionRouter.Jev,
		s.catalystClusterer,
		func(event string, data interface{}) {
			if s.broadcaster != nil {
				if b, err := json.Marshal(data); err == nil {
					s.broadcaster.Broadcast(event, string(b))
				}
			}
		},
	)
	s.earlyExitManager = mgr
	mgr.Start(ctx, interval)
}

// coreNewsClassifier binds headline classification to the decision core
// (9Router structured output paired with Jev shadow). Explicit errors only.
func coreNewsClassifier(c *ai.Client) market.NewsClassifier {
	return func(headlines []string) (market.NewsSentimentReport, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := c.ClassifyNews(ctx, "", headlines)
		if err != nil {
			return market.NewsSentimentReport{}, err
		}
		score := 0.0
		switch res.Label {
		case "BULLISH":
			score = res.Confidence
		case "BEARISH":
			score = -res.Confidence
		}
		return market.NewsSentimentReport{
			Score:         score,
			Polarity:      market.SentimentPolarity(res.Label),
			HeadlineCount: len(headlines),
			KeyPhrases:    res.Evidence,
		}, nil
	}
}
