package ai

import (
	"encoding/json"
	"fmt"
)

// FineTunePair represents a single prompt-completion record formatted for OpenAI/vLLM fine-tuning.
type FineTunePair struct {
	SystemPrompt      string                 `json:"system_prompt"`
	UserPrompt        string                 `json:"user_prompt"`
	AssistantResponse string                 `json:"assistant_response"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

type chatMLMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatMLPayload struct {
	Messages []chatMLMessage `json:"messages"`
}

// ToJSONL formats the pair into a single ChatML JSON line for OpenAI fine-tuning dataset export.
func (p *FineTunePair) ToJSONL() (string, error) {
	payload := chatMLPayload{
		Messages: []chatMLMessage{
			{Role: "system", Content: p.SystemPrompt},
			{Role: "user", Content: p.UserPrompt},
			{Role: "assistant", Content: p.AssistantResponse},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ChatML JSONL: %w", err)
	}

	return string(data), nil
}
