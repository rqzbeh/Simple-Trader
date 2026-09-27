package trader

import (
	"context"
	"errors"
	"testing"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/market"
)

// SC-007: every injected failure surfaces an explicit named error.
func TestChaosAllFailuresExplicit(t *testing.T) {
	cases := []struct {
		name string
		run  func() error
	}{
		{"jev-unconfigured", func() error {
			j := ai.NewJevClient("", "", 0)
			_, _, err := j.Evaluate(context.Background(), "cyc", nil, nil)
			return err
		}},
		{"router-no-threshold", func() error {
			_, err := (&DecisionRouter{}).Route(context.Background(), "cyc", nil, nil, nil)
			return err
		}},
		{"router-escalation-down", func() error {
			srv := jevServer(t, 0.3, "LONG")
			defer srv.Close()
			r := &DecisionRouter{Jev: ai.NewJevClient(srv.URL, "k", 0), Threshold: 0.9,
				Escalate: func(ctx context.Context, s interface{}) (DecisionOutcome, error) {
					return DecisionOutcome{}, errors.New("9router 500")
				}}
			_, err := r.Route(context.Background(), "cyc", nil,
				map[string]ai.JevQuestion{"entry": {Type: "choice"}}, entryVocab)
			return err
		}},
		{"news-classifier-unconfigured", func() error {
			_, err := market.DefaultClassifier([]string{"x"})
			return err
		}},
		{"llm-unconfigured", func() error {
			_, err := ai.NewClient(ai.ClientConfig{}).ClassifyNews(context.Background(), "BTC", []string{"h"})
			return err
		}},
		{"llm-empty-cluster", func() error {
			_, err := (&ai.Client{}).ClassifyNews(context.Background(), "BTC", nil)
			return err
		}},
	}
	for _, c := range cases {
		err := c.run()
		if err == nil {
			t.Fatalf("%s: expected explicit error, got nil (silent degradation)", c.name)
		}
		msg := err.Error()
		if len(msg) < 10 || (!containsSub(msg, "component=") && !containsSub(msg, "cycle=")) &&
			!containsSub(msg, "config:") && !containsSub(msg, "classifier not configured") {
			t.Fatalf("%s: error not explicit: %q", c.name, msg)
		}
	}
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
