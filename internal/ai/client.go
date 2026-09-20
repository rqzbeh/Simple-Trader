package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client interacts with any OpenAI-compatible /v1/chat/completions endpoint.
type Client struct {
	cfg        ClientConfig
	httpClient *http.Client
}

// NewClient creates a new unified OpenAI AI client.
func NewClient(cfg ClientConfig) *Client {
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatRequest struct {
	Model          string          `json:"model"`
	Messages       []openAIMessage `json:"messages"`
	Temperature    float64         `json:"temperature"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// BuildSystemPrompt constructs the in-context learning system prompt including current dynamic weights.
func (c *Client) BuildSystemPrompt(req DecisionRequest) string {
	var weightsStr strings.Builder
	for k, v := range req.Weights {
		weightsStr.WriteString(fmt.Sprintf("- %s: %.2f\n", k, v))
	}

	return fmt.Sprintf(`You are Simple-Trader's AI Quantitative Strategy Core.
Your mission is strict risk-adjusted capital appreciation on global Core & Alpha assets.
Iranian assets are completely disabled. Focus on liquid assets.

Dynamic Indicator Weights (Learned from recent trade outcomes):
%s

Output ONLY valid JSON matching this schema:
{
  "decision": "BUY" | "SELL" | "HOLD",
  "confidence": <float between 0.0 and 1.0>,
  "reasoning": "<concise quantitative analysis>",
  "suggested_stop_loss_pct": <float>,
  "suggested_take_profit_pct": <float>,
  "regime": "BULL" | "BEAR" | "RANGING"
}`, weightsStr.String())
}

// BuildUserPrompt presents the real-time ticker and indicator snapshot.
func (c *Client) BuildUserPrompt(req DecisionRequest) string {
	newsSection := "None"
	if len(req.NewsHeadlines) > 0 {
		newsSection = strings.Join(req.NewsHeadlines, "\n- ")
	}

	return fmt.Sprintf(`Analyze market snapshot:
Asset: %s (Bucket: %s)
Current Price: %.4f (24h Change: %.2f%%)
Technical Snapshot:
- RSI: %.2f
- SuperTrend: %s
- MACD Histogram: %.4f
- Confluence Score: %.2f

Recent News Headlines:
%s

Provide your JSON decision.`,
		req.Symbol, req.Bucket, req.Quote.Price, req.Quote.Change24h,
		req.IndicatorSnap.RSI, req.IndicatorSnap.SuperTrend, req.IndicatorSnap.Histogram,
		req.IndicatorSnap.ConfluenceScore, newsSection)
}

// Analyze requests trade analysis from the OpenAI-compatible engine with heuristic fallback.
func (c *Client) Analyze(ctx context.Context, req DecisionRequest) (*DecisionResponse, error) {
	if c.cfg.BaseURL == "" || c.cfg.APIKey == "" {
		// Fall back to pure quantitative heuristic
		return c.fallbackHeuristic(req), nil
	}

	url := fmt.Sprintf("%s/v1/chat/completions", strings.TrimRight(c.cfg.BaseURL, "/"))
	body := openAIChatRequest{
		Model:       c.cfg.ModelID,
		Temperature: c.cfg.Temperature,
		Messages: []openAIMessage{
			{Role: "system", Content: c.BuildSystemPrompt(req)},
			{Role: "user", Content: c.BuildUserPrompt(req)},
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal openai request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.cfg.APIKey))

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		// Heuristic fallback on network or API failure
		return c.fallbackHeuristic(req), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.fallbackHeuristic(req), nil
	}

	var chatResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return c.fallbackHeuristic(req), nil
	}

	if len(chatResp.Choices) == 0 {
		return c.fallbackHeuristic(req), nil
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	// Strip markdown json fences if present
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var decision DecisionResponse
	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		return c.fallbackHeuristic(req), nil
	}

	return &decision, nil
}

// fallbackHeuristic provides deterministic fallback logic when LLM is unavailable.
func (c *Client) fallbackHeuristic(req DecisionRequest) *DecisionResponse {
	snap := req.IndicatorSnap
	decision := "HOLD"
	confidence := 0.5

	if snap.SuperTrend == "BULL" && snap.RSI > 50 && snap.Histogram > 0 {
		decision = "BUY"
		confidence = 0.75
	} else if snap.SuperTrend == "BEAR" && snap.RSI < 50 && snap.Histogram < 0 {
		decision = "SELL"
		confidence = 0.75
	}

	return &DecisionResponse{
		Decision:               decision,
		Confidence:             confidence,
		Reasoning:              "Deterministic technical confluence fallback execution.",
		SuggestedStopLossPct:   1.5,
		SuggestedTakeProfitPct: 3.0,
		Regime:                 snap.SuperTrend,
	}
}
