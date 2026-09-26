package market

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// oideltaResponse is one row of GET /fapi/v1/openInterestHist.
type oideltaResponse struct {
	Symbol               string `json:"symbol"`
	SumOpenInterest      string `json:"sumOpenInterest"`
	SumOpenInterestValue string `json:"sumOpenInterestValue"`
	Timestamp            int64  `json:"timestamp"`
}

// FetchOIDeltaPct returns the percentage change of futures open interest over
// the last few `period` candles (default 5m x 3 intervals ≈ 15 minutes), used
// by the entry gate OI-trap rule (spec 012 US1, research R1).
//
// Contract: bounded to ~500ms via context; on any failure returns (nil, err)
// and callers treat nil as "unknown" — the gate then skips the OI rule
// instead of blocking entries.
func (b *BinanceFetcher) FetchOIDeltaPct(ctx context.Context, symbol string) (*float64, error) {
	clean := strings.NewReplacer("/", "", "-", "", "_", "").Replace(symbol)

	oiCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	url := fmt.Sprintf("%s/fapi/v1/openInterestHist?symbol=%s&period=5m&limit=3", b.futuresBaseURL, clean)
	req, err := http.NewRequestWithContext(oiCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create open interest request: %w", err)
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open interest request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open interest endpoint returned %d", resp.StatusCode)
	}

	var rows []oideltaResponse
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("failed to decode open interest history: %w", err)
	}
	delta, ok := computeOIDelta(rows)
	if !ok {
		return nil, fmt.Errorf("insufficient open interest history (%d rows)", len(rows))
	}
	return &delta, nil
}

// computeOIDelta derives (last - first) / first * 100 from ordered OI rows.
// Exported for tests via the package-level helper below.
func computeOIDelta(rows []oideltaResponse) (float64, bool) {
	if len(rows) < 2 {
		return 0, false
	}
	first := parseFloat(rows[0].SumOpenInterest)
	last := parseFloat(rows[len(rows)-1].SumOpenInterest)
	if first <= 0 {
		return 0, false
	}
	return (last - first) / first * 100.0, true
}

func parseFloat(s string) float64 {
	var v float64
	if _, err := fmt.Sscanf(s, "%g", &v); err != nil {
		return 0
	}
	return v
}
