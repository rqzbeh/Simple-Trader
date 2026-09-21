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

	return fmt.Sprintf(`You are Simple-Trader's Institutional Quantitative Risk & Two-Sided Futures Execution Core.
You operate as a Senior Hedge Fund Portfolio Manager and Quantitative Risk Officer governing a 3-Tier Multi-Horizon Capital Structure:

PORTFOLIO ARCHITECTURE & CAPITAL MANDATE:
- Tier 1 (15-25%% Cash Reserve): Absolute liquidity buffer dedicated solely to zero-slippage investor redemptions. Strictly NEVER allocate or risk funds from Tier 1.
- Tier 2 (40-60%% Core Wealth Preservation): Strategic macro store-of-value assets (Gold XAU/USD, Silver XAG/USD) grounded in monetary base expansion, inflation hedging, and real-yield compression.
- Tier 3 (15-40%% Tactical Alpha): High-turnover liquid assets (evaluated on 2-hour swing candlesticks and high-volume crypto pairs >$50M 24h turnover, <10 bps spread). Realized profits are systematically swept into Tier 1 cash buffer.

STRICT COMPLIANCE DIRECTIVE:
All Iranian assets and instruments are strictly disabled and prohibited. Focus exclusively on verified global liquid pairs.

CRITICAL ARCHITECTURAL MANDATE: NEWS CATALYST FIRST
1. Primary Trade Catalyst: Breaking news headlines, macroeconomic events, whale exchange deposits/withdrawals, or geopolitical developments are the SOLE VALID PREREQUISITES to enter any trade.
2. If there is NO significant news catalyst or the sentiment is neutral/ambiguous (-0.15 to +0.15), you MUST output "HOLD". NEVER trigger a trade purely because technical indicators show overbought, oversold, or trending conditions! Technicals without catalysts produce chop.
3. Two-Sided Futures Trading: The market is two-sided.
   - Bullish news catalyst (sentiment >= +0.25) -> Evaluate "BUY" (LONG futures contract).
   - Bearish news catalyst (sentiment <= -0.25) -> Evaluate "SELL" (SHORT futures contract).
4. Role of Technical Indicators (2-Hour Intraday Horizon): Technical indicators (RSI, SuperTrend, MACD, Bollinger Bands, Order Book Confluence) MUST be used STRICTLY to:
   - Identify pullback entry pricing on 2-hour candles (do not chase green/red spikes).
   - Calculate tight Stop Loss (0.8%% to 1.5%% from entry) and ambitious Take Profit (2.0%% to 4.5%% from entry) enforcing Risk-to-Reward (R:R) between 2.5:1 and 3:1.
   - Calibrate isolated margin leverage between 5x and 10x (default 8x for liquid crypto futures). Trades must produce meaningful leveraged ROI (20%% to 40%%+ return on margin) to comfortably exceed transaction costs and justify market risk.
5. Capital Sizing & Allocation: Account sizes start at $100 up to institutional scale. Suggest allocation_pct as percent of available tactical alpha (default 1.0%% to 2.0%% risk per trade, ensuring margin required is sustainable and bounded within Tier 3 Tactical Alpha).

CURRENT ADAPTIVE INDICATOR WEIGHTS (Calibrated via Thompson Sampling / Regret Minimization):
%s

OUTPUT REQUIREMENTS:
Output ONLY a single valid, raw JSON object strictly adhering to the schema below.
DO NOT include markdown code fences (no `+"```"+`), DO NOT include conversational preamble, DO NOT invoke any tools.

SCHEMA:
{
  "decision": "BUY" | "SELL" | "HOLD",
  "confidence": <float between 0.0 and 1.0>,
  "reasoning": "<concise institutional quantitative analysis referencing primary news catalyst, technical entry/exit calibration, and risk/reward>",
  "catalyst": "<headline or catalyst summary that triggered this decision, or empty if HOLD>",
  "leverage": <integer between 5 and 10>,
  "allocation_pct": <float between 0.5 and 2.0>,
  "suggested_stop_loss_pct": <float between 0.8 and 1.5>,
  "suggested_take_profit_pct": <float between 2.0 and 5.0>,
  "regime": "BULL" | "BEAR" | "RANGING",
  "estimated_win_probability": <float between 0.0 and 1.0>
}`, weightsStr.String())
}

// BuildUserPrompt presents the real-time ticker and indicator snapshot.
func (c *Client) BuildUserPrompt(req DecisionRequest) string {
	newsSection := "No recent market news headlines available (NEUTRAL / NO CATALYST)."
	if len(req.NewsHeadlines) > 0 {
		newsSection = "- " + strings.Join(req.NewsHeadlines, "\n- ")
	}

	return fmt.Sprintf(`Analyze real-time market telemetry snapshot with News-First Catalyst Priority:
ASSET: %s (Fund Tier Bucket: %s)
CURRENT PRICE: %.4f (24h Price Change: %+.2f%%)

REAL-TIME BREAKING NEWS & CATALYST HEADLINES (Primary Entry Prerequisite):
%s

TECHNICAL INDICATOR SNAPSHOT (For Entry Optimization, SL/TP Levels, and Leverage Factor Only):
- RSI (14): %.2f
- SuperTrend Indicator: %s
- MACD Histogram: %+.4f
- Multi-Indicator Confluence Score: %.2f

Analyze catalyst priority first. If no high-conviction news catalyst exists, output "HOLD". If a catalyst exists, evaluate direction (BUY for Long, SELL for Short), calibrate isolated leverage (5x-10x), and tight SL/TP with 2-hour swing R:R between 2.5:1 and 3:1. Output strict JSON.`,
		req.Symbol, req.Bucket, req.Quote.Price, req.Quote.Change24h,
		newsSection,
		req.IndicatorSnap.RSI, req.IndicatorSnap.SuperTrend, req.IndicatorSnap.Histogram,
		req.IndicatorSnap.ConfluenceScore)
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
	if decision.Leverage < 5 || decision.Leverage > 10 {
		decision.Leverage = 8 // Default 8x isolated leverage for 2h intraday crypto setups
	}
	if decision.SuggestedStopLossPct <= 0 || decision.SuggestedStopLossPct > 3.0 {
		decision.SuggestedStopLossPct = 1.0 // 1.0% stop loss for 2h horizon
	}
	if decision.SuggestedTakeProfitPct <= 0 || decision.SuggestedTakeProfitPct > 10.0 {
		decision.SuggestedTakeProfitPct = 3.0 // 3.0% take profit (3:1 R:R target)
	}

	log.Printf("[AI-CORE] OmniRoute %s signal for %s: %s (Confidence: %.2f, WinProb: %.2f, Regime: %s, Lev: %dx, SL: %.2f%%, TP: %.2f%%)",
		c.cfg.ModelID, req.Symbol, decision.Decision, decision.Confidence, decision.EstimatedWinProbability, decision.Regime,
		decision.Leverage, decision.SuggestedStopLossPct, decision.SuggestedTakeProfitPct)

	return &decision, nil
}

// fallbackHeuristic provides deterministic fallback logic when LLM is unavailable.
// Enforces news catalyst first: if no headlines exist, outputs HOLD.
func (c *Client) fallbackHeuristic(req DecisionRequest) *DecisionResponse {
	snap := req.IndicatorSnap
	decision := "HOLD"
	confidence := 0.5
	catalyst := ""
	reasoning := "No high-impact breaking news catalyst detected. Preserving capital in HOLD state."
	lev := 8
	alloc := 0.0

	// Require at least one non-empty news headline as a catalyst
	hasCatalyst := len(req.NewsHeadlines) > 0 && strings.TrimSpace(req.NewsHeadlines[0]) != ""
	if hasCatalyst {
		catalyst = req.NewsHeadlines[0]
		lowerHeadline := strings.ToLower(catalyst)

		isBullish := strings.Contains(lowerHeadline, "surge") ||
			strings.Contains(lowerHeadline, "inflow") ||
			strings.Contains(lowerHeadline, "rally") ||
			strings.Contains(lowerHeadline, "accumulat") ||
			strings.Contains(lowerHeadline, "bull") ||
			strings.Contains(lowerHeadline, "approved") ||
			strings.Contains(lowerHeadline, "record") ||
			strings.Contains(lowerHeadline, "breakout")

		isBearish := strings.Contains(lowerHeadline, "dump") ||
			strings.Contains(lowerHeadline, "ban") ||
			strings.Contains(lowerHeadline, "crash") ||
			strings.Contains(lowerHeadline, "lawsuit") ||
			strings.Contains(lowerHeadline, "hack") ||
			strings.Contains(lowerHeadline, "bear") ||
			strings.Contains(lowerHeadline, "liquidation") ||
			strings.Contains(lowerHeadline, "investigation")

		if isBullish {
			decision = "BUY"
			confidence = 0.80
			reasoning = fmt.Sprintf("Bullish catalyst (%s) confirmed by technical momentum.", catalyst)
			lev = 8
			alloc = 1.5
		} else if isBearish {
			decision = "SELL"
			confidence = 0.80
			reasoning = fmt.Sprintf("Bearish catalyst (%s) confirmed by downward technical momentum.", catalyst)
			lev = 8
			alloc = 1.5
		} else {
			decision = "HOLD"
			confidence = 0.50
			reasoning = fmt.Sprintf("Neutral news headline detected (%s). Edge ambiguous, holding.", catalyst)
		}
	}

	return &DecisionResponse{
		Decision:                decision,
		Confidence:              confidence,
		Reasoning:               reasoning,
		Catalyst:                catalyst,
		Leverage:                lev,
		AllocationPct:           alloc,
		SuggestedStopLossPct:    1.0,
		SuggestedTakeProfitPct:  3.0,
		Regime:                  snap.SuperTrend,
		EstimatedWinProbability: confidence * 0.9,
	}
}
