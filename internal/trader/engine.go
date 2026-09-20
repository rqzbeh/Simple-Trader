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
	Symbol       string
	Bucket       string
	Side         string // "BUY" or "SELL"
	Price        float64
	PositionSize float64
	StopLoss     float64
	TakeProfit   float64
	AIReasoning  string
}

// ExecutionEngine simulates paper order execution and tracks open/closed positions in-memory.
type ExecutionEngine struct {
	mu            sync.RWMutex
	initialEquity float64
	cash          float64
	positions     map[string]*db.Trade
	closedTrades  []*db.Trade
	orderCounter  int64
}

// NewExecutionEngine initializes the paper execution engine.
func NewExecutionEngine(initialCapital float64) *ExecutionEngine {
	return &ExecutionEngine{
		initialEquity: initialCapital,
		cash:          initialCapital,
		positions:     make(map[string]*db.Trade),
		closedTrades:  make([]*db.Trade, 0),
	}
}

// ExecuteOrder opens a new paper trading position.
func (e *ExecutionEngine) ExecuteOrder(ctx context.Context, req OrderRequest) (*db.Trade, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.orderCounter++
	now := time.Now()

	cost := req.PositionSize * req.Price
	if cost > e.cash {
		return nil, fmt.Errorf("insufficient cash to execute order: need %.2f, have %.2f", cost, e.cash)
	}

	trade := &db.Trade{
		ID:           e.orderCounter,
		Symbol:       req.Symbol,
		Bucket:       req.Bucket,
		Side:         req.Side,
		EntryPrice:   req.Price,
		EntryTime:    now,
		PositionSize: req.PositionSize,
		StopLoss:     req.StopLoss,
		TakeProfit:   req.TakeProfit,
		Status:       "OPEN",
		CreatedAt:    now,
	}

	e.cash -= cost
	e.positions[req.Symbol] = trade

	return trade, nil
}

// CheckExit checks whether the latest market price triggers a Stop Loss or Take Profit.
func (e *ExecutionEngine) CheckExit(symbol string, currentPrice float64) (*db.Trade, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	trade, exists := e.positions[symbol]
	if !exists || trade.Status != "OPEN" {
		return nil, false
	}

	shouldExit := false
	exitReason := ""

	if trade.Side == "BUY" {
		if trade.TakeProfit > 0 && currentPrice >= trade.TakeProfit {
			shouldExit = true
			exitReason = "TAKE_PROFIT"
		} else if trade.StopLoss > 0 && currentPrice <= trade.StopLoss {
			shouldExit = true
			exitReason = "STOP_LOSS"
		}
	} else if trade.Side == "SELL" {
		if trade.TakeProfit > 0 && currentPrice <= trade.TakeProfit {
			shouldExit = true
			exitReason = "TAKE_PROFIT"
		} else if trade.StopLoss > 0 && currentPrice >= trade.StopLoss {
			shouldExit = true
			exitReason = "STOP_LOSS"
		}
	}

	if !shouldExit {
		return nil, false
	}

	// Close position
	trade.ExitPrice = currentPrice
	trade.ExitReason = exitReason
	trade.Status = "CLOSED"
	now := time.Now()
	trade.ExitTime = &now

	pnl, retPct := trade.CalculatePnL()
	trade.RealizedPnL = pnl
	trade.ReturnPct = retPct

	// Credit cash back + PnL
	proceeds := (trade.PositionSize * trade.EntryPrice) + pnl
	e.cash += proceeds

	delete(e.positions, symbol)
	e.closedTrades = append(e.closedTrades, trade)

	return trade, true
}

// GetTotalEquity returns cash plus unrealized marked-to-market position values.
func (e *ExecutionEngine) GetTotalEquity(currentPrices map[string]float64) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	equity := e.cash
	for sym, pos := range e.positions {
		currPrice, ok := currentPrices[sym]
		if !ok {
			currPrice = pos.EntryPrice
		}
		var diff float64
		if pos.Side == "BUY" {
			diff = currPrice - pos.EntryPrice
		} else {
			diff = pos.EntryPrice - currPrice
		}
		unrealized := diff * pos.PositionSize
		equity += (pos.PositionSize * pos.EntryPrice) + unrealized
	}

	return equity
}
