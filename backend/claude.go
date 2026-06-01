package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

func CallClaudeAPIStream(cfg *Config, messages []Message, onDelta func(string) error) (string, error) {
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
		Stream:    true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", baseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
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

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var evt struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			ContentBlock struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content_block"`
			Error *struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}
		if evt.Error != nil {
			return full.String(), fmt.Errorf("%s: %s", evt.Error.Type, evt.Error.Message)
		}

		text := evt.Delta.Text
		if text == "" {
			text = evt.ContentBlock.Text
		}
		if text == "" {
			continue
		}
		full.WriteString(text)
		if err := onDelta(text); err != nil {
			return full.String(), err
		}
	}
	if err := scanner.Err(); err != nil {
		return full.String(), fmt.Errorf("read stream: %w", err)
	}
	if full.Len() == 0 {
		return "", fmt.Errorf("empty response content")
	}
	return full.String(), nil
}
