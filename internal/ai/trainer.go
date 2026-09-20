package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// GPUTrainingResult holds metrics returned by the GPU training script.
type GPUTrainingResult struct {
	Status             string                     `json:"status"`
	Symbol             string                     `json:"symbol"`
	Device             string                     `json:"device"`
	GPUName            string                     `json:"gpu_name"`
	CUDAVersion        string                     `json:"cuda_version"`
	PeakVRAMMB         float64                    `json:"peak_vram_mb"`
	CandlesAnalyzed    int                        `json:"candles_analyzed"`
	Epochs             int                        `json:"epochs"`
	BatchSize          int                        `json:"batch_size"`
	DurationSeconds    float64                    `json:"duration_seconds"`
	FinalTrainAccuracy float64                    `json:"final_train_accuracy"`
	FinalValAccuracy   float64                    `json:"final_val_accuracy"`
	FinalValLoss       float64                    `json:"final_val_loss"`
	ModelPath          string                     `json:"model_path"`
	History            []EpochMetric              `json:"history"`
	BayesianPosteriors map[string]map[string]float64 `json:"bayesian_posteriors"`
	TrainedAt          time.Time                  `json:"trained_at"`
}

// EpochMetric holds per-epoch training metrics.
type EpochMetric struct {
	Epoch     int     `json:"epoch"`
	TrainLoss float64 `json:"train_loss"`
	TrainAcc  float64 `json:"train_acc"`
	ValLoss   float64 `json:"val_loss"`
	ValAcc    float64 `json:"val_acc"`
	VRAMMB    float64 `json:"vram_mb"`
}

// GPUTrainer manages executing real-data GPU model training on the local RTX 2060.
type GPUTrainer struct {
	mu           sync.RWMutex
	pythonBin    string
	scriptPath   string
	sampler      *ThompsonSampler
	lastResult   *GPUTrainingResult
	isTraining   bool
}

// NewGPUTrainer creates a new GPU training manager.
func NewGPUTrainer(pythonBin, scriptPath string, sampler *ThompsonSampler) *GPUTrainer {
	if pythonBin == "" {
		pythonBin = "/home/redsnow/Simple-Trader/ml_env/bin/python"
	}
	if scriptPath == "" {
		scriptPath = "/home/redsnow/Simple-Trader/ml/train_gpu.py"
	}
	return &GPUTrainer{
		pythonBin:  pythonBin,
		scriptPath: scriptPath,
		sampler:    sampler,
	}
}

// Train runs the real-data GPU training script and updates Bayesian posteriors.
func (t *GPUTrainer) Train(ctx context.Context, symbol string, epochs int) (*GPUTrainingResult, error) {
	t.mu.Lock()
	if t.isTraining {
		t.mu.Unlock()
		return nil, fmt.Errorf("a GPU training process is already running")
	}
	t.isTraining = true
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		t.isTraining = false
		t.mu.Unlock()
	}()

	if symbol == "" {
		symbol = "BTCUSDT"
	}
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	symbol = strings.ReplaceAll(symbol, "/", "")
	symbol = strings.ReplaceAll(symbol, "-", "")
	symbol = strings.ReplaceAll(symbol, "_", "")
	if strings.HasSuffix(symbol, "USD") && !strings.HasSuffix(symbol, "USDT") {
		symbol = symbol + "T"
	}
	if epochs <= 0 {
		epochs = 20
	}
	if epochs > 50 {
		epochs = 50
	}

	cmd := exec.CommandContext(ctx, t.pythonBin, t.scriptPath,
		"--symbol", symbol,
		"--epochs", fmt.Sprintf("%d", epochs),
		"--json",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("GPU training failed: %w (stderr: %s)", err, stderr.String())
	}

	outStr := stdout.String()
	startTag := "===JSON_START==="
	endTag := "===JSON_END==="

	startIdx := strings.Index(outStr, startTag)
	endIdx := strings.Index(outStr, endTag)

	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		return nil, fmt.Errorf("failed to parse GPU training JSON payload from output: %s", outStr)
	}

	jsonPayload := strings.TrimSpace(outStr[startIdx+len(startTag) : endIdx])

	var res GPUTrainingResult
	if err := json.Unmarshal([]byte(jsonPayload), &res); err != nil {
		return nil, fmt.Errorf("failed to decode training result JSON: %w", err)
	}

	res.TrainedAt = time.Now()

	// Update Bayesian sampler with real market data results
	if t.sampler != nil && len(res.BayesianPosteriors) > 0 {
		updates := make(map[string]struct{ Alpha, Beta float64 })
		for ind, params := range res.BayesianPosteriors {
			alpha, hasAlpha := params["alpha"]
			beta, hasBeta := params["beta"]
			if hasAlpha && hasBeta {
				updates[ind] = struct{ Alpha, Beta float64 }{
					Alpha: alpha,
					Beta:  beta,
				}
			}
		}
		t.sampler.UpdatePosteriors(updates)
	}

	t.mu.Lock()
	t.lastResult = &res
	t.mu.Unlock()

	return &res, nil
}

// GetStatus returns the current status and latest training metrics.
func (t *GPUTrainer) GetStatus() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	status := map[string]interface{}{
		"is_training":  t.isTraining,
		"hardware":     "NVIDIA GeForce RTX 2060",
		"cuda_enabled": true,
		"last_result":  t.lastResult,
	}

	if t.sampler != nil {
		status["bayesian_posteriors"] = t.sampler.GetPosteriorStats()
		status["current_weights"] = t.sampler.SampleWeights()
	}

	return status
}
