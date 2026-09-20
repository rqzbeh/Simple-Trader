package market

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/db"
)

// ScreenerConfig sets minimum liquidity and tightness thresholds.
type ScreenerConfig struct {
	Min24hVolume    float64       // e.g. 50,000,000 ($50M USD)
	MaxSpreadBps    float64       // e.g. 10.0 (10 basis points = 0.10%)
	PollInterval    time.Duration // e.g. 5 minutes
	CandidatePairs  []string      // Pairs to evaluate
}

// DefaultScreenerConfig provides institutional liquidity parameters.
func DefaultScreenerConfig() ScreenerConfig {
	return ScreenerConfig{
		Min24hVolume: 50000000.0, // $50M 24h volume
		MaxSpreadBps: 10.0,        // 10 bps max spread
		PollInterval: 5 * time.Minute,
		CandidatePairs: []string{
			"BTC/USD", "ETH/USD", "SOL/USD",
			"BNB/USD", "XRP/USD", "ADA/USD",
			"DOGE/USD", "AVAX/USD", "LINK/USD",
			"DOT/USD", "NEAR/USD", "SUI/USD",
			"LTC/USD", "BCH/USD", "PEPE/USD",
			"SHIB/USD", "TRX/USD", "APT/USD",
			"UNI/USD", "ENA/USD",
		},
	}
}

// MarketStatsProvider provides 24h volume, price, and order book spread metrics.
type MarketStatsProvider interface {
	Get24hStats(symbol string) (price float64, volume24h float64, spreadBps float64, err error)
}

// DynamicCryptoScreener filters candidates dynamically to avoid illiquidity slippage.
type DynamicCryptoScreener struct {
	mu             sync.RWMutex
	cfg            ScreenerConfig
	provider       MarketStatsProvider
	redisClient    *cache.Client
	dbStore        *db.Store
	screenedAssets []db.ScreenedAsset
	activeUniverse []string
	running        bool
	stopChan       chan struct{}
}

// NewDynamicCryptoScreener instantiates the automated liquidity screener.
func NewDynamicCryptoScreener(
	cfg ScreenerConfig,
	provider MarketStatsProvider,
	redisClient *cache.Client,
	dbStore *db.Store,
) *DynamicCryptoScreener {
	if cfg.Min24hVolume <= 0 {
		cfg.Min24hVolume = 50000000.0
	}
	if cfg.MaxSpreadBps <= 0 {
		cfg.MaxSpreadBps = 10.0
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Minute
	}
	if len(cfg.CandidatePairs) == 0 {
		cfg.CandidatePairs = DefaultScreenerConfig().CandidatePairs
	}

	return &DynamicCryptoScreener{
		cfg:            cfg,
		provider:       provider,
		redisClient:    redisClient,
		dbStore:        dbStore,
		screenedAssets: make([]db.ScreenedAsset, 0, len(cfg.CandidatePairs)),
		activeUniverse: []string{"BTC/USD", "ETH/USD", "SOL/USD"},
		stopChan:       make(chan struct{}),
	}
}

// EvaluateCandidate evaluates a single candidate pair against liquidity and spread rules.
func (s *DynamicCryptoScreener) EvaluateCandidate(symbol string, price, volume24h, spreadBps float64) db.ScreenedAsset {
	asset := db.ScreenedAsset{
		Symbol:          symbol,
		Price:           price,
		Volume24h:       volume24h,
		BidAskSpreadBps: spreadBps,
		Status:          "ACTIVE",
		ScreenedAt:      time.Now(),
	}

	if volume24h < s.cfg.Min24hVolume {
		asset.Status = "DISQUALIFIED"
		asset.RejectionReason = "volume below $50M threshold"
		return asset
	}

	if spreadBps > s.cfg.MaxSpreadBps {
		asset.Status = "DISQUALIFIED"
		asset.RejectionReason = "spread exceeds 10 bps limit"
		return asset
	}

	return asset
}

// RunScreeningCycle performs one complete pass over all candidate pairs.
func (s *DynamicCryptoScreener) RunScreeningCycle(ctx context.Context) []db.ScreenedAsset {
	s.mu.Lock()
	defer s.mu.Unlock()

	var evaluated []db.ScreenedAsset
	var qualified []string

	for _, symbol := range s.cfg.CandidatePairs {
		var price, vol, spread float64
		if s.provider != nil {
			p, v, sp, err := s.provider.Get24hStats(symbol)
			if err != nil {
				log.Printf("[Screener] Live fetch failed for %s (%v), using default baseline", symbol, err)
				p = 100.0
				v = 60000000.0
				sp = 4.5
			}
			price, vol, spread = p, v, sp
		} else {
			// Mock fallback for testing or bootstrap
			price = 100.0
			vol = 60000000.0
			spread = 4.5
		}

		asset := s.EvaluateCandidate(symbol, price, vol, spread)
		evaluated = append(evaluated, asset)

		if asset.Status == "ACTIVE" {
			qualified = append(qualified, symbol)
		}

		// Persist to Postgres if available
		if s.dbStore != nil && s.dbStore.Pool != nil {
			_, _ = s.dbStore.Pool.Exec(ctx, `
				INSERT INTO crypto_screener_snapshots (symbol, price, volume_24h, bid_ask_spread_bps, status, rejection_reason, screened_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
			`, asset.Symbol, asset.Price, asset.Volume24h, asset.BidAskSpreadBps, asset.Status, asset.RejectionReason, asset.ScreenedAt)
		}
	}

	s.screenedAssets = evaluated
	if len(qualified) > 0 {
		s.activeUniverse = qualified
		if s.redisClient != nil {
			_ = s.redisClient.SetActiveCryptoUniverse(ctx, qualified)
		}
	}

	return evaluated
}

// GetScreenedAssets returns the latest evaluated candidate results.
func (s *DynamicCryptoScreener) GetScreenedAssets() []db.ScreenedAsset {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]db.ScreenedAsset, len(s.screenedAssets))
	copy(res, s.screenedAssets)
	return res
}

// GetActiveUniverse returns the symbols currently qualifying for live trading.
func (s *DynamicCryptoScreener) GetActiveUniverse() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]string, len(s.activeUniverse))
	copy(res, s.activeUniverse)
	return res
}

// Start begins the periodic screening loop.
func (s *DynamicCryptoScreener) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	go func() {
		// Run initial screening
		s.RunScreeningCycle(ctx)

		ticker := time.NewTicker(s.cfg.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.Stop()
				return
			case <-s.stopChan:
				return
			case <-ticker.C:
				res := s.RunScreeningCycle(ctx)
				log.Printf("[Screener] Cycle completed: %d assets evaluated", len(res))
			}
		}
	}()
}

// Stop terminates the screening loop.
func (s *DynamicCryptoScreener) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	close(s.stopChan)
}
