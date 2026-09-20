package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/market"
)

func TestRealDataPipelineRejectsSyntheticData(t *testing.T) {
	sampler := ai.NewThompsonSampler(42)
	pipeline := ai.NewRealDataPipeline(nil, sampler)

	// Pipeline requires minimum 50 candles
	_, err := pipeline.TrainOnAuthenticData(context.Background(), "BTCUSDT", "1h", 20)
	if err == nil {
		t.Errorf("expected error when requesting fewer than 50 authentic candles")
	}
}

func TestRealDataPipelineWithMockBinanceHistory(t *testing.T) {
	baseTime := int64(1710000000000)
	basePrice := 65000.0

	rows := make([][]interface{}, 60)
	for i := 0; i < 60; i++ {
		open := basePrice + float64(i)*10.0
		high := open + 25.0
		low := open - 15.0
		closeVal := open + 12.0
		vol := 150.0
		taker := 80.0
		ts := float64(baseTime + int64(i*3600000))
		closeTs := float64(int64(ts) + 3599999)

		rows[i] = []interface{}{
			ts,
			fmt.Sprintf("%.2f", open),
			fmt.Sprintf("%.2f", high),
			fmt.Sprintf("%.2f", low),
			fmt.Sprintf("%.2f", closeVal),
			fmt.Sprintf("%.2f", vol),
			closeTs,
			"1000000",
			150,
			fmt.Sprintf("%.2f", taker),
			"500000",
			"0",
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rows)
	}))
	defer server.Close()

	downloader := market.NewBinanceHistoricalDownloaderWithBaseURL(server.URL)
	sampler := ai.NewThompsonSampler(42)
	pipeline := ai.NewRealDataPipeline(downloader, sampler)

	metrics, err := pipeline.TrainOnAuthenticData(context.Background(), "BTCUSDT", "1h", 60)
	if err != nil {
		t.Fatalf("training on authentic format data failed: %v", err)
	}

	if metrics.SampleCount != 60 {
		t.Errorf("expected 60 samples analyzed, got %d", metrics.SampleCount)
	}
	if metrics.DirectionalAccuracy <= 0 || metrics.DirectionalAccuracy > 1.0 {
		t.Errorf("unexpected accuracy: %f", metrics.DirectionalAccuracy)
	}
	if len(metrics.WeightsSnapshot) == 0 {
		t.Errorf("expected non-empty weights snapshot")
	}
}
