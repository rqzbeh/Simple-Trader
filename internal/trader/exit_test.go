package trader

import "testing"

// T042/T025: exit question carries ATR as data, not trigger.
func TestExitQuestionCarriesATRAsData(t *testing.T) {
	q := ExitQuestion(1.5, 3.0, 100.0)
	ans, ok := q["exit_now"]
	if !ok || ans.Type != "noul" {
		t.Fatalf("exit_now noul missing: %+v", q)
	}
	crit, ok := ans.Criteria.(map[string]string)
	if !ok || crit["false"] == "" {
		t.Fatal("hold criterion must state ATR alone does not force exit")
	}
}
