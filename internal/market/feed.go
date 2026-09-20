package market

import (
	"context"
	"math/rand"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
)

// SimulatedFeed generates synthetic market ticks for offline testing and development.
type SimulatedFeed struct {
	rnd *rand.Rand
}

// NewSimulatedFeed initializes the simulation feed.
func NewSimulatedFeed() *SimulatedFeed {
	return &SimulatedFeed{
		rnd: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Subscribe returns a channel that emits periodic simulated price movements.
func (s *SimulatedFeed) Subscribe(ctx context.Context) <-chan cache.TickerQuote {
	ch := make(chan cache.TickerQuote, 10)

	basePrices := map[string]float64{
		"BTC/USD": 68000.0,
		"ETH/USD": 3500.0,
		"XAU/USD": 2680.0,
		"XAG/USD": 31.50,
		"SOL/USD": 160.0,
	}

	go func() {
		defer close(ch)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		symbols := []string{"BTC/USD", "ETH/USD", "XAU/USD", "XAG/USD", "SOL/USD"}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sym := symbols[s.rnd.Intn(len(symbols))]
				base := basePrices[sym]
				// Random drift +/- 0.2%
				pct := (s.rnd.Float64() - 0.5) * 0.004
				newPrice := base * (1.0 + pct)
				basePrices[sym] = newPrice

				quote := cache.TickerQuote{
					Symbol:    sym,
					Price:     newPrice,
					Change24h: pct * 100.0,
					High24h:   newPrice * 1.01,
					Low24h:    newPrice * 0.99,
					Volume:    1000.0 + s.rnd.Float64()*500.0,
					UpdatedAt: time.Now().Unix(),
				}

				select {
				case ch <- quote:
				case <-ctx.Done():
					return
				default:
				}
			}
		}
	}()

	return ch
}
