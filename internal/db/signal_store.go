package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrSignalNotFound = errors.New("futures trade signal not found")
	ErrRegimeNotFound = errors.New("macro regime not found")
)

// futuresSignalColumns is the canonical column list for futures_trade_signals.
// Every SELECT must use it so scans stay in sync with the schema
// (migration 000004 added the optimization columns).
const futuresSignalColumns = `id, symbol, direction, status, catalyst_headline, catalyst_source, catalyst_sentiment,
		entry_price, stop_loss, take_profit_1, take_profit_2, leverage,
		risk_reward_ratio, allocated_capital_usd, allocated_capital_pct,
		exit_price, exit_time, exit_reason, realized_pnl_usd, realized_roi_pct,
		telegram_dispatched, telegram_resolved, created_at,
		profile, model_confidence, catalyst_event_id, atr_at_entry, tp1_close_fraction,
		recomputed, decay_state, rejected_reason, indicator_snapshot`

// defaultString returns fallback when v is empty so CHECK-constrained
// columns (profile, decay_state) never receive ”.
func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// nullableJSON renders an empty snapshot as SQL NULL so legacy rows stay
// distinguishable from rows that carry decision-time measurements (US7).
func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

func scanFuturesSignal(row pgx.Row) (FuturesTradeSignal, error) {
	var sig FuturesTradeSignal
	err := row.Scan(
		&sig.ID, &sig.Symbol, &sig.Direction, &sig.Status, &sig.CatalystHeadline, &sig.CatalystSource, &sig.CatalystSentiment,
		&sig.EntryPrice, &sig.StopLoss, &sig.TakeProfit1, &sig.TakeProfit2, &sig.Leverage,
		&sig.RiskRewardRatio, &sig.AllocatedCapitalUSD, &sig.AllocatedCapitalPct,
		&sig.ExitPrice, &sig.ExitTime, &sig.ExitReason, &sig.RealizedPnLUSD, &sig.RealizedROIPct,
		&sig.TelegramDispatched, &sig.TelegramResolved, &sig.CreatedAt,
		&sig.Profile, &sig.ModelConfidence, &sig.CatalystEventID, &sig.ATRAtEntry, &sig.TP1CloseFraction,
		&sig.Recomputed, &sig.DecayState, &sig.RejectedReason, &sig.IndicatorSnapshot,
	)
	return sig, err
}

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

	// Signal optimization (spec 012, data-model.md §2)
	Profile          string   `json:"profile"`
	ModelConfidence  *float64 `json:"model_confidence,omitempty"`
	CatalystEventID  *int64   `json:"catalyst_event_id,omitempty"`
	ATRAtEntry       *float64 `json:"atr_at_entry,omitempty"`
	TP1CloseFraction *float64 `json:"tp1_close_fraction,omitempty"`
	Recomputed       bool     `json:"recomputed"`
	DecayState       string   `json:"decay_state"`
	RejectedReason   *string  `json:"rejected_reason,omitempty"`
	// IndicatorSnapshot records the decision-time indicator measurements that
	// produced this signal (spec 012 US7, FR-022). Null for legacy rows;
	// null means "unknown" and must never be back-filled (Amendment A1).
	IndicatorSnapshot json.RawMessage `json:"indicator_snapshot,omitempty"`
}

// IndicatorSnapshotRecord is the persisted copy of the decision-time
// indicator measurements that produced a signal (spec 012 US7, FR-022).
// Field set mirrors the six confluence indicators; JSONB column
// futures_trade_signals.indicator_snapshot. Zero/empty means "measurement
// existed but was neutral" only when the whole record is present; a NULL
// record means "legacy row, never back-fill" (Amendment A1).
type IndicatorSnapshotRecord struct {
	RSI           float64 `json:"rsi"`
	MACDHistogram float64 `json:"macd_histogram"`
	SuperTrend    string  `json:"supertrend"` // "BULL" | "BEAR" | ""
	CMF           float64 `json:"cmf"`
	KaufmanER     float64 `json:"kaufman_er"`
	OBI           float64 `json:"obi"`
	Divergence    string  `json:"divergence"`
}

// EntryFilterLog records a candidate entry rejected by a gate rule (spec FR-020).
// Rule is a closed enum documented in contracts/api.md §3.
type EntryFilterLog struct {
	ID              int64           `json:"id"`
	CreatedAt       time.Time       `json:"created_at"`
	Symbol          string          `json:"symbol"`
	Direction       string          `json:"direction"`
	CatalystEventID *int64          `json:"catalyst_event_id,omitempty"`
	Rule            string          `json:"rule"`
	Detail          json.RawMessage `json:"detail"`
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
			telegram_dispatched, telegram_resolved,
			profile, model_confidence, catalyst_event_id, atr_at_entry, tp1_close_fraction, decay_state,
			indicator_snapshot
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $13, $14,
			$15, $16,
			$17, $18, $19, $20, $21, $22,
			$23
		)
		RETURNING id, created_at
	`,
		sig.Symbol, sig.Direction, sig.Status, sig.CatalystHeadline, sig.CatalystSource, sig.CatalystSentiment,
		sig.EntryPrice, sig.StopLoss, sig.TakeProfit1, sig.TakeProfit2, sig.Leverage,
		sig.RiskRewardRatio, sig.AllocatedCapitalUSD, sig.AllocatedCapitalPct,
		sig.TelegramDispatched, sig.TelegramResolved,
		defaultString(sig.Profile, "CRYPTO"), sig.ModelConfidence, sig.CatalystEventID,
		sig.ATRAtEntry, sig.TP1CloseFraction, defaultString(sig.DecayState, "NONE"),
		nullableJSON(sig.IndicatorSnapshot),
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
			SELECT `+futuresSignalColumns+`
			FROM futures_trade_signals
			WHERE status = $1
			ORDER BY created_at DESC
			LIMIT $2
		`, status, limit)
	} else {
		rows, err = s.Pool.Query(ctx, `
			SELECT `+futuresSignalColumns+`
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
		sig, err := scanFuturesSignal(rows)
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

	row := s.Pool.QueryRow(ctx, `
		SELECT `+futuresSignalColumns+`
		FROM futures_trade_signals
		WHERE symbol = $1 AND status = 'ACTIVE'
		ORDER BY created_at DESC
		LIMIT 1
	`, symbol)
	sig, err := scanFuturesSignal(row)

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

// InsertEntryFilterLog persists a rejected entry candidate (spec FR-020).
// detail must be a JSON object; nil becomes {}.
func (s *Store) InsertEntryFilterLog(ctx context.Context, symbol, direction string, catalystEventID *int64, rule string, detail json.RawMessage) error {
	if s.Pool == nil {
		return errors.New("database pool not initialized")
	}
	if len(detail) == 0 {
		detail = json.RawMessage(`{}`)
	}
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO entry_filter_log (symbol, direction, catalyst_event_id, rule, detail)
		VALUES ($1, $2, $3, $4, $5)
	`, symbol, direction, catalystEventID, rule, detail)
	if err != nil {
		return fmt.Errorf("failed to insert entry filter log: %w", err)
	}
	return nil
}

// ListEntryFilterLogs returns the newest rejected-entry audit rows.
func (s *Store) ListEntryFilterLogs(ctx context.Context, limit int, rule, symbol string) ([]EntryFilterLog, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, created_at, symbol, direction, catalyst_event_id, rule, detail
		FROM entry_filter_log
		WHERE ($1 = '' OR rule = $1)
		  AND ($2 = '' OR symbol = $2)
		ORDER BY created_at DESC
		LIMIT $3
	`, rule, symbol, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query entry filter log: %w", err)
	}
	defer rows.Close()

	logs := make([]EntryFilterLog, 0)
	for rows.Next() {
		var e EntryFilterLog
		if err := rows.Scan(&e.ID, &e.CreatedAt, &e.Symbol, &e.Direction, &e.CatalystEventID, &e.Rule, &e.Detail); err != nil {
			return nil, fmt.Errorf("failed to scan entry filter log: %w", err)
		}
		logs = append(logs, e)
	}
	return logs, rows.Err()
}

// UpsertRiskProfile persists the effective parameters of one asset-class
// profile (audit copy of the running config, spec FR-019).
func (s *Store) UpsertRiskProfile(ctx context.Context, p RiskProfileRow) error {
	if s.Pool == nil {
		return errors.New("database pool not initialized")
	}
	blackout, err := json.Marshal(p.BlackoutCalendar)
	if err != nil {
		return fmt.Errorf("failed to marshal blackout calendar: %w", err)
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO risk_profiles (
			profile, horizon_min, horizon_max, decay_breakeven_at_min, decay_flat_at_min,
			sl_atr_mult, sl_swing_offset, sl_min_pct, sl_max_pct,
			tp1_atr_mult, tp2_atr_mult, tp1_close_fraction,
			target_hourly_vol_pct, liq_buffer_min, risk_per_trade_pct,
			freshness_half_life_min, weekend_flat, blackout_calendar, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12,
			$13, $14, $15,
			$16, $17, $18, NOW()
		)
		ON CONFLICT (profile) DO UPDATE SET
			horizon_min = EXCLUDED.horizon_min,
			horizon_max = EXCLUDED.horizon_max,
			decay_breakeven_at_min = EXCLUDED.decay_breakeven_at_min,
			decay_flat_at_min = EXCLUDED.decay_flat_at_min,
			sl_atr_mult = EXCLUDED.sl_atr_mult,
			sl_swing_offset = EXCLUDED.sl_swing_offset,
			sl_min_pct = EXCLUDED.sl_min_pct,
			sl_max_pct = EXCLUDED.sl_max_pct,
			tp1_atr_mult = EXCLUDED.tp1_atr_mult,
			tp2_atr_mult = EXCLUDED.tp2_atr_mult,
			tp1_close_fraction = EXCLUDED.tp1_close_fraction,
			target_hourly_vol_pct = EXCLUDED.target_hourly_vol_pct,
			liq_buffer_min = EXCLUDED.liq_buffer_min,
			risk_per_trade_pct = EXCLUDED.risk_per_trade_pct,
			freshness_half_life_min = EXCLUDED.freshness_half_life_min,
			weekend_flat = EXCLUDED.weekend_flat,
			blackout_calendar = EXCLUDED.blackout_calendar,
			updated_at = NOW()
	`,
		p.Name, p.HorizonMin, p.HorizonMax, p.DecayBreakevenAtMin, p.DecayFlatAtMin,
		p.SLAtrMult, p.SLSwingOffset, p.SLMinPct, p.SLMaxPct,
		p.TP1AtrMult, p.TP2AtrMult, p.TP1CloseFrac,
		p.TargetHourlyVolPct, p.LiqBufferMin, p.RiskPerTradePct,
		p.FreshnessHalfLifeMin, p.WeekendFlat, blackout,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert risk profile: %w", err)
	}
	return nil
}

// RiskProfileRow mirrors the risk_profiles table (JSON tags match the API
// contract in contracts/api.md §4).
type RiskProfileRow struct {
	Name                 string           `json:"profile"`
	HorizonMin           int              `json:"horizon_min"`
	HorizonMax           int              `json:"horizon_max"`
	DecayBreakevenAtMin  int              `json:"decay_breakeven_at_min"`
	DecayFlatAtMin       int              `json:"decay_flat_at_min"`
	SLAtrMult            float64          `json:"sl_atr_mult"`
	SLSwingOffset        float64          `json:"sl_swing_offset"`
	SLMinPct             float64          `json:"sl_min_pct"`
	SLMaxPct             float64          `json:"sl_max_pct"`
	TP1AtrMult           float64          `json:"tp1_atr_mult"`
	TP2AtrMult           float64          `json:"tp2_atr_mult"`
	TP1CloseFrac         float64          `json:"tp1_close_fraction"`
	TargetHourlyVolPct   float64          `json:"target_hourly_vol_pct"`
	LiqBufferMin         float64          `json:"liq_buffer_min"`
	RiskPerTradePct      float64          `json:"risk_per_trade_pct"`
	FreshnessHalfLifeMin float64          `json:"freshness_half_life_min"`
	WeekendFlat          bool             `json:"weekend_flat"`
	BlackoutCalendar     []BlackoutWindow `json:"blackout_calendar"`
	UpdatedAt            time.Time        `json:"updated_at"`
}

// BlackoutWindow mirrors risk_profiles.blackout_calendar entries.
type BlackoutWindow struct {
	Name      string `json:"name"`
	BeforeMin int    `json:"before_min"`
	AfterMin  int    `json:"after_min"`
}

// ListRiskProfiles returns persisted risk profiles ordered by name.
func (s *Store) ListRiskProfiles(ctx context.Context) ([]RiskProfileRow, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT profile, horizon_min, horizon_max, decay_breakeven_at_min, decay_flat_at_min,
		       sl_atr_mult, sl_swing_offset, sl_min_pct, sl_max_pct,
		       tp1_atr_mult, tp2_atr_mult, tp1_close_fraction,
		       target_hourly_vol_pct, liq_buffer_min, risk_per_trade_pct,
		       freshness_half_life_min, weekend_flat, blackout_calendar, updated_at
		FROM risk_profiles
		ORDER BY profile
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query risk profiles: %w", err)
	}
	defer rows.Close()

	out := make([]RiskProfileRow, 0, 2)
	for rows.Next() {
		var p RiskProfileRow
		var blackout []byte
		if err := rows.Scan(
			&p.Name, &p.HorizonMin, &p.HorizonMax, &p.DecayBreakevenAtMin, &p.DecayFlatAtMin,
			&p.SLAtrMult, &p.SLSwingOffset, &p.SLMinPct, &p.SLMaxPct,
			&p.TP1AtrMult, &p.TP2AtrMult, &p.TP1CloseFrac,
			&p.TargetHourlyVolPct, &p.LiqBufferMin, &p.RiskPerTradePct,
			&p.FreshnessHalfLifeMin, &p.WeekendFlat, &blackout, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan risk profile: %w", err)
		}
		if len(blackout) > 0 {
			if err := json.Unmarshal(blackout, &p.BlackoutCalendar); err != nil {
				return nil, fmt.Errorf("failed to unmarshal blackout calendar: %w", err)
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
