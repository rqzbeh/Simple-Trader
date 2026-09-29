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
	"sync"
	"time"
)

// Client interacts with any OpenAI-compatible /v1/chat/completions endpoint.
type Client struct {
	mu         sync.RWMutex
	cfg        ClientConfig
	httpClient *http.Client
}

// SetParams dynamically updates AI model parameters live.
// SetBaseURL updates the OpenAI-compatible gateway base URL live (settings, spec-017).
func (c *Client) SetBaseURL(base string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg.BaseURL = base
}

func (c *Client) SetParams(modelID, reasoningEffort string, temperature float64, timeoutSec int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if modelID != "" {
		c.cfg.ModelID = modelID
	}
	if reasoningEffort != "" {
		c.cfg.ReasoningEffort = reasoningEffort
	}
	if temperature >= 0 {
		c.cfg.Temperature = temperature
	}
	if timeoutSec > 0 {
		c.cfg.TimeoutSec = timeoutSec
		c.httpClient.Timeout = time.Duration(timeoutSec) * time.Second
	}
}

// GetConfig returns a copy of current ClientConfig.
func (c *Client) GetConfig() ClientConfig {
	if c == nil {
		return ClientConfig{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg
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
			Transport: upstreamTransport(),
			Timeout:   timeout,
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
	Stream          *bool           `json:"stream,omitempty"`
	ResponseFormat  *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type       string                `json:"type"`
	JSONSchema *jsonSchemaDefinition `json:"json_schema,omitempty"`
}

type jsonSchemaDefinition struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict,omitempty"`
	Schema json.RawMessage `json:"schema"`
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
Evaluate the single supplied symbol ticker and catalyst against real-time indicators.

CRITICAL MANDATE: NEWS CATALYST FIRST
1. Primary Trade Catalyst: Breaking news headlines, macroeconomic releases, or supply/demand events are mandatory prerequisites to enter any trade.
2. If there is NO significant news catalyst or sentiment is neutral/ambiguous (-0.15 to +0.15), you MUST output "HOLD". Never enter on technical indicators alone (technicals without catalysts produce chop).
3. Directional Bias:
   - Bullish news catalyst (sentiment >= +0.25) -> Evaluate "BUY" (LONG futures contract).
   - Bearish news catalyst (sentiment <= -0.25) -> Evaluate "SELL" (SHORT futures contract).
4. Technical Indicators & Risk/Reward: Use indicators strictly to identify pullback entries, compute tight Stop Loss (0.8%%-1.5%%) and Take Profit (2.0%%-5.0%%) with 2.5:1+ R:R, and calibrate leverage (5x-10x). Signal auto-expires in 60 minutes.

FEW-SHOT EXAMPLES:

Example 1: Long on breaking supply shock
Input: Symbol OIL/USD, Headline: "Major pipeline explosion halts 800k bpd crude transit indefinitely", RSI: 48, SuperTrend: BULLISH
Output:
{
  "evidence": ["Pipeline explosion halts 800k bpd indefinitely", "Severe safe-haven & supply shock catalyst", "RSI 48 neutral pullback with bullish SuperTrend"],
  "reasoning": "High-conviction physical supply disruption validates immediate long. Technicals support pullback entry with tight 1.2%% stop and 3.6%% target (3:1 R:R).",
  "decision": "BUY",
  "confidence": 0.88,
  "catalyst": "Major pipeline explosion halts 800k bpd crude transit indefinitely",
  "leverage": 8,
  "allocation_pct": 1.5,
  "suggested_stop_loss_pct": 1.2,
  "suggested_take_profit_pct": 3.6,
  "regime": "BULL",
  "estimated_win_probability": 0.72
}

Example 2: Hold on ambiguous news
Input: Symbol BTC/USDT, Headline: "Analyst predicts possible market chop ahead of options expiry", RSI: 72, SuperTrend: BULLISH
Output:
{
  "evidence": ["Headline is speculative analyst opinion with no confirmed structural flow", "RSI 72 overbought", "No high-impact catalyst detected"],
  "reasoning": "Speculative opinion piece lacks actionable institutional orderflow. Overbought technicals without breaking fundamental catalyst warrant discipline.",
  "decision": "HOLD",
  "confidence": 0.20,
  "catalyst": "",
  "leverage": 5,
  "allocation_pct": 0.0,
  "suggested_stop_loss_pct": 1.0,
  "suggested_take_profit_pct": 2.5,
  "regime": "RANGING",
  "estimated_win_probability": 0.40
}

CURRENT ADAPTIVE INDICATOR WEIGHTS:
%s

OUTPUT REQUIREMENTS:
Output ONLY raw JSON with keys in exact order: evidence -> reasoning -> decision -> confidence -> catalyst -> leverage -> allocation_pct -> suggested_stop_loss_pct -> suggested_take_profit_pct -> regime -> estimated_win_probability.
DO NOT include markdown code fences (no `+"```"+`), DO NOT include conversational preamble.`, weightsStr.String())
}

// BuildUserPrompt presents the real-time ticker and indicator snapshot.
func (c *Client) BuildUserPrompt(req DecisionRequest) string {
	newsSection := "No recent market news headlines available (NEUTRAL / NO CATALYST)."
	if len(req.NewsHeadlines) > 0 {
		newsSection = "- " + strings.Join(req.NewsHeadlines, "\n- ")
	}

	// Clustered catalyst events (spec 012 US3, T036): the deduplicated
	// story with its coverage count, fused score and freshness replaces the
	// old flat list of syndicated duplicates.
	catalystSection := ""
	if len(req.CatalystEvents) > 0 {
		var b strings.Builder
		for _, e := range req.CatalystEvents {
			sources := "unknown"
			if len(e.Sources) > 0 {
				sources = strings.Join(e.Sources, ", ")
			}
			fmt.Fprintf(&b,
				"- CATALYST EVENT: \"%s\" | story_count=%d (syndicated coverage) | fused_sentiment=%+.3f | freshness=%.2f | sources: %s\n",
				e.Headline, e.StoryCount, e.FusedScore, e.Freshness, sources)
		}
		catalystSection = b.String()
	}

	sentimentSection := "PRE-COMPUTED NLP SENTIMENT: unavailable (treat headlines as unscored; do not invent scores)."
	if req.NewsSentiment != nil {
		phrases := "none detected"
		if len(req.NewsSentiment.KeyPhrases) > 0 {
			phrases = strings.Join(req.NewsSentiment.KeyPhrases, ", ")
		}
		sentimentSection = fmt.Sprintf(
			"PRE-COMPUTED NLP SENTIMENT: score %+.3f (%s) over %d headline(s): %d bullish / %d bearish. Trigger phrases: %s.",
			req.NewsSentiment.Score, req.NewsSentiment.Polarity,
			req.NewsSentiment.HeadlineCount, req.NewsSentiment.BullishCount,
			req.NewsSentiment.BearishCount, phrases,
		)
	}

	return fmt.Sprintf(`Analyze real-time market telemetry snapshot with News-First Catalyst Priority:
ASSET: %s (Fund Tier Bucket: %s)
CURRENT PRICE: %.4f (24h Price Change: %+.2f%%)

REAL-TIME BREAKING NEWS & CATALYST HEADLINES (Primary Entry Prerequisite):
%s

%s
%s
Use the pre-computed sentiment as the starting point, then apply the EQUITY-RESEARCH DISCIPLINE from the system prompt: triage each headline (Near-term / Medium-term / Noise), keep only Near-term catalysts, and HOLD when none qualify.

TECHNICAL INDICATOR SNAPSHOT (For Entry Optimization, SL/TP Levels, and Leverage Factor Only):
- RSI (14): %.2f
- SuperTrend Indicator: %s
- MACD Histogram: %+.4f
- Multi-Indicator Confluence Score: %.2f
- Market Regime: %s (VolRatio: %.2f)
- Institutional Garman-Klass Volatility: %.4f
- Institutional Parkinson Volatility: %.4f
- Kaufman Efficiency Ratio (KER 10): %.2f (Trend signal-to-noise)
- Chaikin Money Flow (CMF 20): %+.4f (Institutional accumulation/distribution)
- Normalized ATR (NATR): %.2f%%

Analyze catalyst priority first. If no high-conviction news catalyst exists, output "HOLD". If a catalyst exists, evaluate direction (BUY for Long, SELL for Short), calibrate isolated leverage (5x-10x), and tight SL/TP sized for the %d-minute holding horizon of this bucket. Output strict JSON.`,
		req.Symbol, req.Bucket, req.Quote.Price, req.Quote.Change24h,
		newsSection, catalystSection, sentimentSection,
		req.IndicatorSnap.RSI, req.IndicatorSnap.SuperTrend, req.IndicatorSnap.Histogram,
		req.IndicatorSnap.ConfluenceScore,
		req.IndicatorSnap.Regime, req.IndicatorSnap.VolRatio,
		req.IndicatorSnap.GarmanKlass, req.IndicatorSnap.Parkinson,
		req.IndicatorSnap.KaufmanER, req.IndicatorSnap.CMF, req.IndicatorSnap.NATR,
		req.HorizonMinutes)
}

// Analyze requests trade analysis from the OpenAI-compatible engine with heuristic fallback.
func (c *Client) Analyze(ctx context.Context, req DecisionRequest) (respOut *DecisionResponse, errOut error) {
	start := time.Now()
	defer func() {
		latencyMs := float64(time.Since(start).Microseconds()) / 1000.0
		if errOut != nil {
			RecordGateway(false, latencyMs, errOut.Error())
		} else {
			RecordGateway(true, latencyMs, "")
		}
	}()

	c.mu.RLock()
	cfg := c.cfg
	httpClient := c.httpClient
	c.mu.RUnlock()

	if cfg.BaseURL == "" || cfg.APIKey == "" {
		return nil, WrapDecision("llm", req.Symbol, ErrLLMClassify, "AI client unconfigured (missing BaseURL or APIKey)")
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimRight(cfg.BaseURL, "/"))
	if !strings.HasSuffix(cfg.BaseURL, "/v1") && !strings.Contains(cfg.BaseURL, "/v1/") {
		url = fmt.Sprintf("%s/v1/chat/completions", strings.TrimRight(cfg.BaseURL, "/"))
	}

	streamFalse := false
	body := openAIChatRequest{
		Model:           cfg.ModelID,
		Temperature:     &cfg.Temperature,
		ReasoningEffort: cfg.ReasoningEffort,
		ToolChoice:      "none",
		Stream:          &streamFalse,
		Messages: []openAIMessage{
			{Role: "system", Content: c.BuildSystemPrompt(req)},
			{Role: "user", Content: c.BuildUserPrompt(req)},
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, WrapDecision("llm", req.Symbol, fmt.Errorf("%w: marshal request: %v", ErrLLMClassify, err), "request marshal failed")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, WrapDecision("llm", req.Symbol, fmt.Errorf("%w: create request: %v", ErrLLMClassify, err), "http request creation failed")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.APIKey))

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, WrapDecision("llm", req.Symbol, fmt.Errorf("%w: gateway network failure %s: %v", ErrLLMClassify, url, err), "network error")
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, WrapDecision("llm", req.Symbol, fmt.Errorf("%w: read response: %v", ErrLLMClassify, err), "response read failed")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, WrapDecision("llm", req.Symbol, ErrLLMClassify, fmt.Sprintf("http status %d from %s", resp.StatusCode, url))
	}

	var content string
	trimmedBody := bytes.TrimSpace(respBody)

	if bytes.HasPrefix(trimmedBody, []byte("data:")) || bytes.Contains(trimmedBody, []byte("\ndata:")) {
		// Gateway returned SSE streaming format; aggregate chunk content deltas
		var sb strings.Builder
		for _, line := range strings.Split(string(respBody), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if payload == "[DONE]" || payload == "" {
					continue
				}
				var chunk struct {
					Choices []struct {
						Delta struct {
							Content string `json:"content"`
						} `json:"delta"`
					} `json:"choices"`
				}
				if err := json.Unmarshal([]byte(payload), &chunk); err == nil {
					if len(chunk.Choices) > 0 {
						sb.WriteString(chunk.Choices[0].Delta.Content)
					}
				}
			}
		}
		content = sb.String()
	} else {
		var chatResp openAIChatResponse
		if err := json.Unmarshal(respBody, &chatResp); err != nil {
			return nil, WrapDecision("llm", req.Symbol, fmt.Errorf("%w: unmarshal response: %v", ErrLLMClassify, err), "malformed JSON")
		}
		if len(chatResp.Choices) == 0 {
			return nil, WrapDecision("llm", req.Symbol, ErrLLMClassify, "response has 0 choices")
		}
		content = chatResp.Choices[0].Message.Content
	}

	content = StripCodeFence(content)

	var decision DecisionResponse
	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		return nil, WrapDecision("llm", req.Symbol, fmt.Errorf("%w: parse decision: %v", ErrLLMClassify, err), "decision JSON invalid")
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
	if decision.Leverage < 1 || decision.Leverage > cfg.DefaultLeverage {
		if cfg.DefaultLeverage > 0 {
			decision.Leverage = cfg.DefaultLeverage
		} else {
			decision.Leverage = 8
		}
	}
	if decision.SuggestedStopLossPct <= 0 || decision.SuggestedStopLossPct > cfg.MaxStopLossPct {
		decision.SuggestedStopLossPct = cfg.MinStopLossPct
		if decision.SuggestedStopLossPct <= 0 {
			decision.SuggestedStopLossPct = 1.0
		}
	}
	if decision.SuggestedTakeProfitPct <= 0 || decision.SuggestedTakeProfitPct > cfg.MaxTakeProfitPct {
		decision.SuggestedTakeProfitPct = cfg.MinTakeProfitPct
		if decision.SuggestedTakeProfitPct <= 0 {
			decision.SuggestedTakeProfitPct = 3.0
		}
	}

	log.Printf("[AI-CORE] OmniRoute %s signal for %s: %s (Confidence: %.2f, WinProb: %.2f, Regime: %s, Lev: %dx, SL: %.2f%%, TP: %.2f%%) reasoning=%q",
		cfg.ModelID, req.Symbol, decision.Decision, decision.Confidence, decision.EstimatedWinProbability, decision.Regime,
		decision.Leverage, decision.SuggestedStopLossPct, decision.SuggestedTakeProfitPct, decision.Reasoning)

	return &decision, nil
}
