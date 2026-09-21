package trader

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
)

// OrderRequest contains order parameters for execution.
type OrderRequest struct {
	Symbol            string
	Bucket            string
	Side              string // "BUY" or "SELL"
	Price             float64
	PositionSize      float64
	StopLoss          float64
	TakeProfit        float64
	Leverage          int
	AIReasoning       string
	SymbolSpreadPct   float64
	AvailableDepthQty float64
	IsTaker           bool
}

// PriceProvider defines an abstraction for querying current online market prices.
type PriceProvider interface {
	GetLatestPrice(symbol string) (float64, error)
}

// ExecutionEngine simulates paper order execution and tracks open/closed positions in-memory.
type ExecutionEngine struct {
	mu            sync.RWMutex
	initialEquity float64
	cash          float64
	positions     map[string]*db.Trade
	closedTrades  []*db.Trade
	orderCounter  int64
	friction      FrictionModel
	priceProvider PriceProvider
}

// NewExecutionEngine initializes the paper execution engine with realistic friction.
func NewExecutionEngine(initialCapital float64) *ExecutionEngine {
	return &ExecutionEngine{
		initialEquity: initialCapital,
		cash:          initialCapital,
		positions:     make(map[string]*db.Trade),
		closedTrades:  make([]*db.Trade, 0),
		friction:      DefaultFrictionModel(),
	}
}

// SetPriceProvider attaches an online market price provider for dynamic valuations.
func (e *ExecutionEngine) SetPriceProvider(p PriceProvider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.priceProvider = p
}

// SetFrictionModel sets custom friction parameters.
func (e *ExecutionEngine) SetFrictionModel(f FrictionModel) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.friction = f
}

// ExecuteOrder opens a new paper trading position with realistic friction.
func (e *ExecutionEngine) ExecuteOrder(ctx context.Context, req OrderRequest) (*db.Trade, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.orderCounter++
	now := time.Now()

	executionSide := req.Side
	dir := DirectionLong
	if req.Side == "SELL" || req.Side == "SHORT" {
		executionSide = "SELL"
		dir = DirectionShort
	} else {
		executionSide = "BUY"
		dir = DirectionLong
	}

	quote := e.friction.CalculateExecution(
		executionSide,
		req.PositionSize,
		req.Price,
		req.SymbolSpreadPct,
		req.AvailableDepthQty,
		req.IsTaker,
	)

	lev := req.Leverage
	if lev < 1 {
		lev = 1
	}

	marginCost := (quote.EffectivePrice * req.PositionSize) / float64(lev)
	totalEntryCost := marginCost + quote.TotalFee

	if totalEntryCost > e.cash {
		return nil, fmt.Errorf("insufficient cash to execute order: need %.2f, have %.2f", totalEntryCost, e.cash)
	}

	liqPrice, _ := CalculateLiquidationPrice(quote.EffectivePrice, lev, dir, 0.005)

	trade := &db.Trade{
		ID:               e.orderCounter,
		Symbol:           req.Symbol,
		Bucket:           req.Bucket,
		Side:             executionSide,
		EntryPrice:       quote.EffectivePrice,
		EntryTime:        now,
		PositionSize:     req.PositionSize,
		StopLoss:         req.StopLoss,
		TakeProfit:       req.TakeProfit,
		ExecutionFee:     quote.TotalFee,
		SlippagePaid:     quote.Slippage,
		Leverage:         lev,
		LiquidationPrice: liqPrice,
		Status:           "OPEN",
		CreatedAt:        now,
	}

	e.cash -= totalEntryCost
	e.positions[req.Symbol] = trade

	return trade, nil
}

// CheckExit checks whether the latest market price triggers a Stop Loss or Take Profit with realistic friction.
func (e *ExecutionEngine) CheckExit(symbol string, currentPrice float64) (*db.Trade, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	trade, exists := e.positions[symbol]
	if !exists || trade.Status != "OPEN" {
		return nil, false
	}

	shouldExit := false
	exitReason := ""

	if trade.Side == "BUY" || trade.Side == "LONG" {
		if trade.TakeProfit > 0 && currentPrice >= trade.TakeProfit {
			shouldExit = true
			exitReason = "TAKE_PROFIT"
		} else if trade.StopLoss > 0 && currentPrice <= trade.StopLoss {
			shouldExit = true
			exitReason = "STOP_LOSS"
		} else if trade.LiquidationPrice > 0 && currentPrice <= trade.LiquidationPrice {
			shouldExit = true
			exitReason = "LIQUIDATION"
		}
	} else if trade.Side == "SELL" || trade.Side == "SHORT" {
		if trade.TakeProfit > 0 && currentPrice <= trade.TakeProfit {
			shouldExit = true
			exitReason = "TAKE_PROFIT"
		} else if trade.StopLoss > 0 && currentPrice >= trade.StopLoss {
			shouldExit = true
			exitReason = "STOP_LOSS"
		} else if trade.LiquidationPrice > 0 && currentPrice >= trade.LiquidationPrice {
			shouldExit = true
			exitReason = "LIQUIDATION"
		}
	}

	if !shouldExit {
		return nil, false
	}

	exitSide := "SELL"
	if trade.Side == "SELL" || trade.Side == "SHORT" {
		exitSide = "BUY"
	}

	exitQuote := e.friction.CalculateExecution(
		exitSide,
		trade.PositionSize,
		currentPrice,
		0.0002, // 2 bps default spread
		100.0,  // default depth
		true,   // exit via taker order
	)

	// Close position
	trade.ExitPrice = exitQuote.EffectivePrice
	trade.ExitReason = exitReason
	trade.Status = "CLOSED"
	trade.ExecutionFee += exitQuote.TotalFee
	trade.SlippagePaid += exitQuote.Slippage
	now := time.Now()
	trade.ExitTime = &now

	pnl, retPct := trade.CalculatePnL()
	trade.RealizedPnL = pnl
	trade.ReturnPct = retPct

	// Credit proceeds back: margin + realized net PnL
	lev := trade.Leverage
	if lev < 1 {
		lev = 1
	}
	margin := (trade.PositionSize * trade.EntryPrice) / float64(lev)
	proceeds := margin + pnl
	if proceeds < 0 {
		proceeds = 0
	}
	e.cash += proceeds

	delete(e.positions, symbol)
	e.closedTrades = append(e.closedTrades, trade)

	return trade, true
}

// GetTotalEquity returns cash plus unrealized marked-to-market position values.
// Accepts optional currentPrices map for mark-to-market valuation.
func (e *ExecutionEngine) GetTotalEquity(currentPrices ...map[string]float64) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var prices map[string]float64
	if len(currentPrices) > 0 {
		prices = currentPrices[0]
	}

	equity := e.cash
	for sym, pos := range e.positions {
		currPrice, ok := prices[sym]
		if !ok && e.priceProvider != nil {
			if liveP, err := e.priceProvider.GetLatestPrice(sym); err == nil && liveP > 0 {
				currPrice = liveP
				ok = true
			}
		}
		if !ok {
			currPrice = pos.EntryPrice
		}
		var diff float64
		if pos.Side == "BUY" || pos.Side == "LONG" {
			diff = currPrice - pos.EntryPrice
		} else {
			diff = pos.EntryPrice - currPrice
		}
		unrealized := diff * pos.PositionSize
		lev := pos.Leverage
		if lev < 1 {
			lev = 1
		}
		margin := (pos.PositionSize * pos.EntryPrice) / float64(lev)
		equity += margin + unrealized
	}

	return equity
}

// GetOpenTrades returns all currently open trading positions.
func (e *ExecutionEngine) GetOpenTrades() []*db.Trade {
	e.mu.RLock()
	defer e.mu.RUnlock()

	trades := make([]*db.Trade, 0, len(e.positions))
	for _, t := range e.positions {
		trades = append(trades, t)
	}
	return trades
}

// GetCash returns the currently available unencumbered cash.
func (e *ExecutionEngine) GetCash() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cash
}

// GetInitialEquity returns the initial starting equity of the engine.
func (e *ExecutionEngine) GetInitialEquity() float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.initialEquity
}

// SetTotalEquity adjusts the capital basis grounded in the investor ledger.
func (e *ExecutionEngine) SetTotalEquity(equity float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if equity > 0 {
		e.initialEquity = equity
		e.cash = equity
	}
}

// GetClosedTrades returns historical closed trades.
func (e *ExecutionEngine) GetClosedTrades() []*db.Trade {
	e.mu.RLock()
	defer e.mu.RUnlock()
	trades := make([]*db.Trade, len(e.closedTrades))
	copy(trades, e.closedTrades)
	return trades
}

