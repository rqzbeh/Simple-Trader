package market

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/db"
)

// ScreenerConfig sets minimum liquidity and tightness thresholds.
type ScreenerConfig struct {
	Min24hVolume   float64       // e.g. 50,000,000 ($50M USD)
	MaxSpreadBps   float64       // e.g. 10.0 (10 basis points = 0.10%)
	PollInterval   time.Duration // e.g. 5 minutes
	CandidatePairs []string      // Pairs to evaluate
}

// DefaultScreenerConfig provides institutional liquidity parameters.
// CandidatePairs is derived dynamically from the unified asset catalog.
func DefaultScreenerConfig() ScreenerConfig {
	assets := GetSupportedAssets()
	pairs := make([]string, len(assets))
	for i, a := range assets {
		pairs[i] = a.Symbol
	}
	return ScreenerConfig{
		Min24hVolume:   50000000.0,
		MaxSpreadBps:   10.0,
		PollInterval:   5 * time.Minute,
		CandidatePairs: pairs,
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
		activeUniverse: make([]string, 0),
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

	if price <= 0 {
		asset.Status = "DISQUALIFIED"
		asset.RejectionReason = "invalid or non-positive live market price"
		return asset
	}

	// For ALPHA crypto assets, enforce institutional $50M volume and 10 bps spread to prevent illiquidity slippage.
	// For CORE commodities (metals, energy), liquidity is guaranteed by institutional futures / physical vaults.
	if GetBucket(symbol) == "ALPHA" {
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
	}

	return asset
}

// RunScreeningCycle performs one complete pass over all candidate pairs.
// Market stats are fetched concurrently: the catalog now holds 100+ instruments
// and a sequential pass would exceed the poll interval.
func (s *DynamicCryptoScreener) RunScreeningCycle(ctx context.Context) []db.ScreenedAsset {
	s.mu.Lock()
	defer s.mu.Unlock()

	pairs := s.cfg.CandidatePairs
	type statsResult struct {
		price, vol, spread float64
		err                error
	}

	results := make([]statsResult, len(pairs))
	if s.provider == nil {
		for i := range pairs {
			results[i] = statsResult{err: errors.New("market stats provider uninitialized")}
		}
	} else {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 12)
		for i, symbol := range pairs {
			wg.Add(1)
			go func(idx int, sym string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				p, v, sp, err := s.provider.Get24hStats(sym)

				// CORE commodities on Yahoo feeds are not on Binance: fall back
				// to the live price the feed already cached in Redis.
				if err != nil && GetBucket(sym) == "CORE" {
					assetDef, _ := FindAsset(sym)
					if assetDef.FeedSource == "YAHOO" && s.redisClient != nil {
						if cached, cErr := s.redisClient.GetTicker(ctx, sym); cErr == nil && cached != nil && cached.Price > 0 {
							p, v, sp, err = cached.Price, cached.Volume, 0, nil
						}
					}
				}
				results[idx] = statsResult{price: p, vol: v, spread: sp, err: err}
			}(i, symbol)
		}
		wg.Wait()
	}

	evaluated := make([]db.ScreenedAsset, 0, len(pairs))
	qualified := make([]string, 0, len(pairs))

	for i, symbol := range pairs {
		r := results[i]
		var asset db.ScreenedAsset

		if r.err != nil {
			log.Printf("[Screener] Live fetch failed for %s (%v)", symbol, r.err)
			asset = db.ScreenedAsset{
				Symbol:          symbol,
				Price:           0,
				Volume24h:       0,
				BidAskSpreadBps: 0,
				Status:          "DISQUALIFIED",
				RejectionReason: "Live exchange fetch failed: " + r.err.Error(),
				ScreenedAt:      time.Now(),
			}
		} else {
			asset = s.EvaluateCandidate(symbol, r.price, r.vol, r.spread)
		}

		evaluated = append(evaluated, asset)
		if asset.Status == "ACTIVE" {
			qualified = append(qualified, symbol)
		}

		if s.dbStore != nil && s.dbStore.Pool != nil {
			_, _ = s.dbStore.Pool.Exec(ctx, `
				INSERT INTO crypto_screener_snapshots (symbol, price, volume_24h, bid_ask_spread_bps, status, rejection_reason, screened_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
			`, asset.Symbol, asset.Price, asset.Volume24h, asset.BidAskSpreadBps, asset.Status, asset.RejectionReason, asset.ScreenedAt)
		}
	}

	s.screenedAssets = evaluated
	// Always publish the current pass: leaving a stale universe on an all-reject
	// pass would let the UI claim assets are qualified after they are not.
	s.activeUniverse = qualified
	if s.redisClient != nil {
		_ = s.redisClient.SetActiveCryptoUniverse(ctx, qualified)
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
