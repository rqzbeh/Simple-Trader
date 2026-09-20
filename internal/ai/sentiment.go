package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rqzbeh/simple-trader/internal/market"
)

// SentimentResult holds quantitative sentiment extraction from news headlines.
type SentimentResult struct {
	Score      float64 `json:"score"`      // -1.0 to 1.0
	Polarity   string  `json:"polarity"`   // BULLISH, BEARISH, NEUTRAL
	Confidence float64 `json:"confidence"` // 0.0 to 1.0
	Summary    string  `json:"summary"`
}

// ExtractNewsSentiment processes news headlines through the LLM or falls back to financial lexicon.
func (c *Client) ExtractNewsSentiment(ctx context.Context, headlines []string) (SentimentResult, error) {
	if len(headlines) == 0 {
		return SentimentResult{
			Score:      0.0,
			Polarity:   "NEUTRAL",
			Confidence: 1.0,
			Summary:    "No news headlines available",
		}, nil
	}

	// If no LLM endpoint configured, use fast financial lexicon parser
	if c == nil || c.cfg.BaseURL == "" || c.cfg.APIKey == "" {
		report := market.AnalyzeNewsSentiment(headlines)
		return SentimentResult{
			Score:      report.Score,
			Polarity:   string(report.Polarity),
			Confidence: 0.85,
			Summary:    fmt.Sprintf("Analyzed %d headlines via lexicon; %d key terms matched", report.HeadlineCount, len(report.KeyPhrases)),
		}, nil
	}

	prompt := fmt.Sprintf(`You are a quantitative macro sentiment analyzer.
Evaluate the following news headlines and score the aggregate market impact:
- Score: float between -1.0 (extremely bearish/panic) and +1.0 (extremely bullish/euphoric)
- Polarity: "BULLISH", "BEARISH", or "NEUTRAL"
- Confidence: float between 0.0 and 1.0
- Summary: 1 sentence summary

Headlines:
- %s

Output ONLY valid JSON matching this schema:
{
  "score": <float>,
  "polarity": "BULLISH" | "BEARISH" | "NEUTRAL",
  "confidence": <float>,
  "summary": "<string>"
}`, strings.Join(headlines, "\n- "))

	chatReq := openAIChatRequest{
		Model: c.cfg.ModelID,
		Messages: []openAIMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
		ResponseFormat: &responseFormat{
			Type: "json_object",
		},
	}

	data, err := json.Marshal(chatReq)
	if err != nil {
		return SentimentResult{}, err
	}

	url := fmt.Sprintf("%s/v1/chat/completions", strings.TrimRight(c.cfg.BaseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return SentimentResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.cfg.APIKey))

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		// Fallback to lexical analyzer on network failure
		report := market.AnalyzeNewsSentiment(headlines)
		return SentimentResult{
			Score:      report.Score,
			Polarity:   string(report.Polarity),
			Confidence: 0.70,
			Summary:    fmt.Sprintf("Fallback lexical analysis (%v)", err),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		report := market.AnalyzeNewsSentiment(headlines)
		return SentimentResult{
			Score:      report.Score,
			Polarity:   string(report.Polarity),
			Confidence: 0.70,
			Summary:    fmt.Sprintf("Fallback lexical analysis (HTTP %d)", resp.StatusCode),
		}, nil
	}

	var chatResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return SentimentResult{}, err
	}

	if len(chatResp.Choices) == 0 {
		return SentimentResult{}, fmt.Errorf("no completion choices returned")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var res SentimentResult
	if err := json.Unmarshal([]byte(content), &res); err != nil {
		return SentimentResult{}, fmt.Errorf("failed to parse sentiment JSON: %w", err)
	}

	return res, nil
}
