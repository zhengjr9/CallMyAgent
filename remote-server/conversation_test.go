package main

import "testing"

func TestBuildConversationFlattensClaudeTranscript(t *testing.T) {
	transcript := &Transcript{
		SessionID: "s1",
		Messages: []TranscriptMessage{
			{Role: "user", Content: []interface{}{map[string]interface{}{"type": "text", "text": "hello"}}},
			{Role: "assistant", Content: []interface{}{
				map[string]interface{}{"type": "tool_use", "name": "Bash", "input": map[string]interface{}{"command": "pwd"}},
				map[string]interface{}{"type": "text", "text": "done"},
			}},
		},
	}

	got := BuildConversation(transcript, nil, "callmyagent")
	if got.SessionID != "s1" {
		t.Fatalf("SessionID = %q", got.SessionID)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("len(messages) = %d", len(got.Messages))
	}
	if got.Messages[0].Role != "user" || got.Messages[0].Content != "hello" {
		t.Fatalf("unexpected first message: %#v", got.Messages[0])
	}
	if got.Messages[1].Role != "assistant" || got.Messages[1].Content == "" {
		t.Fatalf("unexpected assistant message: %#v", got.Messages[1])
	}
}
