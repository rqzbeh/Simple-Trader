package db

import (
	"context"
	"fmt"
)

// ManagedParamsReport returns aggregated counts per parameter mode (user_override vs core_managed)
// joined to closed-trade outcomes (FR-306, SC-305).
func (s *Store) ManagedParamsReport(ctx context.Context, days int) (map[string]interface{}, error) {
	if days <= 0 {
		days = 14
	}
	if s.Pool == nil {
		return nil, fmt.Errorf("managed_params_store: database unavailable")
	}

	var total, closedCount int
	err := s.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'CLOSED')
		FROM futures_trade_signals
		WHERE created_at > now() - make_interval(days => $1)`,
		days).Scan(&total, &closedCount)
	if err != nil {
		return nil, fmt.Errorf("managed params report summary: %w", err)
	}

	params := make(map[string]interface{})
	keys := []string{"min_rr", "leverage", "conviction", "atr_regime", "decay", "confluence"}

	for _, k := range keys {
		var overrideCount, managedCount int
		var overrideAvgPnl, managedAvgPnl, overrideAvgROI, managedAvgROI float64

		err := s.Pool.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE parameter_modes->>$1 = 'user_override'),
				COUNT(*) FILTER (WHERE parameter_modes->>$1 = 'core_managed'),
				COALESCE(AVG(realized_pnl_usd) FILTER (WHERE parameter_modes->>$1 = 'user_override' AND status = 'CLOSED'), 0),
				COALESCE(AVG(realized_pnl_usd) FILTER (WHERE parameter_modes->>$1 = 'core_managed' AND status = 'CLOSED'), 0),
				COALESCE(AVG(realized_roi_pct) FILTER (WHERE parameter_modes->>$1 = 'user_override' AND status = 'CLOSED'), 0),
				COALESCE(AVG(realized_roi_pct) FILTER (WHERE parameter_modes->>$1 = 'core_managed' AND status = 'CLOSED'), 0)
			FROM futures_trade_signals
			WHERE created_at > now() - make_interval(days => $2)`,
			k, days).Scan(
			&overrideCount, &managedCount,
			&overrideAvgPnl, &managedAvgPnl,
			&overrideAvgROI, &managedAvgROI,
		)
		if err != nil {
			return nil, fmt.Errorf("managed params report for %q: %w", k, err)
		}

		params[k] = map[string]interface{}{
			"override_count":   overrideCount,
			"managed_count":    managedCount,
			"override_avg_pnl": overrideAvgPnl,
			"managed_avg_pnl":  managedAvgPnl,
			"override_avg_roi": overrideAvgROI,
			"managed_avg_roi":  managedAvgROI,
		}
	}

	return map[string]interface{}{
		"type":           "params",
		"days":           days,
		"total_signals":  total,
		"closed_signals": closedCount,
		"params":         params,
	}, nil
}
