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
	ErrMLRunNotFound = errors.New("ml training run not found")
)

// MLTrainingRun represents an immutable record of real-data model training.
type MLTrainingRun struct {
	ID                  int64           `json:"id"`
	Symbol              string          `json:"symbol"`
	Timeframe           string          `json:"timeframe"`
	SampleCount         int             `json:"sample_count"`
	DateStart           time.Time       `json:"date_start"`
	DateEnd             time.Time       `json:"date_end"`
	TrainingLoss        float64         `json:"training_loss"`
	DirectionalAccuracy float64         `json:"directional_accuracy"`
	WeightsSnapshot     json.RawMessage `json:"weights_snapshot"`
	CreatedAt           time.Time       `json:"created_at"`
}

// RecordMLTrainingRun persists an authentic market data calibration run to PostgreSQL.
func (s *Store) RecordMLTrainingRun(ctx context.Context, run *MLTrainingRun) (*MLTrainingRun, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}

	row := s.Pool.QueryRow(ctx, `
		INSERT INTO ml_training_runs (
			symbol, timeframe, sample_count, date_start, date_end,
			training_loss, directional_accuracy, weights_snapshot
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		)
		RETURNING id, created_at
	`,
		run.Symbol, run.Timeframe, run.SampleCount, run.DateStart, run.DateEnd,
		run.TrainingLoss, run.DirectionalAccuracy, run.WeightsSnapshot,
	)

	err := row.Scan(&run.ID, &run.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to record ml training run: %w", err)
	}

	return run, nil
}

// ListMLTrainingRuns fetches historical training runs for a symbol or all symbols.
func (s *Store) ListMLTrainingRuns(ctx context.Context, symbol string, limit int) ([]MLTrainingRun, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var rows pgx.Rows
	var err error

	if symbol != "" {
		rows, err = s.Pool.Query(ctx, `
			SELECT id, symbol, timeframe, sample_count, date_start, date_end,
			       training_loss, directional_accuracy, weights_snapshot, created_at
			FROM ml_training_runs
			WHERE symbol = $1
			ORDER BY created_at DESC
			LIMIT $2
		`, symbol, limit)
	} else {
		rows, err = s.Pool.Query(ctx, `
			SELECT id, symbol, timeframe, sample_count, date_start, date_end,
			       training_loss, directional_accuracy, weights_snapshot, created_at
			FROM ml_training_runs
			ORDER BY created_at DESC
			LIMIT $1
		`, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query ml training runs: %w", err)
	}
	defer rows.Close()

	runs := make([]MLTrainingRun, 0)
	for rows.Next() {
		var r MLTrainingRun
		err := rows.Scan(
			&r.ID, &r.Symbol, &r.Timeframe, &r.SampleCount, &r.DateStart, &r.DateEnd,
			&r.TrainingLoss, &r.DirectionalAccuracy, &r.WeightsSnapshot, &r.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ml training run row: %w", err)
		}
		runs = append(runs, r)
	}

	return runs, nil
}
