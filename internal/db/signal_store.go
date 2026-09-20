package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrSignalNotFound = errors.New("futures trade signal not found")
	ErrRegimeNotFound = errors.New("macro regime not found")
)

// FuturesTradeSignal represents an executed or active two-sided futures signal.
type FuturesTradeSignal struct {
	ID                  int64      `json:"id"`
	Symbol              string     `json:"symbol"`
	Direction           string     `json:"direction"` // "LONG" or "SHORT"
	Status              string     `json:"status"`    // "ACTIVE", "CLOSED", "CANCELLED"
	CatalystHeadline    string     `json:"catalyst_headline"`
	CatalystSource      string     `json:"catalyst_source"`
	CatalystSentiment   float64    `json:"catalyst_sentiment"`
	EntryPrice          float64    `json:"entry_price"`
	StopLoss            float64    `json:"stop_loss"`
	TakeProfit1         float64    `json:"take_profit_1"`
	TakeProfit2         *float64   `json:"take_profit_2,omitempty"`
	Leverage            int        `json:"leverage"`
	RiskRewardRatio     float64    `json:"risk_reward_ratio"`
	AllocatedCapitalUSD float64    `json:"allocated_capital_usd"`
	AllocatedCapitalPct float64    `json:"allocated_capital_pct"`
	ExitPrice           *float64   `json:"exit_price,omitempty"`
	ExitTime            *time.Time `json:"exit_time,omitempty"`
	ExitReason          *string    `json:"exit_reason,omitempty"`
	RealizedPnLUSD      *float64   `json:"realized_pnl_usd,omitempty"`
	RealizedROIPct      *float64   `json:"realized_roi_pct,omitempty"`
	TelegramDispatched  bool       `json:"telegram_dispatched"`
	TelegramResolved    bool       `json:"telegram_resolved"`
	CreatedAt           time.Time  `json:"created_at"`
}

// MacroRegime represents a recorded macroeconomic and geopolitical regime state with tier allocations.
type MacroRegime struct {
	ID                    int64     `json:"id"`
	Timestamp             time.Time `json:"timestamp"`
	StressScore           float64   `json:"stress_score"`
	GeopoliticalRiskIndex float64   `json:"geopolitical_risk_index"`
	InflationRatePct      float64   `json:"inflation_rate_pct"`
	InterestRateBias      string    `json:"interest_rate_bias"` // "DOVISH", "NEUTRAL", "HAWKISH"
	TargetTier1CashPct    float64   `json:"target_tier1_cash_pct"`
	TargetTier2AlphaPct   float64   `json:"target_tier2_alpha_pct"`
	TargetTier3CorePct    float64   `json:"target_tier3_core_pct"`
	Rationale             string    `json:"rationale"`
}

// InsertFuturesSignal inserts a newly generated futures signal.
func (s *Store) InsertFuturesSignal(ctx context.Context, sig *FuturesTradeSignal) (*FuturesTradeSignal, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}

	row := s.Pool.QueryRow(ctx, `
		INSERT INTO futures_trade_signals (
			symbol, direction, status, catalyst_headline, catalyst_source, catalyst_sentiment,
			entry_price, stop_loss, take_profit_1, take_profit_2, leverage,
			risk_reward_ratio, allocated_capital_usd, allocated_capital_pct,
			telegram_dispatched, telegram_resolved
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $13, $14,
			$15, $16
		)
		RETURNING id, created_at
	`,
		sig.Symbol, sig.Direction, sig.Status, sig.CatalystHeadline, sig.CatalystSource, sig.CatalystSentiment,
		sig.EntryPrice, sig.StopLoss, sig.TakeProfit1, sig.TakeProfit2, sig.Leverage,
		sig.RiskRewardRatio, sig.AllocatedCapitalUSD, sig.AllocatedCapitalPct,
		sig.TelegramDispatched, sig.TelegramResolved,
	)

	err := row.Scan(&sig.ID, &sig.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert futures trade signal: %w", err)
	}

	return sig, nil
}

// ListFuturesSignals retrieves futures trade signals filtered by status and limit.
func (s *Store) ListFuturesSignals(ctx context.Context, status string, limit int) ([]FuturesTradeSignal, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var rows pgx.Rows
	var err error

	if status != "" && status != "ALL" {
		rows, err = s.Pool.Query(ctx, `
			SELECT id, symbol, direction, status, catalyst_headline, catalyst_source, catalyst_sentiment,
			       entry_price, stop_loss, take_profit_1, take_profit_2, leverage,
			       risk_reward_ratio, allocated_capital_usd, allocated_capital_pct,
			       exit_price, exit_time, exit_reason, realized_pnl_usd, realized_roi_pct,
			       telegram_dispatched, telegram_resolved, created_at
			FROM futures_trade_signals
			WHERE status = $1
			ORDER BY created_at DESC
			LIMIT $2
		`, status, limit)
	} else {
		rows, err = s.Pool.Query(ctx, `
			SELECT id, symbol, direction, status, catalyst_headline, catalyst_source, catalyst_sentiment,
			       entry_price, stop_loss, take_profit_1, take_profit_2, leverage,
			       risk_reward_ratio, allocated_capital_usd, allocated_capital_pct,
			       exit_price, exit_time, exit_reason, realized_pnl_usd, realized_roi_pct,
			       telegram_dispatched, telegram_resolved, created_at
			FROM futures_trade_signals
			ORDER BY created_at DESC
			LIMIT $1
		`, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query futures signals: %w", err)
	}
	defer rows.Close()

	signals := make([]FuturesTradeSignal, 0)
	for rows.Next() {
		var sig FuturesTradeSignal
		err := rows.Scan(
			&sig.ID, &sig.Symbol, &sig.Direction, &sig.Status, &sig.CatalystHeadline, &sig.CatalystSource, &sig.CatalystSentiment,
			&sig.EntryPrice, &sig.StopLoss, &sig.TakeProfit1, &sig.TakeProfit2, &sig.Leverage,
			&sig.RiskRewardRatio, &sig.AllocatedCapitalUSD, &sig.AllocatedCapitalPct,
			&sig.ExitPrice, &sig.ExitTime, &sig.ExitReason, &sig.RealizedPnLUSD, &sig.RealizedROIPct,
			&sig.TelegramDispatched, &sig.TelegramResolved, &sig.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan futures signal row: %w", err)
		}
		signals = append(signals, sig)
	}

	return signals, nil
}

// GetActiveFuturesSignalBySymbol gets the currently active futures signal for an asset if any.
func (s *Store) GetActiveFuturesSignalBySymbol(ctx context.Context, symbol string) (*FuturesTradeSignal, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}

	var sig FuturesTradeSignal
	err := s.Pool.QueryRow(ctx, `
		SELECT id, symbol, direction, status, catalyst_headline, catalyst_source, catalyst_sentiment,
		       entry_price, stop_loss, take_profit_1, take_profit_2, leverage,
		       risk_reward_ratio, allocated_capital_usd, allocated_capital_pct,
		       exit_price, exit_time, exit_reason, realized_pnl_usd, realized_roi_pct,
		       telegram_dispatched, telegram_resolved, created_at
		FROM futures_trade_signals
		WHERE symbol = $1 AND status = 'ACTIVE'
		ORDER BY created_at DESC
		LIMIT 1
	`, symbol).Scan(
		&sig.ID, &sig.Symbol, &sig.Direction, &sig.Status, &sig.CatalystHeadline, &sig.CatalystSource, &sig.CatalystSentiment,
		&sig.EntryPrice, &sig.StopLoss, &sig.TakeProfit1, &sig.TakeProfit2, &sig.Leverage,
		&sig.RiskRewardRatio, &sig.AllocatedCapitalUSD, &sig.AllocatedCapitalPct,
		&sig.ExitPrice, &sig.ExitTime, &sig.ExitReason, &sig.RealizedPnLUSD, &sig.RealizedROIPct,
		&sig.TelegramDispatched, &sig.TelegramResolved, &sig.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSignalNotFound
		}
		return nil, fmt.Errorf("failed to query active signal: %w", err)
	}

	return &sig, nil
}

// CloseFuturesSignal records signal closure with realized PnL and leveraged ROI %.
func (s *Store) CloseFuturesSignal(ctx context.Context, id int64, exitPrice float64, exitReason string, pnl, roi float64) error {
	if s.Pool == nil {
		return errors.New("database pool not initialized")
	}

	_, err := s.Pool.Exec(ctx, `
		UPDATE futures_trade_signals
		SET status = 'CLOSED',
		    exit_price = $1,
		    exit_time = NOW(),
		    exit_reason = $2,
		    realized_pnl_usd = $3,
		    realized_roi_pct = $4
		WHERE id = $5
	`, exitPrice, exitReason, pnl, roi, id)

	if err != nil {
		return fmt.Errorf("failed to close futures signal: %w", err)
	}

	return nil
}

// MarkSignalDispatched marks a signal as dispatched to Telegram.
func (s *Store) MarkSignalDispatched(ctx context.Context, id int64) error {
	if s.Pool == nil {
		return errors.New("database pool not initialized")
	}
	_, err := s.Pool.Exec(ctx, `UPDATE futures_trade_signals SET telegram_dispatched = TRUE WHERE id = $1`, id)
	return err
}

// MarkSignalResolved marks a closed signal as resolved to Telegram.
func (s *Store) MarkSignalResolved(ctx context.Context, id int64) error {
	if s.Pool == nil {
		return errors.New("database pool not initialized")
	}
	_, err := s.Pool.Exec(ctx, `UPDATE futures_trade_signals SET telegram_resolved = TRUE WHERE id = $1`, id)
	return err
}

// InsertMacroRegime records a dynamic macro regime snapshot.
func (s *Store) InsertMacroRegime(ctx context.Context, r *MacroRegime) (*MacroRegime, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}

	row := s.Pool.QueryRow(ctx, `
		INSERT INTO macro_regimes (
			stress_score, geopolitical_risk_index, inflation_rate_pct, interest_rate_bias,
			target_tier1_cash_pct, target_tier2_alpha_pct, target_tier3_core_pct, rationale
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
		RETURNING id, timestamp
	`,
		r.StressScore, r.GeopoliticalRiskIndex, r.InflationRatePct, r.InterestRateBias,
		r.TargetTier1CashPct, r.TargetTier2AlphaPct, r.TargetTier3CorePct, r.Rationale,
	)

	err := row.Scan(&r.ID, &r.Timestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to insert macro regime: %w", err)
	}

	return r, nil
}

// GetLatestMacroRegime retrieves the most recent macroeconomic regime snapshot.
func (s *Store) GetLatestMacroRegime(ctx context.Context) (*MacroRegime, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}

	var r MacroRegime
	err := s.Pool.QueryRow(ctx, `
		SELECT id, timestamp, stress_score, geopolitical_risk_index, inflation_rate_pct,
		       interest_rate_bias, target_tier1_cash_pct, target_tier2_alpha_pct,
		       target_tier3_core_pct, rationale
		FROM macro_regimes
		ORDER BY timestamp DESC
		LIMIT 1
	`).Scan(
		&r.ID, &r.Timestamp, &r.StressScore, &r.GeopoliticalRiskIndex, &r.InflationRatePct,
		&r.InterestRateBias, &r.TargetTier1CashPct, &r.TargetTier2AlphaPct,
		&r.TargetTier3CorePct, &r.Rationale,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRegimeNotFound
		}
		return nil, fmt.Errorf("failed to get latest macro regime: %w", err)
	}

	return &r, nil
}
