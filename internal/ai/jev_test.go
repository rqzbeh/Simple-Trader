package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func jevMock(t *testing.T, handler http.HandlerFunc) *httptest.Server { return httptest.NewServer(handler) }

func TestJevEvaluateParsesTypedAnswer(t *testing.T) {
	t.Setenv("JEV_MODEL", "jev-latest") // required env — no in-code model default (spec-017)
	srv := jevMock(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("missing auth header")
		}
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "jev-latest" {
			t.Errorf("model=%v", req["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"model": "jev-1.13.0",
			"answers": map[string]interface{}{
				"entry": map[string]interface{}{
					"type": "choice", "choice": "LONG", "confidence": 0.8,
					"probabilities": map[string]float64{"LONG": 0.8, "SHORT": 0.0, "NO_TRADE": 0.2},
				},
			},
			"usage": map[string]int{"input_tokens": 10, "output_tokens": 5},
		})
	})
	defer srv.Close()
	j := NewJevClient(srv.URL, "k", time.Second)
	answers, usage, err := j.Evaluate(context.Background(), "c1", map[string]string{"symbol": "BTC"},
		map[string]JevQuestion{"entry": {Type: "choice", Instructions: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if answers["entry"].Choice != "LONG" || usage.InputTokens != 10 {
		t.Fatalf("bad parse: %+v %+v", answers, usage)
	}
}

func TestJevSchemaErrorsAreTyped(t *testing.T) {
	srv := jevMock(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"model": "x", "answers": map[string]interface{}{}})
	})
	defer srv.Close()
	j := NewJevClient(srv.URL, "k", time.Second)
	_, _, err := j.Evaluate(context.Background(), "c2", nil, map[string]JevQuestion{"entry": {Type: "choice"}})
	if err == nil || err.Error() == "" {
		t.Fatal("missing answer must be explicit schema error")
	}
}

func TestValidateChoiceVocabularyAndSum(t *testing.T) {
	vocab := map[string]bool{"LONG": true, "SHORT": true, "NO_TRADE": true}
	good := JevAnswer{Type: "choice", Choice: "LONG", Probabilities: map[string]float64{"LONG": 0.7, "SHORT": 0.2, "NO_TRADE": 0.1}}
	if err := ValidateChoice(good, vocab, "c"); err != nil {
		t.Fatal(err)
	}
	buy := JevAnswer{Type: "choice", Choice: "BUY", Probabilities: map[string]float64{"BUY": 1}}
	if err := ValidateChoice(buy, vocab, "c"); err == nil {
		t.Fatal("BUY must fail validation (FR-005)")
	}
}

// TestUpstreamTransport_DirectByDefault: proxy is optional — no
// UPSTREAM_PROXY_URL in .env means a DIRECT connection even when ambient
// HTTP_PROXY/ALL_PROXY variables exist (2026-09-29 requirement).
func TestUpstreamTransport_DirectByDefault(t *testing.T) {
	t.Setenv("UPSTREAM_PROXY_URL", "")
	t.Setenv("HTTP_PROXY", "http://ambient-proxy:3128")
	t.Setenv("HTTPS_PROXY", "http://ambient-proxy:3128")

	tr, ok := upstreamTransport().(*http.Transport)
	if !ok {
		t.Fatalf("upstreamTransport returned %T, want *http.Transport", upstreamTransport())
	}
	if tr.Proxy != nil {
		// Force evaluation: ambient proxy must NOT engage by default.
		req, _ := http.NewRequest(http.MethodGet, "https://api.typesafe.ai/v1", nil)
		u, err := tr.Proxy(req)
		if err != nil {
			t.Fatalf("proxy func error: %v", err)
		}
		if u != nil {
			t.Errorf("default must be DIRECT, ambient proxy resolved to %s", u)
		}
	}
}

// TestUpstreamTransport_ProxyWhenSet: user-provided socks5/http proxy engages.
func TestUpstreamTransport_ProxyWhenSet(t *testing.T) {
	t.Setenv("UPSTREAM_PROXY_URL", "socks5://127.0.0.1:1080")
	tr, ok := upstreamTransport().(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Fatalf("proxy must be configured when UPSTREAM_PROXY_URL is set")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.typesafe.ai/v1", nil)
	u, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("proxy func error: %v", err)
	}
	if u == nil || u.Scheme != "socks5" {
		t.Errorf("proxy = %v, want socks5://127.0.0.1:1080", u)
	}
}

// TestUpstreamTransport_InvalidValueStaysDirect: malformed value cannot route
// traffic somewhere unintended (startup validation rejects it loudly in config).
func TestUpstreamTransport_InvalidValueStaysDirect(t *testing.T) {
	t.Setenv("UPSTREAM_PROXY_URL", "ftp://bad-scheme")
	tr := upstreamTransport().(*http.Transport)
	if tr.Proxy != nil {
		t.Errorf("invalid scheme must not configure a proxy")
	}
}
