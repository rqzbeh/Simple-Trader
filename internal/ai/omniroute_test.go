package ai_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/cache"
)

func TestOmniRouteLiveIntegration(t *testing.T) {
	apiKey := os.Getenv("AI_API_KEY")
	baseURL := os.Getenv("AI_BASE_URL")
	modelID := os.Getenv("AI_MODEL_ID")

	if apiKey == "" || baseURL == "" {
		t.Skip("Skipping live OmniRoute integration test: AI_API_KEY or AI_BASE_URL not set")
	}

	client := ai.NewClient(ai.ClientConfig{
		BaseURL:         baseURL,
		APIKey:          apiKey,
		ModelID:         modelID,
		Temperature:     0.2,
		TimeoutSec:      25,
		ReasoningEffort: "high",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	req := ai.DecisionRequest{
		Symbol: "XAU/USD",
		Bucket: "CORE",
		Quote: cache.TickerQuote{
			Symbol:    "XAU/USD",
			Price:     2650.0,
			Change24h: 1.25,
		},
		IndicatorSnap: cache.IndicatorSnapshot{
			Symbol:          "XAU/USD",
			RSI:             62.5,
			SuperTrend:      "BULL",
			Histogram:       1.8,
			ConfluenceScore: 0.85,
		},
		Weights: map[string]float64{
			"SUPERTREND": 1.4,
			"RSI":        1.2,
			"MACD":       1.3,
		},
		NewsHeadlines: []string{
			"Global central banks increase bullion gold reserves by 14% this quarter.",
			"Treasury yields drift lower following dovish inflation telemetry.",
		},
	}

	decision, err := client.Analyze(ctx, req)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if decision.Decision != "BUY" && decision.Decision != "SELL" && decision.Decision != "HOLD" {
		t.Errorf("Unexpected decision: %s", decision.Decision)
	}
	if decision.Confidence <= 0 || decision.Confidence > 1.0 {
		t.Errorf("Confidence out of range: %f", decision.Confidence)
	}
	if decision.Reasoning == "" {
		t.Errorf("Expected reasoning text from AI model, got empty")
	}

	t.Logf("OmniRoute Response: Decision=%s Confidence=%.2f WinProb=%.2f Regime=%s SL=%.2f%% TP=%.2f%%\nReasoning: %s",
		decision.Decision, decision.Confidence, decision.EstimatedWinProbability, decision.Regime,
		decision.SuggestedStopLossPct, decision.SuggestedTakeProfitPct, decision.Reasoning)
}
