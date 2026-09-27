package trader

import "testing"

// T042/T007: shadow toggles per type (FR-010).
func TestShadowPerTypeToggles(t *testing.T) {
	o := &ShadowOrchestrator{}
	o.SetEnabled("entry", true)
	o.SetEnabled("news", false)
	if !o.active("entry") || o.active("news") || o.active("exit") {
		t.Fatal("per-type toggles incorrect")
	}
}
