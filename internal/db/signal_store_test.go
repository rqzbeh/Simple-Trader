package db

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestTruePnlBackfillIdempotent verifies migration 000005 recomputes a
// placeholder-zero TIME_EXIT row exactly once (second application changes
// nothing). The migration file carries its own BEGIN/COMMIT, so the test
// runs it on the pool — executing it inside a pgx tx would report the
// COMMIT command tag and hide the UPDATE count.
// Requires a live database: set TEST_DATABASE_URL, otherwise the test skips
// (quickstart §2 runs it against the local Compose stack).
func TestTruePnlBackfillIdempotent(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping live-DB backfill test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	// Fixture: LONG, entry 100 -> exit 101, 8x leverage, 8000 capital.
	// True ROI = +8%, PnL = 640 USD.
	var id int64
	err = pool.QueryRow(ctx, `
		INSERT INTO futures_trade_signals
			(symbol, direction, status, catalyst_headline, catalyst_source, catalyst_sentiment,
			 entry_price, stop_loss, take_profit_1, leverage, risk_reward_ratio,
			 allocated_capital_usd, allocated_capital_pct, exit_price, exit_time,
			 exit_reason, realized_pnl_usd, realized_roi_pct, profile, decay_state)
		VALUES ('TEST/USDT', 'LONG', 'CLOSED', 'fixture', 'test', 0,
			100, 99, 103, 8, 3,
			8000, 0.2, 101, NOW(),
			'TIME_EXIT', 0, 0, 'CRYPTO', 'NONE')
		RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("insert fixture: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM futures_trade_signals WHERE id = $1`, id)
	}()

	sqlBytes, err := os.ReadFile("migrations/000005_true_pnl_backfill.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	// First run must recompute the fixture.
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("first backfill run: %v", err)
	}

	var roi, pnl float64
	var recomputed bool
	err = pool.QueryRow(ctx,
		`SELECT realized_roi_pct, realized_pnl_usd, recomputed FROM futures_trade_signals WHERE id = $1`, id,
	).Scan(&roi, &pnl, &recomputed)
	if err != nil {
		t.Fatalf("read backfilled row: %v", err)
	}
	if math.Abs(roi-8.0) > 1e-6 {
		t.Errorf("roi = %.10f, want 8.0", roi)
	}
	if math.Abs(pnl-640.0) > 1e-6 {
		t.Errorf("pnl = %.10f, want 640.0", pnl)
	}
	if !recomputed {
		t.Errorf("recomputed = false, want true")
	}

	// Footprint of every TIME_EXIT row after run 1: run 2 must change nothing.
	footprint := func() (count int64, roiSum, pnlSum float64, unrecomputed int64) {
		t.Helper()
		err := pool.QueryRow(ctx, `
			SELECT count(*),
			       COALESCE(sum(realized_roi_pct), 0),
			       COALESCE(sum(realized_pnl_usd), 0),
			       count(*) FILTER (WHERE NOT recomputed)
			FROM futures_trade_signals
			WHERE status = 'CLOSED' AND exit_reason = 'TIME_EXIT'`).Scan(
			&count, &roiSum, &pnlSum, &unrecomputed)
		if err != nil {
			t.Fatalf("footprint: %v", err)
		}
		return
	}

	c1, r1, p1, u1 := footprint()
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("second backfill run: %v", err)
	}
	c2, r2, p2, u2 := footprint()
	if c1 != c2 || r1 != r2 || p1 != p2 || u1 != u2 {
		t.Errorf("second run changed TIME_EXIT footprint: (count %d->%d, roiSum %v->%v, pnlSum %v->%v, unrecomputed %d->%d), want no change (idempotency)",
			c1, c2, r1, r2, p1, p2, u1, u2)
	}
}

// TestSignalIndicatorSnapshotRoundtrip verifies migration 000006 +
// scanFuturesSignal roundtrip (spec 012 US7, FR-022): a decision-time
// snapshot persists and reads back byte-identical; a legacy NULL row scans
// to a nil RawMessage so attribution stays skipped (Amendment A1).
// Requires TEST_DATABASE_URL (quickstart §2 pattern).
func TestSignalIndicatorSnapshotRoundtrip(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping live-DB snapshot roundtrip test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	// Apply migration 000006 idempotently inside the transaction.
	mig, err := os.ReadFile("migrations/000006_signal_indicator_snapshot.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := tx.Exec(ctx, string(mig)); err != nil {
		t.Fatalf("apply 000006: %v", err)
	}

	want := IndicatorSnapshotRecord{
		RSI: 61.5, MACDHistogram: 0.42, SuperTrend: "BULL",
		CMF: 0.12, KaufmanER: 0.55, OBI: 0.3, Divergence: "NONE",
	}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var withID, withoutID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO futures_trade_signals
			(symbol, direction, status, catalyst_headline, catalyst_source, catalyst_sentiment,
			 entry_price, stop_loss, take_profit_1, leverage, risk_reward_ratio,
			 allocated_capital_usd, allocated_capital_pct, profile, decay_state, indicator_snapshot)
		VALUES ('SNAP/USDT', 'LONG', 'CLOSED', 'fixture', 'test', 0,
			100, 99, 103, 5, 3, 4000, 4, 'CRYPTO', 'NONE', $1)
		RETURNING id`, raw,
	).Scan(&withID)
	if err != nil {
		t.Fatalf("insert with snapshot: %v", err)
	}

	// Legacy shape: column omitted → NULL.
	err = tx.QueryRow(ctx, `
		INSERT INTO futures_trade_signals
			(symbol, direction, status, catalyst_headline, catalyst_source, catalyst_sentiment,
			 entry_price, stop_loss, take_profit_1, leverage, risk_reward_ratio,
			 allocated_capital_usd, allocated_capital_pct, profile, decay_state)
		VALUES ('LEGACY/USDT', 'LONG', 'CLOSED', 'fixture', 'test', 0,
			100, 99, 103, 5, 3, 4000, 4, 'CRYPTO', 'NONE')
		RETURNING id`,
	).Scan(&withoutID)
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	sig, err := scanFuturesSignal(tx.QueryRow(ctx,
		`SELECT `+futuresSignalColumns+` FROM futures_trade_signals WHERE id = $1`, withID))
	if err != nil {
		t.Fatalf("scan snapshot row: %v", err)
	}
	var got IndicatorSnapshotRecord
	if err := json.Unmarshal(sig.IndicatorSnapshot, &got); err != nil {
		t.Fatalf("unmarshal scanned snapshot: %v", err)
	}
	if got != want {
		t.Errorf("snapshot = %+v, want %+v", got, want)
	}

	legacy, err := scanFuturesSignal(tx.QueryRow(ctx,
		`SELECT `+futuresSignalColumns+` FROM futures_trade_signals WHERE id = $1`, withoutID))
	if err != nil {
		t.Fatalf("scan legacy row: %v", err)
	}
	if legacy.IndicatorSnapshot != nil {
		t.Errorf("legacy snapshot = %s, want nil (unknown must stay unknown)", legacy.IndicatorSnapshot)
	}
}
