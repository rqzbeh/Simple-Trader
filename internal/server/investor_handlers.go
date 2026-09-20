package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rqzbeh/simple-trader/internal/db"
)

// getTotalPortfolioEquity returns the current master equity across engines and allocators.
func (s *Server) getTotalPortfolioEquity() float64 {
	if s.execEngine != nil {
		return s.execEngine.GetTotalEquity()
	}
	if s.allocator != nil {
		return s.allocator.GetTotalCapital()
	}
	return 100000.0
}

// getTier1CashReserve returns the unencumbered cash reserve buffer.
func (s *Server) getTier1CashReserve() float64 {
	if s.allocator != nil {
		return s.allocator.GetTier1CashReserve()
	}
	return 15000.0
}

// ListInvestorsHandler returns all registered investors with dynamic pro-rata metrics.
func (s *Server) ListInvestorsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	totalEquity := s.getTotalPortfolioEquity()
	if s.dbStore == nil {
		json.NewEncoder(w).Encode([]db.Investor{})
		return
	}

	investors, err := s.dbStore.ListInvestors(r.Context(), totalEquity)
	if err != nil {
		http.Error(w, `{"error":"failed to fetch investors"}`, http.StatusInternalServerError)
		return
	}

	if investors == nil {
		investors = []db.Investor{}
	}
	json.NewEncoder(w).Encode(investors)
}

// CreateInvestorHandler registers a new investor with an optional initial deposit.
func (s *Server) CreateInvestorHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		Name           string  `json:"name"`
		ContactTag     string  `json:"contact_tag"`
		Notes          string  `json:"notes"`
		InitialDeposit float64 `json:"initial_deposit"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.ContactTag == "" {
		http.Error(w, `{"error":"name and contact_tag are required"}`, http.StatusBadRequest)
		return
	}

	if s.dbStore == nil {
		// Mock response for in-memory mode without PostgreSQL
		inv := db.Investor{
			ID:             "00000000-0000-0000-0000-000000000001",
			Name:           req.Name,
			ContactTag:     req.ContactTag,
			Notes:          req.Notes,
			TotalDeposited: req.InitialDeposit,
			PoolUnits:      req.InitialDeposit,
			Status:         "ACTIVE",
			CurrentEquity:  req.InitialDeposit,
			ROI:            0.0,
			PoolSharePct:   100.0,
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(inv)
		return
	}

	totalEquity := s.getTotalPortfolioEquity()
	inv, err := s.dbStore.CreateInvestor(r.Context(), req.Name, req.ContactTag, req.Notes, req.InitialDeposit, totalEquity)
	if err != nil {
		http.Error(w, `{"error":"failed to create investor: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(inv)
}

// GetInvestorHandler returns a single investor by ID along with their capital transaction history.
func (s *Server) GetInvestorHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	if s.dbStore == nil {
		http.Error(w, `{"error":"investor not found"}`, http.StatusNotFound)
		return
	}

	totalEquity := s.getTotalPortfolioEquity()
	inv, txs, err := s.dbStore.GetInvestor(r.Context(), id, totalEquity)
	if err != nil {
		if errors.Is(err, db.ErrInvestorNotFound) {
			http.Error(w, `{"error":"investor not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to get investor"}`, http.StatusInternalServerError)
		return
	}

	if txs == nil {
		txs = []db.CapitalTransaction{}
	}

	resp := struct {
		*db.Investor
		Transactions []db.CapitalTransaction `json:"transactions"`
	}{
		Investor:     inv,
		Transactions: txs,
	}

	json.NewEncoder(w).Encode(resp)
}

// RecordDepositHandler records an additional capital contribution.
func (s *Server) RecordDepositHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Amount float64 `json:"amount"`
		Notes  string  `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Amount <= 0 {
		http.Error(w, `{"error":"valid positive amount is required"}`, http.StatusBadRequest)
		return
	}

	if s.dbStore == nil {
		http.Error(w, `{"error":"database unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	totalEquity := s.getTotalPortfolioEquity()
	tx, err := s.dbStore.RecordDeposit(r.Context(), id, req.Amount, req.Notes, totalEquity)
	if err != nil {
		if errors.Is(err, db.ErrInvestorNotFound) {
			http.Error(w, `{"error":"investor not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"failed to process deposit: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	// Update allocator total capital if available
	if s.allocator != nil {
		s.allocator.AddCapital(req.Amount)
	}
	json.NewEncoder(w).Encode(tx)
}

// RecordWithdrawalHandler processes an investor withdrawal settled against Tier 1 cash.
func (s *Server) RecordWithdrawalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Amount float64 `json:"amount"`
		Notes  string  `json:"notes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Amount <= 0 {
		http.Error(w, `{"error":"valid positive amount is required"}`, http.StatusBadRequest)
		return
	}

	tier1Cash := s.getTier1CashReserve()
	if req.Amount > tier1Cash {
		http.Error(w, `{"error":"insufficient Tier 1 liquidity reserve for instant withdrawal"}`, http.StatusBadRequest)
		return
	}

	if s.dbStore == nil {
		http.Error(w, `{"error":"database unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	totalEquity := s.getTotalPortfolioEquity()
	tx, err := s.dbStore.RecordWithdrawal(r.Context(), id, req.Amount, req.Notes, totalEquity, tier1Cash)
	if err != nil {
		if errors.Is(err, db.ErrInvestorNotFound) {
			http.Error(w, `{"error":"investor not found"}`, http.StatusNotFound)
			return
		}
		if errors.Is(err, db.ErrInsufficientBalance) {
			http.Error(w, `{"error":"withdrawal amount exceeds investor allocated equity"}`, http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error":"withdrawal rejected: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	// Deduct from Tier 1 liquid cash
	if s.allocator != nil {
		s.allocator.DebitTier1Cash(req.Amount)
	}
	json.NewEncoder(w).Encode(tx)
}
