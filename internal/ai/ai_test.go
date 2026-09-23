package ai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
)

func TestOpenAIClientDecision(t *testing.T) {
	// Mock OpenAI compatible /v1/chat/completions endpoint
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions endpoint, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("expected Bearer token test-api-key, got %s", r.Header.Get("Authorization"))
		}

		resp := map[string]interface{}{
			"id": "chatcmpl-test",
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role": "assistant",
						"content": `{
							"decision": "BUY",
							"confidence": 0.85,
							"reasoning": "Strong bullish momentum confirmed by SuperTrend and MACD positive divergence.",
							"suggested_stop_loss_pct": 1.5,
							"suggested_take_profit_pct": 3.0
						}`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	client := ai.NewClient(ai.ClientConfig{
		BaseURL:     mockServer.URL,
		APIKey:      "test-api-key",
		ModelID:     "test-quant-model",
		Temperature: 0.2,
		TimeoutSec:  5,
	})

	ctx := context.Background()
	req := ai.DecisionRequest{
		Symbol: "BTC/USD",
		Bucket: "ALPHA",
		Quote: cache.TickerQuote{
			Symbol: "BTC/USD",
			Price:  68200.0,
		},
		IndicatorSnap: cache.IndicatorSnapshot{
			Symbol:          "BTC/USD",
			RSI:             62.0,
			SuperTrend:      "BULL",
			ConfluenceScore: 0.8,
		},
		Weights: map[string]float64{
			"RSI":        1.2,
			"MACD":       1.5,
			"SUPERTREND": 1.4,
		},
	}

	decision, err := client.Analyze(ctx, req)
	if err != nil {
		t.Fatalf("unexpected analyze error: %v", err)
	}

	if decision.Decision != "BUY" {
		t.Errorf("expected BUY decision, got %s", decision.Decision)
	}
	if decision.Confidence != 0.85 {
		t.Errorf("expected confidence 0.85, got %f", decision.Confidence)
	}
}

func TestDynamicWeightAdjustment(t *testing.T) {
	engine := ai.NewWeightEngine()

	initialWeights := map[string]float64{
		"RSI":        1.0,
		"MACD":       1.0,
		"SUPERTREND": 1.0,
		"BOLLINGER":  1.0,
	}

	// 1. Profitable trade with BUY on Bullish SuperTrend & positive MACD
	// Expected: Reward matching indicators, slight boost
	rewarded := engine.AdjustWeights(initialWeights, ai.TradeOutcome{
		Pnl:             150.0,
		ReturnPct:       3.2,
		Side:            "BUY",
		SuperTrendTrend: "BULL",
		RSI:             65.0,
		MACDHistogram:   1.2,
	})

	if rewarded["SUPERTREND"] <= 1.0 {
		t.Errorf("expected SUPERTREND to be rewarded (>1.0), got %f", rewarded["SUPERTREND"])
	}
	if rewarded["MACD"] <= 1.0 {
		t.Errorf("expected MACD to be rewarded (>1.0), got %f", rewarded["MACD"])
	}

	// 2. Losing trade (regret penalty)
	// Long trade lost money while RSI was overbought (>75)
	penalized := engine.AdjustWeights(initialWeights, ai.TradeOutcome{
		Pnl:             -200.0,
		ReturnPct:       -4.0,
		Side:            "BUY",
		SuperTrendTrend: "BULL",
		RSI:             78.0,
		MACDHistogram:   -0.5,
	})

	// Indicators that provided false bullish signal should receive regret penalties
	if penalized["RSI"] >= 1.0 {
		t.Errorf("expected RSI to receive regret penalty (<1.0), got %f", penalized["RSI"])
	}

	// Bounds checking: weights must never exceed [0.2, 3.0]
	for k, w := range penalized {
		if w < 0.2 || w > 3.0 {
			t.Errorf("weight %s out of bounds: %f", k, w)
		}
	}
}

func TestFineTuningJSONLExporter(t *testing.T) {
	record := ai.FineTunePair{
		SystemPrompt: "You are a professional quantitative trader.",
		UserPrompt:   "Analyze BTC/USD at 68000 with RSI=65 and SuperTrend=BULL.",
		AssistantResponse: `{
			"decision": "BUY",
			"confidence": 0.88,
			"reasoning": "High momentum trade supported by confluence."
		}`,
		Metadata: map[string]interface{}{
			"symbol": "BTC/USD",
			"pnl":    340.5,
		},
	}

	line, err := record.ToJSONL()
	if err != nil {
		t.Fatalf("failed to serialize fine-tune record: %v", err)
	}

	if !strings.Contains(line, "messages") || !strings.Contains(line, "system") || !strings.Contains(line, "assistant") {
		t.Errorf("invalid ChatML JSONL format: %s", line)
	}
}
