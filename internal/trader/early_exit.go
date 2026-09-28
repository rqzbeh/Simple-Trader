package trader

import (
	"context"
	"fmt"
	"log"
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

// BuildEarlyExitQuestions constructs one batched Jev request with one close_now question per open position.
// Contract §1: type "noul", noul=true means "close now".
func BuildEarlyExitQuestions(positions []*db.Trade, cluster *market.NewsCluster, now time.Time) map[string]ai.JevQuestion {
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
	return questions
}

// EarlyExitGuardParams contains inputs required to evaluate early exit guards.
type EarlyExitGuardParams struct {
	Config          config.EarlyExitConfig
	PositionAgeMin  float64
	LastExitAgeMin  float64
	HasPriorExit    bool
	ExitsTodayCount int
	Confidence      float64
}

// EvaluateEarlyExitGuards evaluates hard guards in contracts §2 order:
// kill_switch → min_hold → cooldown → daily_budget → conf_floor.
// First fail wins. Returns (passed, reason).
func EvaluateEarlyExitGuards(p EarlyExitGuardParams) (bool, string) {
	if !p.Config.Enabled {
		return false, "kill_switch"
	}
	if p.Config.MinHoldMin > 0 && p.PositionAgeMin < float64(p.Config.MinHoldMin) {
		return false, "min_hold"
	}
	if p.Config.CooldownMin > 0 && p.HasPriorExit && p.LastExitAgeMin < float64(p.Config.CooldownMin) {
		return false, "cooldown"
	}
	if p.Config.MaxPerDay > 0 && p.ExitsTodayCount >= p.Config.MaxPerDay {
		return false, "budget"
	}
	if p.Confidence < p.Config.ConfFloor {
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
) error {
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
	if noul < cfg.ConfFloor {
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
		questions := BuildEarlyExitQuestions(eligible, cl, now)
		if len(questions) == 0 {
			continue
		}

		answers, _, err := jev.Evaluate(ctx, cycleID, nil, questions)
		if err != nil {
			log.Printf("[early-exit] cycle=%s: Jev batch evaluation failed: %v", cycleID, err)
			for _, pos := range eligible {
				RecordEarlyExitCoreError(ctx, cycleID, pos, cl, err, store)
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

			_ = ProcessEarlyExitVerdict(ctx, cycleID, cfg, pos, cl, ans, "jev_direct", engine, store, sender, now)

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
