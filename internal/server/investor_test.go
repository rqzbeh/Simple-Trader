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

func TestInvestorAndAllocatorEndpoints(t *testing.T) {
	cfg := &config.Config{
		AIModelID: "anthropic/claude-3-5-sonnet",
	}
	alloc := trader.NewAllocator(trader.Default3TierConfig(100000.0))
	srv := server.NewServer(cfg, nil, nil, nil, alloc, nil)
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := ts.Client()

	// Test 1: GET /api/v1/allocator/tiers
	resp, err := client.Get(ts.URL + "/api/v1/allocator/tiers")
	if err != nil {
		t.Fatalf("GET /api/v1/allocator/tiers failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var tierData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tierData); err != nil {
		t.Fatalf("failed to decode tiers response: %v", err)
	}

	if tierData["tier1_cash"] != 15000.0 {
		t.Fatalf("expected tier1_cash 15000, got %v", tierData["tier1_cash"])
	}
	if tierData["tier2_core"] != 45000.0 {
		t.Fatalf("expected tier2_core 45000, got %v", tierData["tier2_core"])
	}
	if tierData["tier3_tactical"] != 40000.0 {
		t.Fatalf("expected tier3_tactical 40000, got %v", tierData["tier3_tactical"])
	}

	// Test 2: GET /api/v1/investors (in-memory mode without postgres)
	respInv, err := client.Get(ts.URL + "/api/v1/investors")
	if err != nil {
		t.Fatalf("GET /api/v1/investors failed: %v", err)
	}
	defer respInv.Body.Close()

	if respInv.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", respInv.StatusCode)
	}

	// Test 3: POST /api/v1/investors (mock create)
	createReq := map[string]interface{}{
		"name":            "Alice Capital",
		"contact_tag":     "@alice_trading",
		"notes":           "Strategic LP",
		"initial_deposit": 10000.0,
	}
	body, _ := json.Marshal(createReq)
	respCreate, err := client.Post(ts.URL+"/api/v1/investors", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/v1/investors failed: %v", err)
	}
	defer respCreate.Body.Close()

	if respCreate.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", respCreate.StatusCode)
	}

	// Test 4: POST /api/v1/investors/{id}/withdraw with excessive amount against Tier 1 cash
	withdrawReq := map[string]interface{}{
		"amount": 20000.0, // Exceeds $15,000 cash buffer
		"notes":  "Attempted excessive instant withdrawal",
	}
	wBody, _ := json.Marshal(withdrawReq)
	respWithdraw, err := client.Post(ts.URL+"/api/v1/investors/00000000-0000-0000-0000-000000000001/withdraw", "application/json", bytes.NewReader(wBody))
	if err != nil {
		t.Fatalf("POST /api/v1/investors/{id}/withdraw failed: %v", err)
	}
	defer respWithdraw.Body.Close()

	if respWithdraw.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for excessive withdrawal, got %d", respWithdraw.StatusCode)
	}
}
