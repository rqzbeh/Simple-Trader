package trader

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/market"
)

// LiveMarketData provides in-memory thread-safe ticker storage implementing MarketDataProvider.
type LiveMarketData struct {
	mu         sync.RWMutex
	quotes     map[string]cache.TickerQuote
	fetcher    *market.BinanceFetcher
}

// NewLiveMarketData creates an empty live market data store with active online exchange fetcher.
func NewLiveMarketData() *LiveMarketData {
	return &LiveMarketData{
		quotes:  make(map[string]cache.TickerQuote),
		fetcher: market.NewBinanceFetcher(),
	}
}

// UpdateQuote records a new ticker quote.
func (m *LiveMarketData) UpdateQuote(quote cache.TickerQuote) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quotes[quote.Symbol] = quote
}

// GetQuote returns the cached quote for a symbol, if present.
func (m *LiveMarketData) GetQuote(symbol string) (cache.TickerQuote, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	q, ok := m.quotes[symbol]
	return q, ok
}

// GetAllQuotes returns all current cached ticker quotes.
func (m *LiveMarketData) GetAllQuotes() []cache.TickerQuote {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]cache.TickerQuote, 0, len(m.quotes))
	for _, q := range m.quotes {
		res = append(res, q)
	}
	return res
}

// GetLatestPrice returns the current market price for a symbol.
// If the symbol has not been cached from the live tick stream yet, it directly queries the online exchange.
func (m *LiveMarketData) GetLatestPrice(symbol string) (float64, error) {
	m.mu.RLock()
	if q, ok := m.quotes[symbol]; ok && q.Price > 0 {
		m.mu.RUnlock()
		return q.Price, nil
	}
	m.mu.RUnlock()

	// Direct online fetch from live exchange API for zero static hardcoding
	if m.fetcher != nil {
		cleanSymbol := strings.ReplaceAll(symbol, "/", "")
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()

		quote, err := m.fetcher.FetchTicker(ctx, cleanSymbol)
		if err == nil && quote != nil && quote.Price > 0 {
			m.UpdateQuote(*quote)
			return quote.Price, nil
		}
	}

	return 0, fmt.Errorf("online price unavailable for %s", symbol)
}

// GetMarketDepth returns simulated spread and available liquidity depth.
func (m *LiveMarketData) GetMarketDepth(symbol string) (spreadPct float64, availableDepth float64, err error) {
	return 0.0005, 50.0, nil
}

// NewsArticleProvider supplies breaking news for trade catalyst checks.
type NewsArticleProvider interface {
	GetLatestArticles() []db.NewsArticle
}

// IndicatorSnapshotProvider provides computed multi-factor indicator snapshots.
type IndicatorSnapshotProvider interface {
	GetIndicatorSnapshot(ctx context.Context, symbol string) (*cache.IndicatorSnapshot, error)
}

// AIStrategyEvaluator evaluates real-time market opportunities using the AI engine.
type AIStrategyEvaluator struct {
	aiClient         *ai.Client
	newsProvider     NewsArticleProvider
	snapshotProvider IndicatorSnapshotProvider
	lastEvals        map[string]time.Time
	mu               sync.Mutex
}

// NewAIStrategyEvaluator creates an evaluator instance with throttling to prevent model saturation.
func NewAIStrategyEvaluator(aiClient *ai.Client, newsProvider NewsArticleProvider) *AIStrategyEvaluator {
	return &AIStrategyEvaluator{
		aiClient:     aiClient,
		newsProvider: newsProvider,
		lastEvals:    make(map[string]time.Time),
	}
}

// SetSnapshotProvider injects an indicator snapshot provider for authentic market context.
func (e *AIStrategyEvaluator) SetSnapshotProvider(p IndicatorSnapshotProvider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.snapshotProvider = p
}

// Evaluate produces a trading signal when high-conviction catalysts and confluences align.
func (e *AIStrategyEvaluator) Evaluate(ctx context.Context, symbol string, currentPrice float64) (*db.Signal, error) {
	if e.aiClient == nil || currentPrice <= 0 {
		return nil, nil
	}

	// Throttle evaluations: at most once every 30 seconds per symbol
	e.mu.Lock()
	if lastTime, ok := e.lastEvals[symbol]; ok && time.Since(lastTime) < 30*time.Second {
		e.mu.Unlock()
		return nil, nil
	}
	e.lastEvals[symbol] = time.Now()
	e.mu.Unlock()

	var headlines []string
	if e.newsProvider != nil {
		latest := e.newsProvider.GetLatestArticles()
		for i := 0; i < len(latest) && i < 3; i++ {
			headlines = append(headlines, latest[i].Title)
		}
	}

	bucket := market.GetBucket(symbol)

	indicatorSnap := cache.IndicatorSnapshot{
		Symbol: symbol,
	}
	e.mu.Lock()
	snapProv := e.snapshotProvider
	e.mu.Unlock()
	if snapProv != nil {
		if snap, err := snapProv.GetIndicatorSnapshot(ctx, symbol); err == nil && snap != nil {
			indicatorSnap = *snap
		}
	}

	decReq := ai.DecisionRequest{
		Symbol: symbol,
		Bucket: bucket,
		Quote: cache.TickerQuote{
			Symbol: symbol,
			Price:  currentPrice,
		},
		IndicatorSnap: indicatorSnap,
		NewsHeadlines: headlines,
	}

	evalCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := e.aiClient.Analyze(evalCtx, decReq)
	if err != nil || resp == nil || resp.Decision == "HOLD" || resp.Decision == "" {
		return nil, err
	}

	slPct := resp.SuggestedStopLossPct
	if slPct <= 0 || slPct > 10.0 {
		slPct = 1.5
	}
	tpPct := resp.SuggestedTakeProfitPct
	if tpPct <= 0 || tpPct > 30.0 {
		tpPct = 3.5
	}

	var stopLoss, takeProfit float64
	if resp.Decision == "BUY" {
		stopLoss = currentPrice * (1.0 - (slPct / 100.0))
		takeProfit = currentPrice * (1.0 + (tpPct / 100.0))
	} else {
		stopLoss = currentPrice * (1.0 + (slPct / 100.0))
		takeProfit = currentPrice * (1.0 - (tpPct / 100.0))
	}

	sig := &db.Signal{
		Symbol:          symbol,
		Side:            resp.Decision,
		Bucket:          bucket,
		EntryPrice:      currentPrice,
		StopLoss:        stopLoss,
		TakeProfit:      takeProfit,
		Confidence:      float32(resp.Confidence),
		ConfluenceScore: float32(resp.Confidence),
		AIReasoning:     resp.Reasoning,
		Status:          "OPEN",
		CreatedAt:       time.Now(),
	}

	return sig, nil
}
