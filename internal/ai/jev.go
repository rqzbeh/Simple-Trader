package ai

import (
	"os"
	"net/url"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// JevClient talks to TypeSafe System One (spec-013 contracts/shadow-jobs.md §1).
// Typed errors only — no default answers, ever.
type JevClient struct {
	mu      sync.RWMutex
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func NewJevClient(baseURL, apiKey string, timeout time.Duration) *JevClient {
	if baseURL == "" {
		baseURL = "https://api.typesafe.ai"
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &JevClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   "jev-latest",
		http:    &http.Client{Timeout: timeout, Transport: upstreamTransport()},
	}
}

// SetAPIKey dynamically updates the TypeSafe API key live.
func (j *JevClient) SetAPIKey(key string) {
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.apiKey = key
}

// GetAPIKey returns the current TypeSafe API key.
func (j *JevClient) GetAPIKey() string {
	if j == nil {
		return ""
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.apiKey
}

// BaseURL returns the configured base URL.
func (j *JevClient) BaseURL() string {
	if j == nil {
		return "https://api.typesafe.ai"
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.baseURL == "" {
		return "https://api.typesafe.ai"
	}
	return j.baseURL
}

func (j *JevClient) Model() string {
	if j == nil || j.model == "" {
		return "jev-latest"
	}
	return j.model
}

// JevQuestion per official API: type + instructions + criteria.
type JevQuestion struct {
	Type         string      `json:"type"` // choice | noul | score
	Instructions interface{} `json:"instructions"`
	Criteria     interface{} `json:"criteria,omitempty"`
}

// JevAnswer mirrors the response answer union.
type JevAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Noul          *float64           `json:"noul,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Legend        map[string]string  `json:"legend,omitempty"`
}

type jevRequest struct {
	State     interface{}           `json:"state"`
	Model     string                `json:"model"`
	Questions map[string]JevQuestion `json:"questions"`
}

type jevResponse struct {
	Model   string               `json:"model"`
	Answers map[string]JevAnswer `json:"answers"`
	Usage   struct {
		InputTokens  int     `json:"input_tokens"`
		OutputTokens int     `json:"output_tokens"`
		Cost         float64 `json:"cost"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// JevUsage captured for cost reporting.
type JevUsage struct {
	InputTokens  int
	OutputTokens int
	Model        string
	Latency      time.Duration
}

// Evaluate sends one batched request — all independent questions in a single
// parallel pass (research.md batch rule).
func (j *JevClient) Evaluate(ctx context.Context, cycleID string, state interface{}, questions map[string]JevQuestion) (ansOut map[string]JevAnswer, usageOut JevUsage, errOut error) {
	start := time.Now()
	defer func() {
		latencyMs := float64(time.Since(start).Microseconds()) / 1000.0
		if errOut != nil {
			RecordJev(false, latencyMs, errOut.Error())
		} else {
			RecordJev(true, latencyMs, "")
		}
	}()
	apiKey := j.GetAPIKey()
	if apiKey == "" {
		return nil, JevUsage{}, WrapDecision("jev", cycleID, ErrJevAuth, "TYPESAFE_API_KEY missing")
	}
	body, err := json.Marshal(jevRequest{State: state, Model: j.model, Questions: questions})
	if err != nil {
		return nil, JevUsage{}, WrapDecision("jev", cycleID, ErrJevSchema, "marshal request failed")
	}
	start = time.Now()
	// Detach from caller budget: leftover parent deadlines starved this call
	// (deadline exceeded despite healthy network). Values kept, deadline ours.
	ctx = context.WithoutCancel(ctx)
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, j.http.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.BaseURL()+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return nil, JevUsage{}, WrapDecision("jev", cycleID, ErrJevUnavailable, "request creation failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := j.http.Do(req)
	latency := time.Since(start)
	if err != nil {
		if ctx.Err() != nil || strings.Contains(err.Error(), "deadline") {
			return nil, JevUsage{Latency: latency}, WrapDecision("jev", cycleID, ErrJevTimeout, err.Error())
		}
		return nil, JevUsage{Latency: latency}, WrapDecision("jev", cycleID, ErrJevUnavailable, err.Error())
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, JevUsage{Latency: latency}, WrapDecision("jev", cycleID, ErrJevUnavailable, "read response failed")
	}
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return nil, JevUsage{Latency: latency}, WrapDecision("jev", cycleID, ErrJevAuth, fmt.Sprintf("http %d", resp.StatusCode))
	case resp.StatusCode == 429:
		return nil, JevUsage{Latency: latency}, WrapDecision("jev", cycleID, ErrJevRateLimit, "http 429")
	case resp.StatusCode != 200:
		return nil, JevUsage{Latency: latency}, WrapDecision("jev", cycleID, ErrJevUnavailable, fmt.Sprintf("http %d: %.200s", resp.StatusCode, raw))
	}

	var out jevResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, JevUsage{Latency: latency}, WrapDecision("jev", cycleID, ErrJevSchema, "response JSON invalid")
	}
	if out.Error != nil {
		return nil, JevUsage{Latency: latency}, WrapDecision("jev", cycleID, ErrJevSchema, out.Error.Message)
	}
	// Schema check: every requested question must be present and typed.
	for id := range questions {
		if _, ok := out.Answers[id]; !ok {
			return nil, JevUsage{Latency: latency}, WrapDecision("jev", cycleID, ErrJevSchema, "missing answer: "+id)
		}
	}
	usage := JevUsage{
		InputTokens:  out.Usage.InputTokens,
		OutputTokens: out.Usage.OutputTokens,
		Model:        out.Model,
		Latency:      latency,
	}
	return out.Answers, usage, nil
}

// ValidateChoice ensures a Choice answer carries a legal option and a
// distribution over exactly the criteria keys.
func ValidateChoice(ans JevAnswer, allowed map[string]bool, cycleID string) error {
	if ans.Type != "choice" {
		return WrapDecision("jev", cycleID, ErrJevSchema, "expected choice, got "+ans.Type)
	}
	if !allowed[ans.Choice] {
		return WrapDecision("jev", cycleID, ErrJevSchema, "choice not in vocabulary: "+ans.Choice)
	}
	if len(ans.Probabilities) == 0 {
		return WrapDecision("jev", cycleID, ErrJevSchema, "empty probability distribution")
	}
	var sum float64
	for k, v := range ans.Probabilities {
		if !allowed[k] {
			return WrapDecision("jev", cycleID, ErrJevSchema, "probability key outside vocabulary: "+k)
		}
		sum += v
	}
	if sum < 0.97 || sum > 1.03 {
		return WrapDecision("jev", cycleID, ErrJevSchema, fmt.Sprintf("probabilities sum %.4f != 1", sum))
	}
	return nil
}


// upstreamTransport applies optional UPSTREAM_PROXY_URL (socks5:// or http://)
// to decision-core outbound traffic; nil-safe stdlib default otherwise.
func upstreamTransport() http.RoundTripper {
	base := http.DefaultTransport.(*http.Transport).Clone()
	if p := strings.TrimSpace(os.Getenv("UPSTREAM_PROXY_URL")); p != "" {
		if u, err := url.Parse(p); err == nil && u.Scheme != "" {
			base.Proxy = http.ProxyURL(u)
		}
	}
	return base
}
