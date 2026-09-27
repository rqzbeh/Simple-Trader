package ai

import (
	"sync"
	"sync/atomic"
	"time"
)

// GatewayStats holds operational counters and latency metrics for the 9Router / OpenAI gateway.
type GatewayStats struct {
	Total         int64
	Success       int64
	Fail          int64
	LastLatencyMs float64
	EMA_LatencyMs float64
	LastError     string
	LastOK        time.Time
}

// JevStats holds operational counters and latency metrics for the TypeSafe Jev System One service.
type JevStats struct {
	Total         int64
	Success       int64
	Fail          int64
	LastLatencyMs float64
	EMA_LatencyMs float64
	LastError     string
	LastOK        time.Time
}

var (
	gwTotal   int64
	gwSuccess int64
	gwFail    int64
	gwMu      sync.RWMutex
	gwLastLat float64
	gwEMALat  float64
	gwLastErr string
	gwLastOK  time.Time

	jevTotal   int64
	jevSuccess int64
	jevFail    int64
	jevMu      sync.RWMutex
	jevLastLat float64
	jevEMALat  float64
	jevLastErr string
	jevLastOK  time.Time
)

// RecordGateway updates the gateway stats. Errors are masked to a maximum of 200 characters.
func RecordGateway(ok bool, latencyMs float64, errMsg string) {
	if ok {
		atomic.AddInt64(&gwSuccess, 1)
	} else {
		atomic.AddInt64(&gwFail, 1)
	}
	atomic.AddInt64(&gwTotal, 1)

	if len(errMsg) > 200 {
		errMsg = errMsg[:200]
	}

	gwMu.Lock()
	defer gwMu.Unlock()
	gwLastLat = latencyMs
	if latencyMs > 0 {
		if gwEMALat <= 0 {
			gwEMALat = latencyMs
		} else {
			const alpha = 0.2
			gwEMALat = alpha*latencyMs + (1.0-alpha)*gwEMALat
		}
	}
	if ok {
		gwLastOK = time.Now()
	} else {
		gwLastErr = errMsg
	}
}

// GetGatewayStats returns a point-in-time snapshot of the gateway statistics.
func GetGatewayStats() GatewayStats {
	gwMu.RLock()
	defer gwMu.RUnlock()
	return GatewayStats{
		Total:         atomic.LoadInt64(&gwTotal),
		Success:       atomic.LoadInt64(&gwSuccess),
		Fail:          atomic.LoadInt64(&gwFail),
		LastLatencyMs: gwLastLat,
		EMA_LatencyMs: gwEMALat,
		LastError:     gwLastErr,
		LastOK:        gwLastOK,
	}
}

// RecordJev updates the Jev System One stats. Errors are masked to a maximum of 200 characters.
func RecordJev(ok bool, latencyMs float64, errMsg string) {
	if ok {
		atomic.AddInt64(&jevSuccess, 1)
	} else {
		atomic.AddInt64(&jevFail, 1)
	}
	atomic.AddInt64(&jevTotal, 1)

	if len(errMsg) > 200 {
		errMsg = errMsg[:200]
	}

	jevMu.Lock()
	defer jevMu.Unlock()
	jevLastLat = latencyMs
	if latencyMs > 0 {
		if jevEMALat <= 0 {
			jevEMALat = latencyMs
		} else {
			const alpha = 0.2
			jevEMALat = alpha*latencyMs + (1.0-alpha)*jevEMALat
		}
	}
	if ok {
		jevLastOK = time.Now()
	} else {
		jevLastErr = errMsg
	}
}

// GetJevStats returns a point-in-time snapshot of the Jev System One statistics.
func GetJevStats() JevStats {
	jevMu.RLock()
	defer jevMu.RUnlock()
	return JevStats{
		Total:         atomic.LoadInt64(&jevTotal),
		Success:       atomic.LoadInt64(&jevSuccess),
		Fail:          atomic.LoadInt64(&jevFail),
		LastLatencyMs: jevLastLat,
		EMA_LatencyMs: jevEMALat,
		LastError:     jevLastErr,
		LastOK:        jevLastOK,
	}
}

// ResetStatsForTest resets all statistics for isolated testing.
func ResetStatsForTest() {
	atomic.StoreInt64(&gwTotal, 0)
	atomic.StoreInt64(&gwSuccess, 0)
	atomic.StoreInt64(&gwFail, 0)
	gwMu.Lock()
	gwLastLat = 0
	gwEMALat = 0
	gwLastErr = ""
	gwLastOK = time.Time{}
	gwMu.Unlock()

	atomic.StoreInt64(&jevTotal, 0)
	atomic.StoreInt64(&jevSuccess, 0)
	atomic.StoreInt64(&jevFail, 0)
	jevMu.Lock()
	jevLastLat = 0
	jevEMALat = 0
	jevLastErr = ""
	jevLastOK = time.Time{}
	jevMu.Unlock()
}
