package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ShadowDecision row (migration 000007_shadow_decisions).
type ShadowDecision struct {
	ID             int64
	CreatedAt      time.Time
	JudgmentType   string // entry | exit | news
	CycleID        string
	Symbol         string
	NewsClusterID  *int64
	StateRef       string
	Judge          string // jev | llm
	Choice         string
	Noul           *float64
	Score          *float64
	Probabilities  map[string]float64
	Confidence     *float64
	BaselineChoice string
	LatencyMS      int
	InputTokens    int
	OutputTokens   int
	Route          string // jev_direct | escalated
	Status         string // ok | error
	Error          string
	Outcome        *float64
}

var allowedEntry = map[string]bool{"LONG": true, "SHORT": true, "NO_TRADE": true}
var allowedNews = map[string]bool{"BULLISH": true, "BEARISH": true, "NEUTRAL": true, "MIXED": true}

// Validate enforces data-model.md rules before insert.
func (d *ShadowDecision) Validate() error {
	switch d.JudgmentType {
	case "entry", "exit", "news":
	default:
		return fmt.Errorf("shadow_decisions: bad judgment_type %q", d.JudgmentType)
	}
	switch d.Judge {
	case "jev", "llm":
	default:
		return fmt.Errorf("shadow_decisions: bad judge %q", d.Judge)
	}
	if d.CycleID == "" {
		return fmt.Errorf("shadow_decisions: empty cycle_id")
	}
	if d.Status == "error" {
		if d.Error == "" {
			return fmt.Errorf("shadow_decisions: status=error requires non-empty error (FR-007)")
		}
		if d.Choice != "" || d.Noul != nil || d.Score != nil {
			return fmt.Errorf("shadow_decisions: error rows carry no judgment values")
		}
		return nil
	}
	if d.Status != "ok" {
		return fmt.Errorf("shadow_decisions: bad status %q", d.Status)
	}
	if d.Error != "" {
		return fmt.Errorf("shadow_decisions: status=ok must not carry error")
	}
	switch d.JudgmentType {
	case "entry":
		if !allowedEntry[d.Choice] {
			return fmt.Errorf("shadow_decisions: entry choice outside vocabulary: %q", d.Choice)
		}
	case "news":
		if !allowedNews[d.Choice] {
			return fmt.Errorf("shadow_decisions: news choice outside vocabulary: %q", d.Choice)
		}
	case "exit":
		if d.Noul == nil || *d.Noul < 0 || *d.Noul > 1 {
			return fmt.Errorf("shadow_decisions: exit requires noul in [0,1]")
		}
	}
	return nil
}

func strPtr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func anyPtr(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	return v
}

// InsertShadowDecision persists one record.
func (s *Store) InsertShadowDecision(ctx context.Context, d *ShadowDecision) error {
	if err := d.Validate(); err != nil {
		return err
	}
	var probs []byte
	if d.Probabilities != nil {
		var err error
		probs, err = json.Marshal(d.Probabilities)
		if err != nil {
			return fmt.Errorf("shadow_decisions: marshal probabilities: %w", err)
		}
	}
	row := s.Pool.QueryRow(ctx, `
		INSERT INTO shadow_decisions
		(judgment_type, cycle_id, symbol, news_cluster_id, state_ref, judge,
		 choice, noul, score, probabilities, confidence, baseline_choice,
		 route, latency_ms, input_tokens, output_tokens, status, error, outcome)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		RETURNING id, created_at`,
		d.JudgmentType, d.CycleID, strPtr(d.Symbol), d.NewsClusterID, strPtr(d.StateRef), d.Judge,
		strPtr(d.Choice), d.Noul, d.Score, anyPtr(probs), d.Confidence, strPtr(d.BaselineChoice),
		strPtr(d.Route), d.LatencyMS, d.InputTokens, d.OutputTokens, d.Status, strPtr(d.Error), d.Outcome)
	return row.Scan(&d.ID, &d.CreatedAt)
}

// Report aggregates evidence (FR-017).
func (s *Store) ShadowReport(ctx context.Context, judgmentType string, days int) (map[string]interface{}, error) {
	if days <= 0 {
		days = 14
	}
	var total, ok, errs, agree, withOutcome int
	var avgMS, avgConf, inTok, outTok float64
	err := s.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'ok'),
			COUNT(*) FILTER (WHERE status = 'error'),
			COALESCE(AVG(latency_ms), 0),
			COALESCE(AVG(confidence), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COUNT(*) FILTER (WHERE status = 'ok' AND choice = baseline_choice),
			COUNT(*) FILTER (WHERE outcome IS NOT NULL)
		FROM shadow_decisions
		WHERE judgment_type = $1 AND created_at > now() - make_interval(days => $2)`,
		judgmentType, days).Scan(&total, &ok, &errs, &avgMS, &avgConf, &inTok, &outTok, &agree, &withOutcome)
	if err != nil {
		return nil, fmt.Errorf("shadow report: %w", err)
	}
	if total == 0 {
		return nil, fmt.Errorf("shadow report: no records for type=%s days=%d", judgmentType, days)
	}
	agreement := 0.0
	if ok > 0 {
		agreement = float64(agree) / float64(ok)
	}
	return map[string]interface{}{
		"type":                 judgmentType,
		"days":                 days,
		"pairs":                total,
		"ok":                   ok,
		"errors":               errs,
		"agreement_rate":       agreement,
		"avg_latency_ms":       avgMS,
		"avg_confidence":       avgConf,
		"cost_per_1k":          (inTok + outTok) / float64(total),
		"records_with_outcome": withOutcome,
		"calibration":          s.calibrationBuckets(ctx, judgmentType, days),
	}, nil
}

// calibrationBuckets: SC-006 — predicted confidence vs realized outcome rate.
func (s *Store) calibrationBuckets(ctx context.Context, judgmentType string, days int) []map[string]interface{} {
	rows, err := s.Pool.Query(ctx, `
		SELECT width_bucket(confidence, 0, 1, 5) AS bucket,
		       AVG(confidence) AS predicted,
		       AVG(CASE WHEN outcome > 0 THEN 1.0 ELSE 0.0 END) AS realized,
		       COUNT(*) AS n
		FROM shadow_decisions
		WHERE judgment_type = $1 AND status = 'ok' AND outcome IS NOT NULL
		  AND created_at > now() - make_interval(days => $2)
		GROUP BY bucket ORDER BY bucket`, judgmentType, days)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []map[string]interface{}
	for rows.Next() {
		var b, predicted, realized, n interface{}
		if rows.Scan(&b, &predicted, &realized, &n) != nil {
			return out
		}
		out = append(out, map[string]interface{}{
			"bucket": b, "predicted": predicted, "realized": realized, "n": n,
		})
	}
	return out
}
