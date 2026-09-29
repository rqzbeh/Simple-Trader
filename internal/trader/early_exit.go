package trader

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/market"
	"github.com/rqzbeh/simple-trader/internal/telegram"
)

// EarlyExitStore abstracts persistence for early exit judgments.
type EarlyExitStore interface {
	InsertEarlyExitJudgment(ctx context.Context, j *db.EarlyExitJudgment) error
	HasTerminalEarlyExitJudgment(ctx context.Context, positionID int64, clusterID int64) (bool, error)
	CountEarlyExitsToday(ctx context.Context, symbol string, now time.Time) (int, error)
	GetLastEarlyExitTime(ctx context.Context, symbol string) (time.Time, bool, error)
	RecordEarlyExitClose(ctx context.Context, j *db.EarlyExitJudgment, signalID *int64, exitPrice float64, exitReason string, pnl, roi float64) error
}

// EarlyExitSender sends alerts (contracts §4).
type EarlyExitSender interface {
	SendMessageWithRetry(ctx context.Context, text string) error
}

// EarlyExitPositionContext describes an open trade inside Jev question instructions.
type EarlyExitPositionContext struct {
	Direction string  `json:"direction"`
	Entry     float64 `json:"entry"`
	ATR_SL    float64 `json:"atr_sl"`
	ATR_TP    float64 `json:"atr_tp"`
	AgeMin    float64 `json:"age_min"`
}

// EarlyExitClusterContext describes the catalyst cluster inside Jev question instructions.
type EarlyExitClusterContext struct {
	Headline string   `json:"headline"`
	AgeMin   float64  `json:"age_min"`
	Labels   []string `json:"labels,omitempty"`
}

// EarlyExitQuestionInstructions is the typed instructions payload for close_now questions.
type EarlyExitQuestionInstructions struct {
	Question string                   `json:"question"`
	Position EarlyExitPositionContext `json:"position"`
	Cluster  EarlyExitClusterContext  `json:"cluster"`
}

// Question keys for early-exit guards in Jev batch (spec-019).
const (
	QuestionEarlyExitMinHold   = "early_exit_min_hold"
	QuestionEarlyExitCooldown  = "early_exit_cooldown"
	QuestionEarlyExitMaxPerDay = "early_exit_max_per_day"
	QuestionEarlyExitConfFloor = "early_exit_conf_floor"
)

var (
	earlyExitMinHoldLevels   = []float64{10.0, 30.0, 60.0}
	earlyExitCooldownLevels  = []float64{15.0, 45.0, 90.0, 150.0}
	earlyExitMaxPerDayLevels = []float64{1.0, 3.0, 5.0}
	earlyExitConfFloorLevels = []float64{0.60, 0.70, 0.80, 0.90}
)

func EarlyExitMinHoldQuestion() ai.JevQuestion {
	return ai.JevQuestion{
		Type: "score",
		Criteria: []string{
			"<15 minutes too eager",
			"15-45 minutes reasonable",
			">45 minutes patient",
		},
		Instructions: map[string]interface{}{
			"question":   "Minimum hold time in minutes before an open position is eligible for early exit",
			"scale_note": "Score reflects position patience band in minutes: <15m (10m), 15-45m (30m), >45m (60m)",
		},
	}
}

func EarlyExitCooldownQuestion() ai.JevQuestion {
	return ai.JevQuestion{
		Type: "score",
		Criteria: []string{
			"<30 minutes rapid re-evaluation",
			"30-60 minutes balanced",
			"60-120 minutes conservative",
			">120 minutes patient",
		},
		Instructions: map[string]interface{}{
			"question":   "Cooldown period in minutes between early exit closures for the same symbol",
			"scale_note": "Score reflects cooldown minutes band: <30m (15m), 30-60m (45m), 60-120m (90m), >120m (150m)",
		},
	}
}

func EarlyExitMaxPerDayQuestion() ai.JevQuestion {
	return ai.JevQuestion{
		Type: "score",
		Criteria: []string{
			"1 exit: high conservatism",
			"2-3 exits: balanced default",
			"4-5 exits: active news session",
		},
		Instructions: map[string]interface{}{
			"question":   "Maximum number of news-driven early exit closures allowed per symbol per day (1-5 bands)",
			"scale_note": "Daily budget of news-driven early exits per symbol: 1 exit, 3 exits, 5 exits",
		},
	}
}

func EarlyExitConfFloorQuestion() ai.JevQuestion {
	return ai.JevQuestion{
		Type: "score",
		Criteria: []string{
			"0.55-0.65: moderate conviction",
			"0.65-0.75: standard confidence threshold",
			"0.75-0.85: high conviction requirement",
			">0.85: extreme certainty required",
		},
		Instructions: map[string]interface{}{
			"question":   "Minimum confidence floor required to trigger news-driven early exit (0.50-1.00)",
			"scale_note": "Confidence floor threshold on 0.50-1.00 scale for news invalidation",
		},
	}
}

// BuildEarlyExitGuardQuestions constructs Jev questions ONLY for guards in managed mode (FR-301).
// User override present => question omitted (FR-301).
func BuildEarlyExitGuardQuestions(cfg config.EarlyExitConfig, cluster *market.NewsCluster) map[string]ai.JevQuestion {
	questions := make(map[string]ai.JevQuestion)
	if cfg.Managed != nil && cfg.Managed["MIN_HOLD_MIN"] {
		questions[QuestionEarlyExitMinHold] = EarlyExitMinHoldQuestion()
	}
	if cfg.Managed != nil && cfg.Managed["COOLDOWN_MIN"] {
		questions[QuestionEarlyExitCooldown] = EarlyExitCooldownQuestion()
	}
	if cfg.Managed != nil && cfg.Managed["MAX_PER_DAY"] {
		questions[QuestionEarlyExitMaxPerDay] = EarlyExitMaxPerDayQuestion()
	}
	if cfg.Managed != nil && cfg.Managed["CONF_FLOOR"] {
		questions[QuestionEarlyExitConfFloor] = EarlyExitConfFloorQuestion()
	}
	return questions
}

// BuildEarlyExitQuestions constructs one batched Jev request with one close_now question per open position,
// plus guard questions for guards in managed mode when cfg is provided (FR-301).
// Contract §1: type "noul", noul=true means "close now".
func BuildEarlyExitQuestions(positions []*db.Trade, cluster *market.NewsCluster, now time.Time, cfgs ...config.EarlyExitConfig) map[string]ai.JevQuestion {
	questions := make(map[string]ai.JevQuestion, len(positions))
	clusterAgeMin := 0.0
	if cluster != nil && !cluster.FirstSeen.IsZero() {
		clusterAgeMin = now.Sub(cluster.FirstSeen).Minutes()
		if clusterAgeMin < 0 {
			clusterAgeMin = 0
		}
	}
	headline := ""
	if cluster != nil {
		headline = cluster.Headline
	}

	for _, pos := range positions {
		if pos == nil || pos.Status != "OPEN" {
			continue
		}
		qID := fmt.Sprintf("close_now:%d", pos.ID)
		dir := "LONG"
		if pos.Side == "SELL" || pos.Side == "SHORT" {
			dir = "SHORT"
		}
		posAgeMin := 0.0
		if !pos.EntryTime.IsZero() {
			posAgeMin = now.Sub(pos.EntryTime).Minutes()
			if posAgeMin < 0 {
				posAgeMin = 0
			}
		}

		questions[qID] = ai.JevQuestion{
			Type: "noul",
			Instructions: EarlyExitQuestionInstructions{
				Question: "Should this open position be CLOSED immediately because fresh news invalidated it?",
				Position: EarlyExitPositionContext{
					Direction: dir,
					Entry:     pos.EntryPrice,
					ATR_SL:    pos.StopLoss,
					ATR_TP:    pos.TakeProfit,
					AgeMin:    posAgeMin,
				},
				Cluster: EarlyExitClusterContext{
					Headline: headline,
					AgeMin:   clusterAgeMin,
				},
			},
			Criteria: map[string]string{
				"true":  "News invalidates the thesis via high-severity opposing catalyst — close now",
				"false": "Thesis intact — hold",
			},
		}
	}

	if len(cfgs) > 0 {
		guardQs := BuildEarlyExitGuardQuestions(cfgs[0], cluster)
		for k, q := range guardQs {
			questions[k] = q
		}
	}

	return questions
}

// EarlyExitPositionState describes an open position inside the early exit state (T004b).
type EarlyExitPositionState struct {
	ID         int64   `json:"id"`
	Symbol     string  `json:"symbol"`
	Side       string  `json:"side"`
	EntryPrice float64 `json:"entry_price"`
	StopLoss   float64 `json:"stop_loss,omitempty"`
	TakeProfit float64 `json:"take_profit,omitempty"`
	AgeMin     float64 `json:"age_min"`
	Leverage   int     `json:"leverage,omitempty"`
}

// EarlyExitStateObject is the real, non-null state payload passed to Jev in the early-exit cycle (T004b).
// Zero-fake-data: fields omit when absent, never fabricated.
type EarlyExitStateObject struct {
	Timestamp      string                   `json:"timestamp"`
	SessionHourUTC int                      `json:"session_hour_utc"`
	Cluster        *CatalystFact            `json:"cluster,omitempty"`
	ClusterAgeMin  float64                  `json:"cluster_age_min"`
	Positions      []EarlyExitPositionState `json:"positions"`
	Symbols        []string                 `json:"symbols,omitempty"`
}

// BuildEarlyExitState builds the real minimal state for the early-exit Jev batch (T004b).
// Guarantees a non-null object for TypeSafe without fabricated data.
func BuildEarlyExitState(positions []*db.Trade, cluster *market.NewsCluster, now time.Time) EarlyExitStateObject {
	posStates := make([]EarlyExitPositionState, 0, len(positions))
	symSet := make(map[string]bool)
	for _, p := range positions {
		if p == nil || p.Status != "OPEN" {
			continue
		}
		age := 0.0
		if !p.EntryTime.IsZero() {
			age = now.Sub(p.EntryTime).Minutes()
			if age < 0 {
				age = 0
			}
		}
		dir := "LONG"
		if p.Side == "SELL" || p.Side == "SHORT" {
			dir = "SHORT"
		}
		posStates = append(posStates, EarlyExitPositionState{
			ID:         p.ID,
			Symbol:     p.Symbol,
			Side:       dir,
			EntryPrice: p.EntryPrice,
			StopLoss:   p.StopLoss,
			TakeProfit: p.TakeProfit,
			AgeMin:     age,
			Leverage:   p.Leverage,
		})
		if p.Symbol != "" {
			symSet[p.Symbol] = true
		}
	}

	var cat *CatalystFact
	clusterAgeMin := 0.0
	if cluster != nil {
		if !cluster.FirstSeen.IsZero() {
			clusterAgeMin = now.Sub(cluster.FirstSeen).Minutes()
			if clusterAgeMin < 0 {
				clusterAgeMin = 0
			}
		}
		cat = &CatalystFact{
			Headline:       cluster.Headline,
			StoryCount:     cluster.StoryCount,
			FusedSentiment: cluster.FusedSentiment,
			Freshness:      cluster.FreshWeight,
			Sources:        cluster.Sources,
		}
	}

	symbols := make([]string, 0, len(symSet))
	for s := range symSet {
		symbols = append(symbols, s)
	}

	return EarlyExitStateObject{
		Timestamp:      now.UTC().Format(time.RFC3339),
		SessionHourUTC: now.UTC().Hour(),
		Cluster:        cat,
		ClusterAgeMin:  clusterAgeMin,
		Positions:      posStates,
		Symbols:        symbols,
	}
}

// ResolvedEarlyExitGuards contains per-cycle guard values and clamp details.
type ResolvedEarlyExitGuards struct {
	Enabled     bool
	MinHoldMin  int
	MaxPerDay   int
	CooldownMin int
	ConfFloor   float64
	Clamped     map[string]bool
	ClampDetail map[string]interface{}
}

func lookupGuardAnswer(answers map[string]ai.JevAnswer, keys ...string) *ai.JevAnswer {
	if answers == nil {
		return nil
	}
	for _, k := range keys {
		if a, ok := answers[k]; ok {
			return &a
		}
		if a, ok := answers["guard:"+k]; ok {
			return &a
		}
	}
	return nil
}

func extractGuardNumeric(raw interface{}, levelVals []float64) (float64, error) {
	if score, ok := rawScoreOnly(raw); ok {
		return score, nil
	}
	val, _, err := answerValue(raw, levelVals)
	return val, err
}

// ResolveEarlyExitGuards resolves early exit guard values from config overrides or Jev answers.
// Clamps values per hard safety bounds (FR-304/FR-603): min_hold [0,1440], cooldown [0,1440], max_per_day [1,10], conf_floor [0.5,1].
func ResolveEarlyExitGuards(cfg config.EarlyExitConfig, answers map[string]ai.JevAnswer, cycleID string) (ResolvedEarlyExitGuards, error) {
	res := ResolvedEarlyExitGuards{
		Enabled:     cfg.IsEnabled(),
		MinHoldMin:  cfg.MinHoldMin,
		MaxPerDay:   cfg.MaxPerDay,
		CooldownMin: cfg.CooldownMin,
		ConfFloor:   cfg.ConfFloor,
		Clamped:     make(map[string]bool),
		ClampDetail: make(map[string]interface{}),
	}

	// 1. MinHoldMin
	if cfg.Managed != nil && cfg.Managed["MIN_HOLD_MIN"] {
		ans := lookupGuardAnswer(answers, QuestionEarlyExitMinHold, "min_hold_min", "min_hold")
		if ans == nil {
			return res, fmt.Errorf("component=early-exit cycle=%s: missing answer for managed guard min_hold (FR-307 zero-fallback)", cycleID)
		}
		val, err := extractGuardNumeric(ans, earlyExitMinHoldLevels)
		if err != nil {
			return res, fmt.Errorf("component=early-exit cycle=%s: guard min_hold schema error: %w", cycleID, err)
		}
		clamped := false
		applied := int(math.Round(val))
		if val < 0 {
			applied = 0
			clamped = true
		} else if val > 1440 {
			applied = 1440
			clamped = true
		}
		if clamped {
			res.Clamped["min_hold"] = true
			res.ClampDetail["min_hold"] = map[string]interface{}{
				"requested": val,
				"applied":   applied,
				"bound":     "min_hold_cap",
			}
			log.Printf("[early-exit] cycle=%s: guard min_hold clamped from %v to %d (bound=min_hold_cap)", cycleID, val, applied)
		}
		res.MinHoldMin = applied
	}

	// 2. CooldownMin
	if cfg.Managed != nil && cfg.Managed["COOLDOWN_MIN"] {
		ans := lookupGuardAnswer(answers, QuestionEarlyExitCooldown, "cooldown_min", "cooldown")
		if ans == nil {
			return res, fmt.Errorf("component=early-exit cycle=%s: missing answer for managed guard cooldown (FR-307 zero-fallback)", cycleID)
		}
		val, err := extractGuardNumeric(ans, earlyExitCooldownLevels)
		if err != nil {
			return res, fmt.Errorf("component=early-exit cycle=%s: guard cooldown schema error: %w", cycleID, err)
		}
		clamped := false
		applied := int(math.Round(val))
		if val < 0 {
			applied = 0
			clamped = true
		} else if val > 1440 {
			applied = 1440
			clamped = true
		}
		if clamped {
			res.Clamped["cooldown"] = true
			res.ClampDetail["cooldown"] = map[string]interface{}{
				"requested": val,
				"applied":   applied,
				"bound":     "cooldown_cap",
			}
			log.Printf("[early-exit] cycle=%s: guard cooldown clamped from %v to %d (bound=cooldown_cap)", cycleID, val, applied)
		}
		res.CooldownMin = applied
	}

	// 3. MaxPerDay
	if cfg.Managed != nil && cfg.Managed["MAX_PER_DAY"] {
		ans := lookupGuardAnswer(answers, QuestionEarlyExitMaxPerDay, "max_per_day")
		if ans == nil {
			return res, fmt.Errorf("component=early-exit cycle=%s: missing answer for managed guard max_per_day (FR-307 zero-fallback)", cycleID)
		}
		val, err := extractGuardNumeric(ans, earlyExitMaxPerDayLevels)
		if err != nil {
			return res, fmt.Errorf("component=early-exit cycle=%s: guard max_per_day schema error: %w", cycleID, err)
		}
		clamped := false
		applied := int(math.Round(val))
		if val < 1 {
			applied = 1
			clamped = true
		} else if val > 10 {
			applied = 10
			clamped = true
		}
		if clamped {
			res.Clamped["max_per_day"] = true
			res.ClampDetail["max_per_day"] = map[string]interface{}{
				"requested": val,
				"applied":   applied,
				"bound":     "max_per_day_cap",
			}
			log.Printf("[early-exit] cycle=%s: guard max_per_day clamped from %v to %d (bound=max_per_day_cap)", cycleID, val, applied)
		}
		res.MaxPerDay = applied
	}

	// 4. ConfFloor
	if cfg.Managed != nil && cfg.Managed["CONF_FLOOR"] {
		ans := lookupGuardAnswer(answers, QuestionEarlyExitConfFloor, "conf_floor")
		if ans == nil {
			return res, fmt.Errorf("component=early-exit cycle=%s: missing answer for managed guard conf_floor (FR-307 zero-fallback)", cycleID)
		}
		val, err := extractGuardNumeric(ans, earlyExitConfFloorLevels)
		if err != nil {
			return res, fmt.Errorf("component=early-exit cycle=%s: guard conf_floor schema error: %w", cycleID, err)
		}
		clamped := false
		applied := val
		if val < 0.5 {
			applied = 0.5
			clamped = true
		} else if val > 1.0 {
			applied = 1.0
			clamped = true
		}
		if clamped {
			res.Clamped["conf_floor"] = true
			res.ClampDetail["conf_floor"] = map[string]interface{}{
				"requested": val,
				"applied":   applied,
				"bound":     "conf_floor_cap",
			}
			log.Printf("[early-exit] cycle=%s: guard conf_floor clamped from %v to %v (bound=conf_floor_cap)", cycleID, val, applied)
		}
		res.ConfFloor = applied
	}

	return res, nil
}

// EarlyExitGuardParams contains inputs required to evaluate early exit guards.
type EarlyExitGuardParams struct {
	Config          config.EarlyExitConfig
	PositionAgeMin  float64
	LastExitAgeMin  float64
	HasPriorExit    bool
	ExitsTodayCount int
	Confidence      float64
	ResolvedGuards  *ResolvedEarlyExitGuards
}

// EvaluateEarlyExitGuards evaluates hard guards in contracts §2 order:
// kill_switch → min_hold → cooldown → daily_budget → conf_floor.
// When ResolvedGuards is supplied, resolved per-cycle values are used (T005).
// First fail wins. Returns (passed, reason).
func EvaluateEarlyExitGuards(p EarlyExitGuardParams) (bool, string) {
	enabled := p.Config.IsEnabled()
	minHold := p.Config.MinHoldMin
	cooldown := p.Config.CooldownMin
	maxPerDay := p.Config.MaxPerDay
	confFloor := p.Config.ConfFloor

	if p.ResolvedGuards != nil {
		enabled = p.ResolvedGuards.Enabled
		minHold = p.ResolvedGuards.MinHoldMin
		cooldown = p.ResolvedGuards.CooldownMin
		maxPerDay = p.ResolvedGuards.MaxPerDay
		confFloor = p.ResolvedGuards.ConfFloor
	}

	if !enabled {
		return false, "kill_switch"
	}
	if minHold > 0 && p.PositionAgeMin < float64(minHold) {
		return false, "min_hold"
	}
	if cooldown > 0 && p.HasPriorExit && p.LastExitAgeMin < float64(cooldown) {
		return false, "cooldown"
	}
	if maxPerDay > 0 && p.ExitsTodayCount >= maxPerDay {
		return false, "budget"
	}
	if p.Confidence < confFloor {
		return false, "conf_floor"
	}
	return true, ""
}

func parseClusterIDInt64(clusterID string) *int64 {
	if id, err := strconv.ParseInt(clusterID, 10, 64); err == nil {
		return &id
	}
	return nil
}

// RecordEarlyExitCoreError records a decision core error row without touching the position (FR-105).
func RecordEarlyExitCoreError(ctx context.Context, cycleID string, pos *db.Trade, cluster *market.NewsCluster, coreErr error, store EarlyExitStore) {
	if store == nil || pos == nil {
		return
	}
	errMsg := fmt.Sprintf("component=early-exit cycle=%s: %v", cycleID, coreErr)
	var cID *int64
	headline := ""
	if cluster != nil {
		cID = parseClusterIDInt64(cluster.ID)
		headline = cluster.Headline
	}
	j := &db.EarlyExitJudgment{
		CycleID:         cycleID,
		PositionID:      pos.ID,
		Symbol:          pos.Symbol,
		ClusterID:       cID,
		ClusterHeadline: headline,
		Action:          "error",
		Status:          "error",
		Error:           errMsg,
	}
	_ = store.InsertEarlyExitJudgment(ctx, j)
}

// ProcessEarlyExitVerdict executes verdict mapping, guard evaluation, position closure,
// Telegram alert (exactly once), and judgment persistence.
func ProcessEarlyExitVerdict(
	ctx context.Context,
	cycleID string,
	cfg config.EarlyExitConfig,
	pos *db.Trade,
	cluster *market.NewsCluster,
	ans ai.JevAnswer,
	route string,
	engine *ExecutionEngine,
	store EarlyExitStore,
	sender EarlyExitSender,
	now time.Time,
	resolved ...ResolvedEarlyExitGuards,
) error {
	var rg *ResolvedEarlyExitGuards
	if len(resolved) > 0 {
		rg = &resolved[0]
	}

	confFloor := cfg.ConfFloor
	if rg != nil {
		confFloor = rg.ConfFloor
	}

	var noul float64
	if ans.Noul != nil {
		noul = *ans.Noul
	} else {
		noul = ans.Confidence
	}
	conf := ans.Confidence
	if conf == 0 {
		conf = noul
	}

	var cID *int64
	headline := ""
	if cluster != nil {
		cID = parseClusterIDInt64(cluster.ID)
		headline = cluster.Headline
	}

	// 1. Verdict mapping
	// If close probability < conf floor => HOLD
	if noul < confFloor {
		fFalse := false
		j := &db.EarlyExitJudgment{
			CycleID:         cycleID,
			PositionID:      pos.ID,
			Symbol:          pos.Symbol,
			ClusterID:       cID,
			ClusterHeadline: headline,
			Verdict:         "HOLD",
			Noul:            &noul,
			Confidence:      &conf,
			Route:           route,
			GuardsPassed:    &fFalse,
			GuardReason:     "conf_floor",
			Action:          "guarded_skip",
			Status:          "ok",
		}
		if store != nil {
			_ = store.InsertEarlyExitJudgment(ctx, j)
		}
		return nil
	}

	// 2. Candidate DO_NOT_HOLD -> evaluate guards
	posAgeMin := 0.0
	if !pos.EntryTime.IsZero() {
		posAgeMin = now.Sub(pos.EntryTime).Minutes()
		if posAgeMin < 0 {
			posAgeMin = 0
		}
	}

	var lastExitAgeMin float64
	var hasPrior bool
	var exitsTodayCount int

	if store != nil {
		if lastExit, exists, err := store.GetLastEarlyExitTime(ctx, pos.Symbol); err == nil && exists {
			hasPrior = true
			lastExitAgeMin = now.Sub(lastExit).Minutes()
		}
		if count, err := store.CountEarlyExitsToday(ctx, pos.Symbol, now); err == nil {
			exitsTodayCount = count
		}
	}

	passed, guardReason := EvaluateEarlyExitGuards(EarlyExitGuardParams{
		Config:          cfg,
		PositionAgeMin:  posAgeMin,
		LastExitAgeMin:  lastExitAgeMin,
		HasPriorExit:    hasPrior,
		ExitsTodayCount: exitsTodayCount,
		Confidence:      conf,
		ResolvedGuards:  rg,
	})

	if !passed {
		// Guard blocked close: record guarded_skip with exact reason
		fFalse := false
		j := &db.EarlyExitJudgment{
			CycleID:         cycleID,
			PositionID:      pos.ID,
			Symbol:          pos.Symbol,
			ClusterID:       cID,
			ClusterHeadline: headline,
			Verdict:         "DO_NOT_HOLD",
			Noul:            &noul,
			Confidence:      &conf,
			Route:           route,
			GuardsPassed:    &fFalse,
			GuardReason:     guardReason,
			Action:          "guarded_skip",
			Status:          "ok",
		}
		if store != nil {
			_ = store.InsertEarlyExitJudgment(ctx, j)
		}
		return nil
	}

	// 3. Guards passed -> execute close under engine lock
	fTrue := true
	closedTrade, closed := engine.ForceClosePosition(pos.Symbol, 0, "NEWS_EARLY_EXIT")
	if !closed || closedTrade == nil {
		// Close failed -> record action=error with close_error
		closeErr := fmt.Sprintf("component=early-exit cycle=%s: ForceClosePosition failed for %s", cycleID, pos.Symbol)
		j := &db.EarlyExitJudgment{
			CycleID:         cycleID,
			PositionID:      pos.ID,
			Symbol:          pos.Symbol,
			ClusterID:       cID,
			ClusterHeadline: headline,
			Verdict:         "DO_NOT_HOLD",
			Noul:            &noul,
			Confidence:      &conf,
			Route:           route,
			GuardsPassed:    &fTrue,
			Action:          "error",
			CloseError:      closeErr,
			Status:          "ok",
		}
		if store != nil {
			_ = store.InsertEarlyExitJudgment(ctx, j)
		}
		return fmt.Errorf("%s", closeErr)
	}

	// 4. Close succeeded -> Telegram alert EXACTLY ONCE
	dir := "LONG"
	if closedTrade.Side == "SELL" || closedTrade.Side == "SHORT" {
		dir = "SHORT"
	}
	tgMsg := telegram.EarlyExitMessage{
		Symbol:     closedTrade.Symbol,
		Direction:  dir,
		Cluster:    headline,
		Confidence: conf,
		Route:      route,
		PnLUSD:     closedTrade.RealizedPnL,
		ReturnPct:  float64(closedTrade.ReturnPct),
		ExitReason: "NEWS_EARLY_EXIT",
	}
	text := telegram.FormatEarlyExit(tgMsg)

	var tgSent bool
	var tgErrStr string
	if sender != nil {
		if err := sender.SendMessageWithRetry(ctx, text); err != nil {
			tgSent = false
			tgErrStr = fmt.Sprintf("component=early-exit cycle=%s: telegram send: %v", cycleID, err)
		} else {
			tgSent = true
		}
	}

	// 5. Persist closed judgment row and signal status atomically
	outcome := float64(closedTrade.ReturnPct)
	j := &db.EarlyExitJudgment{
		CycleID:         cycleID,
		PositionID:      pos.ID,
		Symbol:          pos.Symbol,
		ClusterID:       cID,
		ClusterHeadline: headline,
		Verdict:         "DO_NOT_HOLD",
		Noul:            &noul,
		Confidence:      &conf,
		Route:           route,
		GuardsPassed:    &fTrue,
		Action:          "closed",
		TelegramSent:    &tgSent,
		TelegramError:   tgErrStr,
		Status:          "ok",
		Outcome:         &outcome,
	}
	if store != nil {
		pnlUSD := closedTrade.RealizedPnL
		roiPct := float64(closedTrade.ReturnPct)
		if err := store.RecordEarlyExitClose(ctx, j, pos.SignalID, closedTrade.ExitPrice, "NEWS_EARLY_EXIT", pnlUSD, roiPct); err != nil {
			// Transaction failed -> record action=error with close_error and return explicit error (no silent divergence)
			divergenceErr := fmt.Sprintf("component=early-exit cycle=%s: RecordEarlyExitClose failed for %s: %v", cycleID, pos.Symbol, err)
			errJ := &db.EarlyExitJudgment{
				CycleID:         cycleID,
				PositionID:      pos.ID,
				Symbol:          pos.Symbol,
				ClusterID:       cID,
				ClusterHeadline: headline,
				Verdict:         "DO_NOT_HOLD",
				Noul:            &noul,
				Confidence:      &conf,
				Route:           route,
				GuardsPassed:    &fTrue,
				Action:          "error",
				CloseError:      divergenceErr,
				Status:          "ok",
			}
			_ = store.InsertEarlyExitJudgment(ctx, errJ)
			return fmt.Errorf("%s", divergenceErr)
		}
	}

	return nil
}

// EarlyExitManager coordinates periodic news-driven early exit evaluation.
type EarlyExitManager struct {
	mu        sync.Mutex
	cfg       config.EarlyExitConfig
	engine    *ExecutionEngine
	store     EarlyExitStore
	sender    EarlyExitSender
	jev       *ai.JevClient
	clusterer *market.Clusterer
	broadcast func(event string, data interface{})
}

// NewEarlyExitManager constructs an EarlyExitManager.
func NewEarlyExitManager(
	cfg config.EarlyExitConfig,
	engine *ExecutionEngine,
	store EarlyExitStore,
	sender EarlyExitSender,
	jev *ai.JevClient,
	clusterer *market.Clusterer,
	broadcast func(event string, data interface{}),
) *EarlyExitManager {
	return &EarlyExitManager{
		cfg:       cfg,
		engine:    engine,
		store:     store,
		sender:    sender,
		jev:       jev,
		clusterer: clusterer,
		broadcast: broadcast,
	}
}

// UpdateConfig updates guard configuration live.
func (m *EarlyExitManager) UpdateConfig(cfg config.EarlyExitConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
}

// EvaluateCycle evaluates open positions against active clusters in one batched cycle.
func (m *EarlyExitManager) EvaluateCycle(ctx context.Context) {
	m.mu.Lock()
	cfg := m.cfg
	engine := m.engine
	store := m.store
	sender := m.sender
	jev := m.jev
	clusterer := m.clusterer
	broadcast := m.broadcast
	m.mu.Unlock()

	if engine == nil || jev == nil || clusterer == nil {
		return
	}

	openTrades := engine.GetOpenTrades()
	if len(openTrades) == 0 {
		return
	}

	now := time.Now()
	clusters := clusterer.Clusters(now)
	if len(clusters) == 0 {
		return
	}

	// For each active cluster, batch all eligible open positions
	for _, cl := range clusters {
		var clusterIDInt int64
		if id, err := strconv.ParseInt(cl.ID, 10, 64); err == nil {
			clusterIDInt = id
		}

		eligible := make([]*db.Trade, 0, len(openTrades))
		for _, pos := range openTrades {
			if store != nil && clusterIDInt > 0 {
				hasTerminal, err := store.HasTerminalEarlyExitJudgment(ctx, pos.ID, clusterIDInt)
				if err == nil && hasTerminal {
					continue // deduplicated: already judged for this cluster
				}
			}
			eligible = append(eligible, pos)
		}

		if len(eligible) == 0 {
			continue
		}

		cycleID := fmt.Sprintf("early-exit-%d-%s", now.UnixNano(), cl.ID)
		questions := BuildEarlyExitQuestions(eligible, cl, now, cfg)
		if len(questions) == 0 {
			continue
		}

		state := BuildEarlyExitState(eligible, cl, now)
		answers, _, err := jev.Evaluate(ctx, cycleID, state, questions)
		if err != nil {
			log.Printf("[early-exit] cycle=%s: Jev batch evaluation failed: %v", cycleID, err)
			for _, pos := range eligible {
				RecordEarlyExitCoreError(ctx, cycleID, pos, cl, err, store)
			}
			continue
		}

		resolvedGuards, rerr := ResolveEarlyExitGuards(cfg, answers, cycleID)
		if rerr != nil {
			log.Printf("[early-exit] cycle=%s: ResolveEarlyExitGuards failed: %v", cycleID, rerr)
			for _, pos := range eligible {
				RecordEarlyExitCoreError(ctx, cycleID, pos, cl, rerr, store)
			}
			continue
		}

		for _, pos := range eligible {
			qID := fmt.Sprintf("close_now:%d", pos.ID)
			ans, ok := answers[qID]
			if !ok {
				continue
			}

			// Capture status before verdict processing
			preCount := len(engine.GetClosedTrades())

			_ = ProcessEarlyExitVerdict(ctx, cycleID, cfg, pos, cl, ans, "jev_direct", engine, store, sender, now, resolvedGuards)

			// If position was closed, broadcast SSE event
			if len(engine.GetClosedTrades()) > preCount && broadcast != nil {
				closedTrades := engine.GetClosedTrades()
				lastClosed := closedTrades[len(closedTrades)-1]
				broadcast("trade_closed", lastClosed)
			}
		}
	}
}

// Start spawns the background worker evaluating early exits periodically.
func (m *EarlyExitManager) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.EvaluateCycle(ctx)
			}
		}
	}()
}
