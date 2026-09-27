package ai

import (
	"context"
	"io"
	"net/http"
	"encoding/json"
	"fmt"
	"strings"
)

// NewsLabel vocabulary (spec-013 FR-004).
var NewsLabelAllowed = map[string]bool{"BULLISH": true, "BEARISH": true, "NEUTRAL": true, "MIXED": true}

// ClassifyNewsResult is the structured classifier output.
type ClassifyNewsResult struct {
	Evidence   []string `json:"evidence"`
	Reasoning  string   `json:"reasoning"`
	Label      string   `json:"label"`
	Confidence float64  `json:"confidence"`
}

// newsClassifySchema: field order evidence → reasoning → label
// (research.md: label-last prevents post-hoc rationalization in decoding).
const newsClassifySchema = `{
  "type": "object",
  "properties": {
    "evidence": {"type": "array", "items": {"type": "string"}},
    "reasoning": {"type": "string"},
    "label": {"type": "string", "enum": ["BULLISH","BEARISH","NEUTRAL","MIXED"]},
    "confidence": {"type": "number"}
  },
  "required": ["evidence","reasoning","label","confidence"],
  "additionalProperties": false
}`

// newsSystemPrompt: label definitions with positive AND negative anchors,
// negation and slang rules, balanced examples (research.md rules).
const newsSystemPrompt = `You are a crypto/futures news impact classifier.
Decide the net market impact of ONE headline cluster over a 0-24h horizon.

Labels (exactly one):
- BULLISH: net positive price impact expected. Examples: "ETF inflows hit record", "central bank cuts rates".
  NOT bullish when negated: "inflows stall", "no rate cut expected".
- BEARISH: net negative price impact expected. Examples: "exchange hack", "regulator sues issuer".
  NOT bearish when reversed: "SEC drops lawsuit", "hack recovered, funds reimbursed".
- NEUTRAL: no material impact expected. Examples: "company hires new CFO".
- MIXED: materially conflicting signals in the same cluster.

Rules:
1. Handle negation: "no", "not", "fails to", "drops plan" invert polarity.
2. Handle crypto slang: "wagmi/lfg" bullish tone, "rug/jeet" bearish, "gm" neutral.
3. Extract evidence spans before deciding. Report your confidence 0..1 honestly.
4. Never guess from topic alone — impact, not subject, decides the label.`

// newsUserPrompt balances few-shot across classes inside the user turn.
const newsUserPromptTemplate = `Classify this headline cluster for symbol context %q:

%s

Respond as JSON with keys evidence, reasoning, label, confidence (in that order).`

// ClassifyNews runs the structured-output classifier over one cluster.
// Explicit errors only — no neutral fabrication (FR-007).
func (c *Client) ClassifyNews(ctx context.Context, symbol string, headlines []string) (ClassifyNewsResult, error) {
	if c == nil || c.cfg.BaseURL == "" || c.cfg.APIKey == "" {
		return ClassifyNewsResult{}, WrapDecision("news-classifier", symbol, ErrLLMClassify, "client unconfigured (no fallback)")
	}
	if len(headlines) == 0 {
		return ClassifyNewsResult{}, WrapDecision("news-classifier", symbol, ErrLLMClassify, "empty headline cluster")
	}
	var sb strings.Builder
	for _, h := range headlines {
		sb.WriteString("- ")
		sb.WriteString(h)
		sb.WriteString("\n")
	}
	f := false
	req := openAIChatRequest{
		Model:     c.cfg.ModelID,
		Stream:    &f,
		ToolChoice: "none",
		Messages: []openAIMessage{
			{Role: "system", Content: newsSystemPrompt},
			{Role: "user", Content: fmt.Sprintf(newsUserPromptTemplate, symbol, sb.String())},
		},
		ResponseFormat: &responseFormat{Type: "json_object"},
	}
	out, err := c.completeJSON(ctx, req)
	if err != nil {
		return ClassifyNewsResult{}, WrapDecision("news-classifier", symbol, err, "completion failed")
	}
	out = stripCodeFence(out)
	var res ClassifyNewsResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return ClassifyNewsResult{}, WrapDecision("news-classifier", symbol, fmt.Errorf("%w: %v", ErrLLMClassify, err), "structured output invalid")
	}
	if !NewsLabelAllowed[res.Label] {
		return ClassifyNewsResult{}, WrapDecision("news-classifier", symbol, ErrLLMClassify, "label outside vocabulary: "+res.Label)
	}
	return res, nil
}

// stripCodeFence removes markdown fences some gateway models wrap around JSON.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```JSON")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	if i := strings.Index(s, "{"); i > 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			s = s[i : j+1]
		}
	}
	return strings.TrimSpace(s)
}

// completeJSON posts a chat request and returns the assistant content.
func (c *Client) completeJSON(ctx context.Context, req openAIChatRequest) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("%w: marshal: %v", ErrLLMClassify, err)
	}
	if c.cfg.BaseURL == "" || c.cfg.APIKey == "" {
		return "", ErrLLMClassify
	}
	url := c.cfg.BaseURL + "/chat/completions"
	if !strings.Contains(c.cfg.BaseURL, "/v1") {
		url = c.cfg.BaseURL + "/v1/chat/completions"
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(data)))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return "", ErrLLMTimeout
		}
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("%w: http %d", ErrLLMClassify, resp.StatusCode)
	}
	var chatResp openAIChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("%w: response JSON: %v", ErrLLMClassify, err)
	}
	if len(chatResp.Choices) == 0 {
		return "", ErrLLMClassify
	}
	return chatResp.Choices[0].Message.Content, nil
}
