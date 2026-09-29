package trader

import (
	"os"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/config"
)

// TestMain bootstraps the required environment from config.SampleRequiredEnv
// (spec-017: no in-code defaults — tests must state their config) and loads
// the timeframe sets so GetBucketTimeframeSet works outside boot.
func TestMain(m *testing.M) {
	for k, v := range config.SampleRequiredEnv() {
		os.Setenv(k, v)
	}
	if _, err := config.LoadTimeframeConfig(); err != nil {
		os.Stderr.WriteString("TestMain: LoadTimeframeConfig: " + err.Error() + "\n")
		os.Exit(1)
	}
	os.Exit(m.Run())
}
