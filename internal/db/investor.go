package db

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
)

var (
	ErrInvestorNotFound      = errors.New("investor not found")
	ErrInsufficientBalance   = errors.New("insufficient investor equity balance")
	ErrInsufficientLiquidity = errors.New("insufficient tier 1 liquidity buffer")
	ErrInvalidAmount         = errors.New("amount must be greater than zero")
)

// CalculateNAV returns current pool NAV per unit. Defaults to 1.0 if total units is 0.
func CalculateNAV(totalPortfolioEquity, totalUnits float64) float64 {
	if totalUnits <= 0 || totalPortfolioEquity <= 0 {
		return 1.0
	}
	nav := totalPortfolioEquity / totalUnits
	return math.Round(nav*1000000) / 1000000
}

// CreateInvestor registers an investor profile and optionally records their initial deposit.
func (s *Store) CreateInvestor(ctx context.Context, name, contactTag, notes string, initialDeposit, masterPortfolioEquity float64) (*Investor, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Determine current pool units and master equity to calculate NAV
	var totalPoolUnits float64
	err = tx.QueryRow(ctx, `SELECT COALESCE(SUM(pool_units), 0) FROM investors WHERE status = 'ACTIVE'`).Scan(&totalPoolUnits)
	if err != nil {
		return nil, fmt.Errorf("failed to query total pool units: %w", err)
	}

	nav := CalculateNAV(masterPortfolioEquity, totalPoolUnits)
	var unitsIssued float64
	if initialDeposit > 0 {
		unitsIssued = initialDeposit / nav
	}

	var inv Investor
	err = tx.QueryRow(ctx, `
		INSERT INTO investors (name, contact_tag, notes, total_deposited, pool_units, status)
		VALUES ($1, $2, $3, $4, $5, 'ACTIVE')
		RETURNING id, name, contact_tag, notes, total_deposited, total_withdrawn, pool_units, status, created_at, updated_at
	`, name, contactTag, notes, initialDeposit, unitsIssued).Scan(
		&inv.ID, &inv.Name, &inv.ContactTag, &inv.Notes,
		&inv.TotalDeposited, &inv.TotalWithdrawn, &inv.PoolUnits,
		&inv.Status, &inv.CreatedAt, &inv.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert investor: %w", err)
	}

	if initialDeposit > 0 {
		_, err = tx.Exec(ctx, `
			INSERT INTO investor_transactions (investor_id, tx_type, amount, pool_units, nav_at_execution, notes)
			VALUES ($1, 'DEPOSIT', $2, $3, $4, 'Initial capital deposit')
		`, inv.ID, initialDeposit, unitsIssued, nav)
		if err != nil {
			return nil, fmt.Errorf("failed to record initial deposit transaction: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit investor creation: %w", err)
	}

	// Compute derived fields
	inv.CurrentEquity = inv.PoolUnits * nav
	inv.NetProfit = inv.CurrentEquity + inv.TotalWithdrawn - inv.TotalDeposited
	if inv.TotalDeposited > 0 {
		inv.ROI = math.Round((inv.NetProfit/inv.TotalDeposited)*10000) / 100
	}
	if totalPoolUnits+unitsIssued > 0 {
		inv.PoolSharePct = math.Round((inv.PoolUnits/(totalPoolUnits+unitsIssued))*10000) / 100
	}

	return &inv, nil
}

// ListInvestors returns all investors with live calculated NAV pro-rata attribution.
func (s *Store) ListInvestors(ctx context.Context, masterPortfolioEquity float64) ([]Investor, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT id, name, contact_tag, notes, total_deposited, total_withdrawn, pool_units, status, created_at, updated_at
		FROM investors
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query investors: %w", err)
	}
	defer rows.Close()

	var list []Investor
	var totalPoolUnits float64

	for rows.Next() {
		var inv Investor
		err := rows.Scan(
			&inv.ID, &inv.Name, &inv.ContactTag, &inv.Notes,
			&inv.TotalDeposited, &inv.TotalWithdrawn, &inv.PoolUnits,
			&inv.Status, &inv.CreatedAt, &inv.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan investor row: %w", err)
		}
		if inv.Status == "ACTIVE" {
			totalPoolUnits += inv.PoolUnits
		}
		list = append(list, inv)
	}

	nav := CalculateNAV(masterPortfolioEquity, totalPoolUnits)

	for i := range list {
		list[i].CurrentEquity = math.Round(list[i].PoolUnits*nav*100) / 100
		list[i].NetProfit = math.Round((list[i].CurrentEquity+list[i].TotalWithdrawn-list[i].TotalDeposited)*100) / 100
		if list[i].TotalDeposited > 0 {
			list[i].ROI = math.Round((list[i].NetProfit/list[i].TotalDeposited)*10000) / 100
		}
		if totalPoolUnits > 0 {
			list[i].PoolSharePct = math.Round((list[i].PoolUnits/totalPoolUnits)*10000) / 100
		}
	}

	return list, nil
}

// GetInvestor returns an investor by ID with live metrics and recent transactions.
func (s *Store) GetInvestor(ctx context.Context, id string, masterPortfolioEquity float64) (*Investor, []CapitalTransaction, error) {
	if s.Pool == nil {
		return nil, nil, errors.New("database pool not initialized")
	}

	var inv Investor
	err := s.Pool.QueryRow(ctx, `
		SELECT id, name, contact_tag, notes, total_deposited, total_withdrawn, pool_units, status, created_at, updated_at
		FROM investors WHERE id = $1
	`, id).Scan(
		&inv.ID, &inv.Name, &inv.ContactTag, &inv.Notes,
		&inv.TotalDeposited, &inv.TotalWithdrawn, &inv.PoolUnits,
		&inv.Status, &inv.CreatedAt, &inv.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrInvestorNotFound
		}
		return nil, nil, fmt.Errorf("failed to get investor: %w", err)
	}

	var totalPoolUnits float64
	_ = s.Pool.QueryRow(ctx, `SELECT COALESCE(SUM(pool_units), 0) FROM investors WHERE status = 'ACTIVE'`).Scan(&totalPoolUnits)
	nav := CalculateNAV(masterPortfolioEquity, totalPoolUnits)

	inv.CurrentEquity = math.Round(inv.PoolUnits*nav*100) / 100
	inv.NetProfit = math.Round((inv.CurrentEquity+inv.TotalWithdrawn-inv.TotalDeposited)*100) / 100
	if inv.TotalDeposited > 0 {
		inv.ROI = math.Round((inv.NetProfit/inv.TotalDeposited)*10000) / 100
	}
	if totalPoolUnits > 0 {
		inv.PoolSharePct = math.Round((inv.PoolUnits/totalPoolUnits)*10000) / 100
	}

	// Fetch transactions
	txRows, err := s.Pool.Query(ctx, `
		SELECT id, investor_id, tx_type, amount, pool_units, nav_at_execution, notes, created_at
		FROM investor_transactions
		WHERE investor_id = $1
		ORDER BY created_at DESC
	`, id)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query transactions: %w", err)
	}
	defer txRows.Close()

	var txs []CapitalTransaction
	for txRows.Next() {
		var t CapitalTransaction
		if err := txRows.Scan(&t.ID, &t.InvestorID, &t.TxType, &t.Amount, &t.PoolUnits, &t.NAVAtExecution, &t.Notes, &t.CreatedAt); err == nil {
			txs = append(txs, t)
		}
	}

	return &inv, txs, nil
}

// RecordDeposit records an incremental deposit, issues pool units at current NAV, and updates investor balance.
func (s *Store) RecordDeposit(ctx context.Context, investorID string, amount float64, notes string, masterPortfolioEquity float64) (*CapitalTransaction, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var inv Investor
	err = tx.QueryRow(ctx, `SELECT id, status FROM investors WHERE id = $1 FOR UPDATE`, investorID).Scan(&inv.ID, &inv.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvestorNotFound
		}
		return nil, err
	}

	var totalPoolUnits float64
	err = tx.QueryRow(ctx, `SELECT COALESCE(SUM(pool_units), 0) FROM investors WHERE status = 'ACTIVE'`).Scan(&totalPoolUnits)
	if err != nil {
		return nil, err
	}

	nav := CalculateNAV(masterPortfolioEquity, totalPoolUnits)
	unitsIssued := amount / nav

	var txRecord CapitalTransaction
	err = tx.QueryRow(ctx, `
		INSERT INTO investor_transactions (investor_id, tx_type, amount, pool_units, nav_at_execution, notes)
		VALUES ($1, 'DEPOSIT', $2, $3, $4, $5)
		RETURNING id, investor_id, tx_type, amount, pool_units, nav_at_execution, notes, created_at
	`, investorID, amount, unitsIssued, nav, notes).Scan(
		&txRecord.ID, &txRecord.InvestorID, &txRecord.TxType, &txRecord.Amount, &txRecord.PoolUnits, &txRecord.NAVAtExecution, &txRecord.Notes, &txRecord.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to record transaction: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE investors
		SET total_deposited = total_deposited + $1,
		    pool_units = pool_units + $2,
		    updated_at = NOW()
		WHERE id = $3
	`, amount, unitsIssued, investorID)
	if err != nil {
		return nil, fmt.Errorf("failed to update investor: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &txRecord, nil
}

// RecordWithdrawal processes an investor redemption, burning pool units and validating balance and Tier 1 reserve.
func (s *Store) RecordWithdrawal(ctx context.Context, investorID string, amount float64, notes string, masterPortfolioEquity, tier1CashReserve float64) (*CapitalTransaction, error) {
	if s.Pool == nil {
		return nil, errors.New("database pool not initialized")
	}
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}

	if amount > tier1CashReserve {
		return nil, ErrInsufficientLiquidity
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var inv Investor
	err = tx.QueryRow(ctx, `SELECT id, pool_units, status FROM investors WHERE id = $1 FOR UPDATE`, investorID).Scan(&inv.ID, &inv.PoolUnits, &inv.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvestorNotFound
		}
		return nil, err
	}

	var totalPoolUnits float64
	err = tx.QueryRow(ctx, `SELECT COALESCE(SUM(pool_units), 0) FROM investors WHERE status = 'ACTIVE'`).Scan(&totalPoolUnits)
	if err != nil {
		return nil, err
	}

	nav := CalculateNAV(masterPortfolioEquity, totalPoolUnits)
	unitsToRedeem := amount / nav

	if inv.PoolUnits < unitsToRedeem {
		return nil, ErrInsufficientBalance
	}

	var txRecord CapitalTransaction
	err = tx.QueryRow(ctx, `
		INSERT INTO investor_transactions (investor_id, tx_type, amount, pool_units, nav_at_execution, notes)
		VALUES ($1, 'WITHDRAWAL', $2, $3, $4, $5)
		RETURNING id, investor_id, tx_type, amount, pool_units, nav_at_execution, notes, created_at
	`, investorID, amount, -unitsToRedeem, nav, notes).Scan(
		&txRecord.ID, &txRecord.InvestorID, &txRecord.TxType, &txRecord.Amount, &txRecord.PoolUnits, &txRecord.NAVAtExecution, &txRecord.Notes, &txRecord.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert withdrawal transaction: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE investors
		SET total_withdrawn = total_withdrawn + $1,
		    pool_units = GREATEST(0, pool_units - $2),
		    updated_at = NOW()
		WHERE id = $3
	`, amount, unitsToRedeem, investorID)
	if err != nil {
		return nil, fmt.Errorf("failed to update investor: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &txRecord, nil
}
