package db

import (
	"context"
	"fmt"
	"time"
)

// EarlyExitJudgment corresponds to early_exit_judgments table (migration 000008).
type EarlyExitJudgment struct {
	ID              int64
	CreatedAt       time.Time
	CycleID         string
	PositionID      int64
	Symbol          string
	ClusterID       *int64
	ClusterHeadline string
	Verdict         string // HOLD | DO_NOT_HOLD | ""
	Noul            *float64
	Confidence      *float64
	Route           string // jev_direct | escalated
	GuardsPassed    *bool
	GuardReason     string
	Action          string // closed | guarded_skip | error
	TelegramSent    *bool
	TelegramError   string
	CloseError      string
	Status          string // ok | error
	Error           string
	Outcome         *float64
}

var allowedEarlyExitVerdicts = map[string]bool{
	"HOLD":        true,
	"DO_NOT_HOLD": true,
}

var allowedEarlyExitActions = map[string]bool{
	"closed":       true,
	"guarded_skip": true,
	"error":        true,
}

var allowedEarlyExitRoutes = map[string]bool{
	"jev_direct": true,
	"escalated":  true,
}

// Validate validates early exit judgment rules before persistence.
func (j *EarlyExitJudgment) Validate() error {
	if j.CycleID == "" {
		return fmt.Errorf("early_exit_judgments: empty cycle_id")
	}
	if j.PositionID <= 0 {
		return fmt.Errorf("early_exit_judgments: position_id must be > 0")
	}
	if j.Symbol == "" {
		return fmt.Errorf("early_exit_judgments: empty symbol")
	}
	switch j.Status {
	case "ok", "error":
	default:
		return fmt.Errorf("early_exit_judgments: bad status %q", j.Status)
	}

	if j.Status == "error" {
		if j.Error == "" {
			return fmt.Errorf("early_exit_judgments: status=error requires non-empty error (FR-105)")
		}
		if j.Verdict != "" {
			return fmt.Errorf("early_exit_judgments: error rows must carry no verdict")
		}
		return nil
	}

	// Status == "ok"
	if j.Error != "" {
		return fmt.Errorf("early_exit_judgments: status=ok must not carry error")
	}
	if j.Verdict != "" && !allowedEarlyExitVerdicts[j.Verdict] {
		return fmt.Errorf("early_exit_judgments: verdict %q outside vocabulary HOLD/DO_NOT_HOLD (FR-107)", j.Verdict)
	}
	if j.Action != "" && !allowedEarlyExitActions[j.Action] {
		return fmt.Errorf("early_exit_judgments: action %q outside allowed set", j.Action)
	}
	if j.Route != "" && !allowedEarlyExitRoutes[j.Route] {
		return fmt.Errorf("early_exit_judgments: route %q outside allowed set", j.Route)
	}
	return nil
}

// InsertEarlyExitJudgment inserts an early exit judgment row.
func (s *Store) InsertEarlyExitJudgment(ctx context.Context, j *EarlyExitJudgment) error {
	if err := j.Validate(); err != nil {
		return err
	}
	if s.Pool == nil {
		return fmt.Errorf("early_exit_store: database pool is nil")
	}

	row := s.Pool.QueryRow(ctx, `
		INSERT INTO early_exit_judgments
		(cycle_id, position_id, symbol, cluster_id, cluster_headline, verdict,
		 noul, confidence, route, guards_passed, guard_reason, action,
		 telegram_sent, telegram_error, close_error, status, error, outcome)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		RETURNING id, created_at`,
		j.CycleID, j.PositionID, j.Symbol, j.ClusterID, strPtr(j.ClusterHeadline), strPtr(j.Verdict),
		j.Noul, j.Confidence, strPtr(j.Route), j.GuardsPassed, strPtr(j.GuardReason), strPtr(j.Action),
		j.TelegramSent, strPtr(j.TelegramError), strPtr(j.CloseError), j.Status, strPtr(j.Error), j.Outcome)

	return row.Scan(&j.ID, &j.CreatedAt)
}

// HasTerminalEarlyExitJudgment checks if a terminal judgment (closed or guarded_skip)
// already exists for this (position_id, cluster_id) pair.
func (s *Store) HasTerminalEarlyExitJudgment(ctx context.Context, positionID int64, clusterID int64) (bool, error) {
	if s.Pool == nil {
		return false, nil
	}
	var count int
	err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM early_exit_judgments
		WHERE position_id = $1 AND cluster_id = $2 AND action IN ('closed', 'guarded_skip')`,
		positionID, clusterID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("has terminal early exit judgment: %w", err)
	}
	return count > 0, nil
}

// CountEarlyExitsToday counts executed early exits for a symbol on the current calendar day (UTC).
func (s *Store) CountEarlyExitsToday(ctx context.Context, symbol string, now time.Time) (int, error) {
	if s.Pool == nil {
		return 0, nil
	}
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var count int
	err := s.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM early_exit_judgments
		WHERE symbol = $1 AND action = 'closed' AND created_at >= $2`,
		symbol, startOfDay).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count early exits today: %w", err)
	}
	return count, nil
}

// GetLastEarlyExitTime returns the timestamp of the last executed early exit for a symbol.
func (s *Store) GetLastEarlyExitTime(ctx context.Context, symbol string) (time.Time, bool, error) {
	if s.Pool == nil {
		return time.Time{}, false, nil
	}
	var t time.Time
	err := s.Pool.QueryRow(ctx, `
		SELECT created_at
		FROM early_exit_judgments
		WHERE symbol = $1 AND action = 'closed'
		ORDER BY created_at DESC
		LIMIT 1`, symbol).Scan(&t)
	if err != nil {
		if err.Error() == "no rows in result set" || err.Error() == "sql: no rows in result set" {
			return time.Time{}, false, nil
		}
		// pgx returns pgx.ErrNoRows which strings match
		return time.Time{}, false, nil
	}
	return t, true, nil
}

// BackfillEarlyExitOutcome updates the realized outcome for judgments matching position_id (FR-106).
func (s *Store) BackfillEarlyExitOutcome(ctx context.Context, positionID int64, outcome float64) error {
	if s.Pool == nil {
		return nil
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE early_exit_judgments
		SET outcome = $1
		WHERE position_id = $2 AND outcome IS NULL`,
		outcome, positionID)
	if err != nil {
		return fmt.Errorf("backfill early exit outcome: %w", err)
	}
	return nil
}

// EarlyExitReport returns aggregated counts and outcome analysis (FR-106 / US3).
func (s *Store) EarlyExitReport(ctx context.Context, days int) (map[string]interface{}, error) {
	if days <= 0 {
		days = 14
	}
	if s.Pool == nil {
		return nil, fmt.Errorf("early_exit_store: database unavailable")
	}

	var total, closedCount, guardedSkipCount, errorCount, withOutcome int
	var avgConf, avgOutcome float64

	err := s.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE action = 'closed'),
			COUNT(*) FILTER (WHERE action = 'guarded_skip'),
			COUNT(*) FILTER (WHERE action = 'error' OR status = 'error'),
			COUNT(*) FILTER (WHERE outcome IS NOT NULL),
			COALESCE(AVG(confidence), 0),
			COALESCE(AVG(outcome) FILTER (WHERE outcome IS NOT NULL), 0)
		FROM early_exit_judgments
		WHERE created_at > now() - make_interval(days => $1)`,
		days).Scan(&total, &closedCount, &guardedSkipCount, &errorCount, &withOutcome, &avgConf, &avgOutcome)
	if err != nil {
		return nil, fmt.Errorf("early exit report: %w", err)
	}

	return map[string]interface{}{
		"type":                 "news_exit",
		"days":                 days,
		"total_judgments":      total,
		"closed":               closedCount,
		"guarded_skip":         guardedSkipCount,
		"errors":               errorCount,
		"records_with_outcome": withOutcome,
		"avg_confidence":       avgConf,
		"avg_outcome":          avgOutcome,
	}, nil
}
