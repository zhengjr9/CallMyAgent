package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ConversationMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ConversationExport struct {
	SessionID string                `json:"session_id"`
	Target    string                `json:"target"`
	Messages  []ConversationMessage `json:"messages"`
}

func BuildConversation(t *Transcript, fallback []Message, target string) ConversationExport {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		target = "callmyagent"
	}
	out := ConversationExport{Target: target}
	if t != nil {
		out.SessionID = t.SessionID
		for _, msg := range t.Messages {
			out.Messages = append(out.Messages, ConversationMessage{
				Role:    normalizeRole(msg.Role),
				Content: flattenContent(msg.Content, msg.Thinking),
			})
		}
	} else {
		for _, msg := range fallback {
			if out.SessionID == "" {
				out.SessionID = msg.SessionID
			}
			out.Messages = append(out.Messages, ConversationMessage{
				Role:    normalizeRole(msg.Role),
				Content: msg.Content,
			})
		}
	}
	out.Messages = compactMessages(out.Messages)
	if target == "codex" {
		out.Messages = toCodexMessages(out.Messages)
	}
	return out
}

func compactMessages(messages []ConversationMessage) []ConversationMessage {
	result := make([]ConversationMessage, 0, len(messages))
	for _, msg := range messages {
		msg.Content = strings.TrimSpace(msg.Content)
		if msg.Content == "" {
			continue
		}
		if msg.Role == "" {
			msg.Role = "assistant"
		}
		if len(result) > 0 && result[len(result)-1].Role == msg.Role {
			result[len(result)-1].Content += "\n\n" + msg.Content
			continue
		}
		result = append(result, msg)
	}
	return result
}

func toCodexMessages(messages []ConversationMessage) []ConversationMessage {
	result := make([]ConversationMessage, 0, len(messages))
	for _, msg := range messages {
		role := msg.Role
		if role != "user" && role != "assistant" && role != "system" {
			role = "assistant"
		}
		result = append(result, ConversationMessage{Role: role, Content: msg.Content})
	}
	return result
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user", "human":
		return "user"
	case "assistant", "ai":
		return "assistant"
	case "system", "developer":
		return "system"
	default:
		return role
	}
}

func flattenContent(content interface{}, thinking string) string {
	var parts []string
	if strings.TrimSpace(thinking) != "" {
		parts = append(parts, "[thinking]\n"+strings.TrimSpace(thinking))
	}
	switch v := content.(type) {
	case string:
		parts = append(parts, v)
	case []interface{}:
		for _, item := range v {
			parts = append(parts, flattenBlock(item))
		}
	case []map[string]interface{}:
		for _, item := range v {
			parts = append(parts, flattenBlock(item))
		}
	case nil:
	default:
		if b, err := json.Marshal(v); err == nil {
			parts = append(parts, string(b))
		} else {
			parts = append(parts, fmt.Sprint(v))
		}
	}
	return strings.TrimSpace(strings.Join(nonEmpty(parts), "\n\n"))
}

func flattenBlock(block interface{}) string {
	m, ok := block.(map[string]interface{})
	if !ok {
		if b, err := json.Marshal(block); err == nil {
			return string(b)
		}
		return fmt.Sprint(block)
	}
	switch getString(m, "type") {
	case "text":
		return getString(m, "text")
	case "thinking":
		return "[thinking]\n" + getString(m, "thinking")
	case "tool_use":
		input := ""
		if raw, err := json.Marshal(m["input"]); err == nil && string(raw) != "null" {
			input = "\n" + string(raw)
		}
		return "[tool_use:" + getString(m, "name") + "]" + input
	case "tool_result":
		switch c := m["content"].(type) {
		case string:
			return "[tool_result]\n" + c
		default:
			raw, _ := json.Marshal(c)
			return "[tool_result]\n" + string(raw)
		}
	default:
		raw, _ := json.Marshal(m)
		return string(raw)
	}
}

func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
