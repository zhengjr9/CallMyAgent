package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const metaSystemPrompt = `You are a development task planning assistant. Your job is to help the user refine their development requirements through conversation, then produce a complete, detailed prompt that will be given to Claude Code to execute the actual development work.

Your workflow:
1. Ask clarifying questions about the task scope, tech stack, constraints
2. Understand the repository structure and existing code patterns
3. After 2-5 rounds of conversation, produce a FINAL prompt

The FINAL prompt should:
- Be self-contained (no ambiguity)
- Include specific file paths, function names, patterns to follow
- Specify the exact deliverables expected
- Be wrapped in <final-prompt>...</final-prompt> tags

When you output the final prompt inside <final-prompt> tags, also say "任务规划完成" (or "Task planning complete") so the system knows to stop the conversation.

Always respond in Chinese.`

func CallClaudeAPI(cfg *Config, messages []Message) (string, error) {
	baseURL := cfg.AnthropicBaseURL
	if cfg.ClaudeBaseURL != "" {
		baseURL = cfg.ClaudeBaseURL
	}

	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-5-20250514"
	}

	reqBody := AnthropicRequest{
		Model:     model,
		MaxTokens: 4096,
		System:    metaSystemPrompt,
		Messages:  messages,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := baseURL + "/v1/messages"
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	// Support both API key and auth token
	if cfg.AnthropicAPIKey != "" {
		httpReq.Header.Set("x-api-key", cfg.AnthropicAPIKey)
	}
	if cfg.ClaudeAPIToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.ClaudeAPIToken)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp AnthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(apiResp.Content) == 0 {
		return "", fmt.Errorf("empty response content")
	}

	// Find first text content
	for _, c := range apiResp.Content {
		if c.Text != "" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("no text content in response")
}
