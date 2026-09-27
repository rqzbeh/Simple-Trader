package server

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
)

type SystemStatsResponse struct {
	Version          string         `json:"version"`
	UptimeSeconds    int64          `json:"uptime_seconds"`
	RoutingThreshold *float64       `json:"routing_threshold,omitempty"`
	Gateway          StatsComponent `json:"gateway"`
	Jev              JevComponent   `json:"jev"`
}

type StatsComponent struct {
	Total         int64   `json:"total"`
	Success       int64   `json:"success"`
	Fail          int64   `json:"fail"`
	SuccessRate   float64 `json:"success_rate"`
	EMALatencyMs  float64 `json:"ema_latency_ms"`
	LastLatencyMs float64 `json:"last_latency_ms"`
	LastError     string  `json:"last_error"`
	LastOKAt      *string `json:"last_ok_at"`
}

type JevComponent struct {
	StatsComponent
	Model         string `json:"model"`
	KeyConfigured bool   `json:"key_configured"`
}

func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	uptime := int64(0)
	if !s.startTime.IsZero() {
		uptime = int64(time.Since(s.startTime).Seconds())
	}

	gw := ai.GetGatewayStats()
	var gwRate float64
	if gw.Total > 0 {
		gwRate = math.Round((float64(gw.Success)/float64(gw.Total))*10000) / 10000
	}
	var gwLastOK *string
	if !gw.LastOK.IsZero() {
		tStr := gw.LastOK.UTC().Format(time.RFC3339)
		gwLastOK = &tStr
	}
	gwErr := gw.LastError
	if len(gwErr) > 200 {
		gwErr = gwErr[:200]
	}

	jev := ai.GetJevStats()
	var jevRate float64
	if jev.Total > 0 {
		jevRate = math.Round((float64(jev.Success)/float64(jev.Total))*10000) / 10000
	}
	var jevLastOK *string
	if !jev.LastOK.IsZero() {
		tStr := jev.LastOK.UTC().Format(time.RFC3339)
		jevLastOK = &tStr
	}
	jevErr := jev.LastError
	if len(jevErr) > 200 {
		jevErr = jevErr[:200]
	}

	jevModel := "jev-latest"
	if s.decisionRouter != nil && s.decisionRouter.Jev != nil {
		jevModel = s.decisionRouter.Jev.Model()
	}

	keyConfigured := os.Getenv("TYPESAFE_API_KEY") != ""

	var routingThreshold *float64
	if s.cfg != nil {
		if thr, err := s.cfg.RoutingThreshold(); err == nil {
			routingThreshold = &thr
		}
	}

	resp := SystemStatsResponse{
		Version:          "3.0.0-decision-core",
		UptimeSeconds:    uptime,
		RoutingThreshold: routingThreshold,
		Gateway: StatsComponent{
			Total:         gw.Total,
			Success:       gw.Success,
			Fail:          gw.Fail,
			SuccessRate:   gwRate,
			EMALatencyMs:  math.Round(gw.EMA_LatencyMs*10) / 10,
			LastLatencyMs: math.Round(gw.LastLatencyMs*10) / 10,
			LastError:     gwErr,
			LastOKAt:      gwLastOK,
		},
		Jev: JevComponent{
			StatsComponent: StatsComponent{
				Total:         jev.Total,
				Success:       jev.Success,
				Fail:          jev.Fail,
				SuccessRate:   jevRate,
				EMALatencyMs:  math.Round(jev.EMA_LatencyMs*10) / 10,
				LastLatencyMs: math.Round(jev.LastLatencyMs*10) / 10,
				LastError:     jevErr,
				LastOKAt:      jevLastOK,
			},
			Model:         jevModel,
			KeyConfigured: keyConfigured,
		},
	}

	_ = json.NewEncoder(w).Encode(resp)
}
