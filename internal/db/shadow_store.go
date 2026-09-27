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
	rows, err := s.Pool.Query(ctx, `
		SELECT judge, route, status,
		       COUNT(*) AS n,
		       AVG(latency_ms) AS avg_ms,
		       AVG(confidence) AS avg_conf,
		       SUM(input_tokens) AS in_tok,
		       SUM(output_tokens) AS out_tok,
		       CASE WHEN choice IS NOT NULL AND choice = baseline_choice THEN 1 ELSE 0 END AS agree,
		       outcome
		FROM shadow_decisions
		WHERE judgment_type = $1 AND created_at > now() - make_interval(days => $2)
		GROUP BY judge, route, status, outcome`, judgmentType, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	total, ok, errs := 0, 0, 0
	var latSum, confSum, inTok, outTok float64
	var agree, withOutcome int
	for rows.Next() {
		var judge, route, status string
		var n, avgMS, avgConf, inT, outT, agree1, outcome *float64
		if err := rows.Scan(&judge, &route, &status, &n, &avgMS, &avgConf, &inT, &outT, &agree1, &outcome); err != nil {
			return nil, err
		}
		if n == nil {
			continue
		}
		total += int(*n)
		if status == "error" {
			errs += int(*n)
		} else {
			ok += int(*n)
		}
		if avgMS != nil {
			latSum += *avgMS * *n
		}
		if avgConf != nil {
			confSum += *avgConf * *n
		}
		if inT != nil {
			inTok += *inT
		}
		if outT != nil {
			outTok += *outT
		}
		if agree1 != nil {
			agree += int(*agree1)
		}
		if outcome != nil {
			withOutcome += int(*n)
		}
	}
	if total == 0 {
		return nil, fmt.Errorf("shadow report: no records for type=%s days=%d", judgmentType, days)
	}
	agreement := 0.0
	if ok > 0 {
		agreement = float64(agree) / float64(ok)
	}
	return map[string]interface{}{
		"type":                    judgmentType,
		"days":                    days,
		"pairs":                   total,
		"ok":                      ok,
		"errors":                  errs,
		"agreement_rate":          agreement,
		"avg_latency_ms":          latSum / float64(total),
		"avg_confidence":          confSum / float64(total),
		"cost_per_1k":             (inTok + outTok) / float64(total),
		"records_with_outcome":    withOutcome,
	}, nil
}
