package trader

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
)

func jevServer(t *testing.T, conf float64, choice string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		other := "SHORT"
		if choice == "SHORT" {
			other = "LONG"
		}
		resp := map[string]interface{}{
			"model": "jev-test",
			"answers": map[string]interface{}{
				// spec-018: production asks `direction` (relative Choice,
				// no NO_TRADE attractor) — fixtures follow the contract.
				"direction": map[string]interface{}{
					"type": "choice", "choice": choice, "confidence": conf,
					"probabilities": map[string]float64{choice: conf, other: 1 - conf},
				},
			},
			"usage": map[string]int{"input_tokens": 100, "output_tokens": 10},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestRouterRequiresThreshold(t *testing.T) {
	r := &DecisionRouter{Threshold: 0}
	_, err := r.Route(context.Background(), "c1", nil, nil, nil)
	if !errors.Is(err, ai.ErrConfigMissing) {
		t.Fatalf("expected ErrConfigMissing, got %v", err)
	}
}

func TestRouterJevDirectAboveThreshold(t *testing.T) {
	srv := jevServer(t, 0.9, "LONG")
	defer srv.Close()
	escalated := false
	r := &DecisionRouter{
		Jev:       ai.NewJevClient(srv.URL, "k", time.Second),
		Threshold: 0.75,
		Escalate: func(ctx context.Context, state interface{}) (DecisionOutcome, error) {
			escalated = true
			return DecisionOutcome{}, nil
		},
	}
	out, err := r.Route(context.Background(), "c1", nil,
		map[string]ai.JevQuestion{"direction": {Type: "choice"}}, directionVocab)
	if err != nil {
		t.Fatal(err)
	}
	if out.Route != "jev_direct" || out.Choice != "LONG" {
		t.Fatalf("got route=%s choice=%s", out.Route, out.Choice)
	}
	if escalated {
		t.Fatal("escalation must not run above threshold (one final writer)")
	}
}

func TestRouterEscalatesBelowThreshold(t *testing.T) {
	srv := jevServer(t, 0.4, "SHORT")
	defer srv.Close()
	r := &DecisionRouter{
		Jev:       ai.NewJevClient(srv.URL, "k", time.Second),
		Threshold: 0.75,
		Escalate: func(ctx context.Context, state interface{}) (DecisionOutcome, error) {
			return DecisionOutcome{Choice: "LONG", Confidence: 0.8}, nil
		},
	}
	out, err := r.Route(context.Background(), "c1", nil,
		map[string]ai.JevQuestion{"direction": {Type: "choice"}}, directionVocab)
	if err != nil {
		t.Fatal(err)
	}
	if out.Route != "escalated" || out.Choice != "LONG" {
		t.Fatalf("got route=%s choice=%s", out.Route, out.Choice)
	}
	if out.Baseline != "SHORT" {
		t.Fatalf("jev baseline not recorded: %q", out.Baseline)
	}
}

func TestRouterEscalationFailureNotDowngraded(t *testing.T) {
	srv := jevServer(t, 0.4, "SHORT")
	defer srv.Close()
	r := &DecisionRouter{
		Jev:       ai.NewJevClient(srv.URL, "k", time.Second),
		Threshold: 0.75,
		Escalate: func(ctx context.Context, state interface{}) (DecisionOutcome, error) {
			return DecisionOutcome{}, errors.New("9router down")
		},
	}
	_, err := r.Route(context.Background(), "c1", nil,
		map[string]ai.JevQuestion{"direction": {Type: "choice"}}, directionVocab)
	if err == nil {
		t.Fatal("expected explicit escalation failure, got nil")
	}
}
