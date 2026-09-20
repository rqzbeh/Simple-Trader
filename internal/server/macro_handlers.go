package server

import (
	"encoding/json"
	"net/http"

	"github.com/rqzbeh/simple-trader/internal/trader"
)

// GetMacroRegimeHandler handles GET /api/v1/macro/regime
func (s *Server) GetMacroRegimeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.allocator == nil || s.allocator.GetMacroEngine() == nil {
		// Return standard default regime state if allocator is unconfigured
		defaultEngine := trader.NewMacroRegimeEngine()
		json.NewEncoder(w).Encode(defaultEngine.GetCurrentState())
		return
	}

	state := s.allocator.GetMacroEngine().GetCurrentState()
	json.NewEncoder(w).Encode(state)
}

// UpdateMacroRegimeHandler handles POST /api/v1/macro/regime/update
func (s *Server) UpdateMacroRegimeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.allocator == nil {
		http.Error(w, `{"error":"allocator uninitialized"}`, http.StatusServiceUnavailable)
		return
	}

	var req trader.MacroIndicators
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid macro indicators: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	newState := s.allocator.ApplyMacroRegime(req)

	// Broadcast updated macro regime via SSE
	if s.broadcaster != nil {
		if bytes, err := json.Marshal(newState); err == nil {
			s.broadcaster.Broadcast("macro_regime_updated", string(bytes))
		}
	}

	json.NewEncoder(w).Encode(newState)
}
