package trader

import (
	"context"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/db"
)

// ExitQuestion builds the core's exit judgment: ATR levels are data, never
// triggers (US3 / FR-014).
func ExitQuestion(atrSL, atrTP, current float64) map[string]ai.JevQuestion {
	return map[string]ai.JevQuestion{
		"exit_now": {
			Type: "noul",
			Instructions: map[string]interface{}{
				"question": "Should this open futures position be closed NOW?",
				"atr_stop_loss": atrSL, "atr_take_profit": atrTP, "current_price": current,
			},
			Criteria: map[string]string{
				"true":  "Exit edge now: target/stop proximity, reversal evidence, or catalyst expiry",
				"false": "Hold: no exit edge; ATR levels alone do not force exit",
			},
		},
	}
}

// JudgeExit records an exit judgment for an open position (async, non-blocking).
func (o *ShadowOrchestrator) JudgeExit(cycleID, symbol string, state interface{}, atrSL, atrTP, current, baseline float64) {
	if !o.active("exit") || o.Router == nil {
		return
	}
	_ = o.Enqueue(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		d := &db.ShadowDecision{JudgmentType: "exit", CycleID: cycleID, Symbol: symbol, Judge: "jev", StateRef: symbol}
		answers, usage, err := o.Router.Jev.Evaluate(ctx, cycleID, state, ExitQuestion(atrSL, atrTP, current))
		if err != nil {
			d.Status, d.Error = "error", err.Error()
		} else {
			ans := answers["exit_now"]
			if ans.Noul == nil {
				d.Status, d.Error = "error", ai.WrapDecision("jev", cycleID, ai.ErrJevSchema, "missing noul answer").Error()
			} else {
				d.Status, d.Noul, d.Route = "ok", ans.Noul, "jev_direct"
				d.LatencyMS = int(usage.Latency / time.Millisecond)
				d.InputTokens, d.OutputTokens = usage.InputTokens, usage.OutputTokens
				conf := ans.Confidence
				d.Confidence = &conf
			}
		}
		_ = o.Record(ctx, d)
	})
}
