package server

import (
	"os"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/config"
)

// TestMain installs the required environment from config.SampleRequiredEnv
// (spec-017: constructors fail loud without config — tests must state theirs).
func TestMain(m *testing.M) {
	for k, v := range config.SampleRequiredEnv() {
		os.Setenv(k, v)
	}
	os.Exit(m.Run())
}
