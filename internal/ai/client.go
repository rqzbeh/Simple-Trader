package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
		timeout = 30 * time.Second
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.ReasoningEffort == "" {
		cfg.ReasoningEffort = "high"
	}

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
	Model           string          `json:"model"`
	Messages        []openAIMessage `json:"messages"`
	Temperature     *float64        `json:"temperature,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	ToolChoice      string          `json:"tool_choice,omitempty"`
	ResponseFormat  *responseFormat `json:"response_format,omitempty"`
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
		Code    string `json:"code,omitempty"`
	} `json:"error,omitempty"`
}

// BuildSystemPrompt constructs an institutional quantitative trading core system prompt.
func (c *Client) BuildSystemPrompt(req DecisionRequest) string {
	var weightsStr strings.Builder
	if len(req.Weights) > 0 {
		for k, v := range req.Weights {
			weightsStr.WriteString(fmt.Sprintf("- %s: %.2f\n", k, v))
		}
	} else {
		weightsStr.WriteString("- Equal Baseline Weight (1.00)\n")
	}

	return fmt.Sprintf(`You are Simple-Trader's Institutional Quantitative Risk & Systematic Execution Core.
You operate as a Senior Portfolio Manager and Quantitative Risk Officer governing a 3-Tier Multi-Horizon Capital Structure:

PORTFOLIO ARCHITECTURE & CAPITAL MANDATE:
- Tier 1 (15%% Cash Reserve): Absolute liquidity buffer dedicated solely to zero-slippage investor redemptions. Strictly NEVER allocate or risk funds from Tier 1.
- Tier 2 (45%% Core Wealth Preservation): Strategic macro store-of-value assets (Gold XAU/USD, Silver XAG/USD) grounded in monetary base expansion, inflation hedging, and real-yield compression.
- Tier 3 (40%% Tactical Alpha): High-turnover liquid assets (evaluated on 3-hour swing candlesticks and high-volume crypto pairs >$50M 24h turnover, <10 bps spread). Realized profits are systematically swept into Tier 1 cash buffer.

STRICT COMPLIANCE DIRECTIVE:
All Iranian assets and instruments are strictly disabled and prohibited. Focus exclusively on verified global liquid pairs.

QUANTITATIVE EVALUATION PRINCIPLES:
1. Asymmetric Payoff Expectation: Require minimum 2.0:1 Reward-to-Risk ratio on entry signals. If directional edge is ambiguous or volume confirms distribution, output "HOLD".
2. Multi-Signal Confluence: Synthesize technical indicators (SuperTrend trend regime, RSI momentum exhaustion, MACD histogram velocity), 3-hour price action structure, and live market headline sentiment.
3. Liquidity & Slippage Defense: Account for order book spread and turnover. Reject illiquid expansion.
4. Bayesian Weight Attribution: Factor in recent empirical indicator performance weights derived from historical trade outcomes.

CURRENT ADAPTIVE INDICATOR WEIGHTS (Calibrated via Thompson Sampling / Regret Minimization):
%s

OUTPUT REQUIREMENTS:
Output ONLY a single valid, raw JSON object strictly adhering to the schema below.
DO NOT include markdown code fences (no `+"```"+`), DO NOT include conversational preamble, DO NOT invoke any tools.

SCHEMA:
{
  "decision": "BUY" | "SELL" | "HOLD",
  "confidence": <float between 0.0 and 1.0>,
  "reasoning": "<concise institutional quantitative analysis referencing technicals, news sentiment, and risk/reward>",
  "suggested_stop_loss_pct": <float between 0.5 and 5.0>,
  "suggested_take_profit_pct": <float between 1.0 and 15.0>,
  "regime": "BULL" | "BEAR" | "RANGING",
  "estimated_win_probability": <float between 0.0 and 1.0>
}`, weightsStr.String())
}

// BuildUserPrompt presents the real-time ticker and indicator snapshot.
func (c *Client) BuildUserPrompt(req DecisionRequest) string {
	newsSection := "No recent market news headlines."
	if len(req.NewsHeadlines) > 0 {
		newsSection = "- " + strings.Join(req.NewsHeadlines, "\n- ")
	}

	return fmt.Sprintf(`Analyze real-time market telemetry snapshot:
ASSET: %s (Fund Tier Bucket: %s)
CURRENT PRICE: %.4f (24h Price Change: %+.2f%%)

TECHNICAL INDICATOR SNAPSHOT:
- RSI (14): %.2f
- SuperTrend Indicator: %s
- MACD Histogram: %+.4f
- Multi-Indicator Confluence Score: %.2f

REAL-TIME NEWS & SENTIMENT:
%s

Synthesize the technical momentum, news sentiment, and risk regime. Output strict JSON.`,
		req.Symbol, req.Bucket, req.Quote.Price, req.Quote.Change24h,
		req.IndicatorSnap.RSI, req.IndicatorSnap.SuperTrend, req.IndicatorSnap.Histogram,
		req.IndicatorSnap.ConfluenceScore, newsSection)
}

// Analyze requests trade analysis from the OpenAI-compatible engine with heuristic fallback.
func (c *Client) Analyze(ctx context.Context, req DecisionRequest) (*DecisionResponse, error) {
	if c.cfg.BaseURL == "" || c.cfg.APIKey == "" {
		log.Printf("[INFO] AI client unconfigured (missing BaseURL or APIKey). Using quantitative fallback heuristic.")
		return c.fallbackHeuristic(req), nil
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimRight(c.cfg.BaseURL, "/"))
	if !strings.HasSuffix(c.cfg.BaseURL, "/v1") && !strings.Contains(c.cfg.BaseURL, "/v1/") {
		url = fmt.Sprintf("%s/v1/chat/completions", strings.TrimRight(c.cfg.BaseURL, "/"))
	}

	body := openAIChatRequest{
		Model:           c.cfg.ModelID,
		Temperature:     &c.cfg.Temperature,
		ReasoningEffort: c.cfg.ReasoningEffort,
		ToolChoice:      "none",
		Messages: []openAIMessage{
			{Role: "system", Content: c.BuildSystemPrompt(req)},
			{Role: "user", Content: c.BuildUserPrompt(req)},
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		log.Printf("[WARN] AI request marshal error: %v. Using fallback heuristic.", err)
		return c.fallbackHeuristic(req), nil
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		log.Printf("[WARN] AI HTTP request creation error: %v. Using fallback heuristic.", err)
		return c.fallbackHeuristic(req), nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.cfg.APIKey))

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		log.Printf("[WARN] AI gateway network failure (%s): %v. Using fallback heuristic.", url, err)
		return c.fallbackHeuristic(req), nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[WARN] AI response read failure: %v. Using fallback heuristic.", err)
		return c.fallbackHeuristic(req), nil
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[WARN] AI gateway HTTP %d from %s: %s. Using fallback heuristic.", resp.StatusCode, url, string(respBody))
		return c.fallbackHeuristic(req), nil
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		log.Printf("[WARN] AI response JSON unmarshal error: %v. Body: %s. Using fallback.", err, string(respBody))
		return c.fallbackHeuristic(req), nil
	}

	if len(chatResp.Choices) == 0 {
		log.Printf("[WARN] AI response returned 0 choices. Using fallback.")
		return c.fallbackHeuristic(req), nil
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	// Strip markdown code fences if present
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	// Extract JSON between first '{' and last '}'
	startIdx := strings.Index(content, "{")
	endIdx := strings.LastIndex(content, "}")
	if startIdx >= 0 && endIdx > startIdx {
		content = content[startIdx : endIdx+1]
	}

	var decision DecisionResponse
	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		log.Printf("[WARN] Failed to parse AI decision JSON: %v. Content: %s. Using fallback.", err, content)
		return c.fallbackHeuristic(req), nil
	}

	// Validate decision output bounds
	if decision.Decision != "BUY" && decision.Decision != "SELL" && decision.Decision != "HOLD" {
		decision.Decision = "HOLD"
	}
	if decision.Confidence < 0 {
		decision.Confidence = 0.5
	} else if decision.Confidence > 1.0 {
		decision.Confidence = 1.0
	}
	if decision.SuggestedStopLossPct <= 0 {
		decision.SuggestedStopLossPct = 1.5
	}
	if decision.SuggestedTakeProfitPct <= 0 {
		decision.SuggestedTakeProfitPct = 3.0
	}

	log.Printf("[AI-CORE] OmniRoute %s signal for %s: %s (Confidence: %.2f, WinProb: %.2f, Regime: %s)",
		c.cfg.ModelID, req.Symbol, decision.Decision, decision.Confidence, decision.EstimatedWinProbability, decision.Regime)

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
		Decision:                decision,
		Confidence:              confidence,
		Reasoning:               "Deterministic technical confluence fallback execution.",
		SuggestedStopLossPct:    1.5,
		SuggestedTakeProfitPct:  3.0,
		Regime:                  snap.SuperTrend,
		EstimatedWinProbability: confidence * 0.9,
	}
}
