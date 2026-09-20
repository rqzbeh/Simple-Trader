package market

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/db"
)

// IngestionManager coordinates polling and streaming market quotes into Redis and Postgres.
type IngestionManager struct {
	binanceFetcher *BinanceFetcher
	yahooFetcher   *YahooFinanceFetcher
	redisClient    *cache.Client
	dbStore        *db.Store
	deduplicator   *NewsDeduplicator
}

// NewIngestionManager creates an ingestion manager.
func NewIngestionManager(redisClient *cache.Client, dbStore *db.Store) *IngestionManager {
	return &IngestionManager{
		binanceFetcher: NewBinanceFetcher(),
		yahooFetcher:   NewYahooFinanceFetcher(),
		redisClient:    redisClient,
		dbStore:        dbStore,
		deduplicator:   NewNewsDeduplicator(),
	}
}

// UpdateAssetPrice pulls current quote data for an asset, updates Redis, and broadcasts a tick.
func (m *IngestionManager) UpdateAssetPrice(ctx context.Context, asset AssetDefinition) (*cache.TickerQuote, error) {
	var quote *cache.TickerQuote
	var err error

	if asset.FeedSource == "BINANCE" {
		quote, err = m.binanceFetcher.FetchTicker(ctx, asset.SourceParam)
	} else if asset.FeedSource == "YAHOO" {
		quote, err = m.yahooFetcher.FetchQuote(ctx, asset.SourceParam)
	} else {
		return nil, fmt.Errorf("unknown feed source: %s", asset.FeedSource)
	}

	if err != nil {
		return nil, fmt.Errorf("failed fetching %s: %w", asset.Symbol, err)
	}

	quote.Symbol = asset.Symbol

	// Update Redis cache
	if m.redisClient != nil {
		if err := m.redisClient.SetTicker(ctx, quote.Symbol, quote, 2*time.Minute); err != nil {
			log.Printf("[market] warning: redis set ticker failed for %s: %v", quote.Symbol, err)
		}

		if data, err := quote.Marshal(); err == nil {
			_ = m.redisClient.Publish(ctx, cache.ChannelMarketTicks, string(data))
		}
	}

	return quote, nil
}
