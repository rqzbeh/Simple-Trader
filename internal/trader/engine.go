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
	AIReasoning       string
	SymbolSpreadPct   float64
	AvailableDepthQty float64
	IsTaker           bool
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

	quote := e.friction.CalculateExecution(
		req.Side,
		req.PositionSize,
		req.Price,
		req.SymbolSpreadPct,
		req.AvailableDepthQty,
		req.IsTaker,
	)

	if quote.TotalNetCost > e.cash {
		return nil, fmt.Errorf("insufficient cash to execute order: need %.2f, have %.2f", quote.TotalNetCost, e.cash)
	}

	trade := &db.Trade{
		ID:           e.orderCounter,
		Symbol:       req.Symbol,
		Bucket:       req.Bucket,
		Side:         req.Side,
		EntryPrice:   quote.EffectivePrice,
		EntryTime:    now,
		PositionSize: req.PositionSize,
		StopLoss:     req.StopLoss,
		TakeProfit:   req.TakeProfit,
		ExecutionFee: quote.TotalFee,
		SlippagePaid: quote.Slippage,
		Status:       "OPEN",
		CreatedAt:    now,
	}

	e.cash -= quote.TotalNetCost
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

	exitSide := "SELL"
	if trade.Side == "SELL" {
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

	// Credit proceeds back
	// Buy position closed: proceeds = (size * exitPrice) - exitFee
	// Sell position closed: proceeds = (size * entryPrice) + pnl
	proceeds := (trade.PositionSize * trade.EntryPrice) + pnl
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
