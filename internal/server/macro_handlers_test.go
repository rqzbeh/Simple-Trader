package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/server"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

func TestMacroRegimeEndpoints(t *testing.T) {
	cfg := &config.Config{
		AIModelID: "test-model",
	}

	allocCfg := trader.Default3TierConfig(100000.0)
	allocator := trader.NewAllocator(allocCfg)

	srv := server.NewServer(cfg, nil, nil, nil, allocator, nil)
	r := srv.Router()

	// 1. Test GET /api/v1/macro/regime
	req := httptest.NewRequest(http.MethodGet, "/api/v1/macro/regime", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var state trader.MacroRegimeState
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatalf("failed to decode macro regime state: %v", err)
	}

	if state.Score <= 0 || state.Regime == "" {
		t.Fatalf("invalid macro regime state returned: %+v", state)
	}

	// 2. Test POST /api/v1/macro/regime with updated crisis indicators
	crisisPayload := trader.MacroIndicators{
		GeopoliticalIndex: 0.95,
		InflationIndex:    0.85,
		InterestRateIndex: 0.75,
		ActiveConflicts:   []string{"Simulated Major Conflict"},
		InflationRateYoY:  4.2,
		BenchmarkRate:     5.5,
	}

	bodyBytes, _ := json.Marshal(crisisPayload)
	reqPost := httptest.NewRequest(http.MethodPost, "/api/v1/macro/regime", bytes.NewReader(bodyBytes))
	reqPost.Header.Set("Content-Type", "application/json")
	recPost := httptest.NewRecorder()
	r.ServeHTTP(recPost, reqPost)

	if recPost.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recPost.Code)
	}

	var updatedState trader.MacroRegimeState
	if err := json.Unmarshal(recPost.Body.Bytes(), &updatedState); err != nil {
		t.Fatalf("failed to decode updated state: %v", err)
	}

	if updatedState.Regime != trader.RegimeCrisis {
		t.Fatalf("expected regime CRISIS after post, got %s", updatedState.Regime)
	}
	if updatedState.TargetCorePct < 0.55 {
		t.Fatalf("expected core >= 0.55, got %f", updatedState.TargetCorePct)
	}

	// 3. Verify allocator 3-tier targets reflect the updated macro regime
	breakdown := allocator.Get3TierBreakdown()
	if corePct, ok := breakdown["tier2_target_pct"].(float64); !ok || corePct < 0.55 {
		t.Fatalf("expected allocator tier2_target_pct to reflect crisis >= 0.55, got %v", breakdown["tier2_target_pct"])
	}
}
