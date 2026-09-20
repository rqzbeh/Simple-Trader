package ai_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
)

func TestGPUTrainerDirectExecution(t *testing.T) {
	pythonBin := "/home/redsnow/Simple-Trader/ml_env/bin/python"
	scriptPath := "/home/redsnow/Simple-Trader/ml/train_gpu.py"

	if _, err := os.Stat(pythonBin); err != nil {
		t.Skipf("Skipping GPU test: python binary not found at %s", pythonBin)
	}
	if _, err := os.Stat(scriptPath); err != nil {
		t.Skipf("Skipping GPU test: script not found at %s", scriptPath)
	}

	sampler := ai.NewThompsonSampler(42)
	trainer := ai.NewGPUTrainer(pythonBin, scriptPath, sampler)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Train 5 epochs on authentic BTCUSDT data
	res, err := trainer.Train(ctx, "BTCUSDT", 5)
	if err != nil {
		t.Fatalf("GPU training failed: %v", err)
	}

	if res.Status != "success" {
		t.Errorf("expected status 'success', got %s", res.Status)
	}
	if res.CandlesAnalyzed < 1000 {
		t.Errorf("expected at least 1000 candles analyzed, got %d", res.CandlesAnalyzed)
	}
	if res.PeakVRAMMB <= 0 {
		t.Errorf("expected positive peak VRAM allocation, got %f", res.PeakVRAMMB)
	}
	if res.GPUName == "" {
		t.Errorf("expected GPU name to be reported")
	}

	// Verify Bayesian updates occurred
	stats := sampler.GetPosteriorStats()
	rsiStats := stats["RSI"]
	if rsiStats["alpha"] <= 2.0 && rsiStats["beta"] <= 2.0 {
		t.Errorf("expected Bayesian posteriors to be updated from real data, got alpha=%f beta=%f", rsiStats["alpha"], rsiStats["beta"])
	}

	// Verify status endpoint
	status := trainer.GetStatus()
	if status["is_training"] != false {
		t.Errorf("expected is_training to be false after completion")
	}
}
