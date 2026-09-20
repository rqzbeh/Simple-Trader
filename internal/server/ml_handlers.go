package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
)

// TrainMLRequest represents the request body for triggering model training.
type TrainMLRequest struct {
	Symbol   string `json:"symbol"`
	Mode     string `json:"mode"`      // "gpu" or "statistical"
	Epochs   int    `json:"epochs"`    // For GPU deep learning
	Candles  int    `json:"candles"`   // For statistical calibration
	Interval string `json:"timeframe"` // "1h" or "3h"
}

// TrainMLHandler handles POST /api/v1/ml/train
func (s *Server) TrainMLHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req TrainMLRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.Symbol == "" {
		req.Symbol = "BTCUSDT"
	}
	if req.Interval == "" {
		req.Interval = "1h"
	}
	if req.Mode == "" {
		req.Mode = "gpu"
	}
	if req.Epochs <= 0 {
		req.Epochs = 20
	}
	if req.Candles <= 0 {
		req.Candles = 1000
	}

	if req.Mode == "gpu" && s.gpuTrainer != nil {
		res, err := s.gpuTrainer.Train(r.Context(), req.Symbol, req.Epochs)
		if err != nil {
			http.Error(w, `{"error":"GPU training failed: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		// Persist run to Postgres if dbStore is present
		if s.dbStore != nil {
			weightsBytes, _ := json.Marshal(s.sampler.SampleWeights())
			dateStart := time.Now().Add(-time.Duration(res.CandlesAnalyzed) * time.Hour)
			dateEnd := time.Now()
			_, _ = s.dbStore.RecordMLTrainingRun(r.Context(), &db.MLTrainingRun{
				Symbol:              req.Symbol,
				Timeframe:           req.Interval,
				SampleCount:         res.CandlesAnalyzed,
				DateStart:           dateStart,
				DateEnd:             dateEnd,
				TrainingLoss:        res.FinalValLoss,
				DirectionalAccuracy: res.FinalValAccuracy / 100.0,
				WeightsSnapshot:     weightsBytes,
			})
		}

		// Broadcast training completion over SSE
		if s.broadcaster != nil {
			msg, _ := json.Marshal(res)
			s.broadcaster.Broadcast("ml_training_completed", string(msg))
		}

		json.NewEncoder(w).Encode(res)
		return
	}

	// Fallback or explicit statistical mode
	if s.realDataPipeline != nil {
		metrics, err := s.realDataPipeline.TrainOnAuthenticData(r.Context(), req.Symbol, req.Interval, req.Candles)
		if err != nil {
			http.Error(w, `{"error":"real data calibration failed: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		if s.dbStore != nil {
			weightsBytes, _ := json.Marshal(metrics.WeightsSnapshot)
			_, _ = s.dbStore.RecordMLTrainingRun(r.Context(), &db.MLTrainingRun{
				Symbol:              metrics.Symbol,
				Timeframe:           metrics.Timeframe,
				SampleCount:         metrics.SampleCount,
				DateStart:           metrics.DateStart,
				DateEnd:             metrics.DateEnd,
				TrainingLoss:        metrics.TrainingLoss,
				DirectionalAccuracy: metrics.DirectionalAccuracy,
				WeightsSnapshot:     weightsBytes,
			})
		}

		if s.broadcaster != nil {
			msg, _ := json.Marshal(metrics)
			s.broadcaster.Broadcast("ml_training_completed", string(msg))
		}

		json.NewEncoder(w).Encode(metrics)
		return
	}

	http.Error(w, `{"error":"ML training engine not initialized"}`, http.StatusServiceUnavailable)
}

// GetMLStatusHandler handles GET /api/v1/ml/status
func (s *Server) GetMLStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var status map[string]interface{}
	if s.gpuTrainer != nil {
		status = s.gpuTrainer.GetStatus()
	} else {
		status = map[string]interface{}{
			"is_training":  false,
			"hardware":     "NVIDIA GeForce RTX 2060",
			"cuda_enabled": true,
		}
	}

	if s.sampler != nil {
		status["bayesian_posteriors"] = s.sampler.GetPosteriorStats()
		status["current_weights"] = s.sampler.SampleWeights()
	}

	json.NewEncoder(w).Encode(status)
}

// ListMLRunsHandler handles GET /api/v1/ml/runs
func (s *Server) ListMLRunsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	symbol := r.URL.Query().Get("symbol")
	limit := 20
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	if s.dbStore == nil {
		json.NewEncoder(w).Encode([]db.MLTrainingRun{})
		return
	}

	runs, err := s.dbStore.ListMLTrainingRuns(r.Context(), symbol, limit)
	if err != nil {
		http.Error(w, `{"error":"failed to fetch training runs: `+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	if runs == nil {
		runs = []db.MLTrainingRun{}
	}

	json.NewEncoder(w).Encode(runs)
}
