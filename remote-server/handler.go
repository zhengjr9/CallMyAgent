package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type Handler struct {
	store *Store
}

func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Check if requesting specific fields
		if strings.Contains(r.URL.RawQuery, "fields") {
			stats := h.store.GetStats()
			writeJSON(w, stats)
			return
		}
		sessions := h.store.ListSessions()
		writeJSON(w, sessions)
	case http.MethodPost, http.MethodPut:
		var sess Session
		if err := json.NewDecoder(r.Body).Decode(&sess); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		h.store.PutSession(&sess)
		log.Printf("[session] %.8s %s (%s) model=%s", sess.SessionID, sess.Status, sess.Cwd, sess.Model)
		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if id == "" {
		http.Error(w, "missing session id", http.StatusBadRequest)
		return
	}

	parts := strings.Split(id, "/")
	sessionID := parts[0]

	if len(parts) > 1 {
		switch parts[1] {
		case "messages":
			msgs := h.store.GetMessages(sessionID)
			writeJSON(w, msgs)
			return
		case "transcript":
			t := h.store.GetTranscript(sessionID)
			if t == nil {
				writeJSON(w, map[string]string{"error": "no transcript"})
				return
			}
			writeJSON(w, t)
			return
		case "events":
			events := h.store.GetEvents(sessionID)
			writeJSON(w, events)
			return
		case "tools":
			tools := h.store.GetToolUsage(sessionID)
			writeJSON(w, tools)
			return
		}
	}

	sess := h.store.GetSession(sessionID)
	if sess == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	writeJSON(w, sess)
}

func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Build HookEvent from raw
	event := HookEvent{
		SessionID:      getString(raw, "session_id"),
		Event:          firstString(raw, "event", "hook_event_name"),
		Cwd:            getString(raw, "cwd"),
		TranscriptPath: getString(raw, "transcript_path"),
		Raw:            raw,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}

	h.store.AddEvent(event)

	// If it's a SessionStart with source=compact, trigger resume logic
	if event.Event == "SessionStart" && getString(raw, "source") == "compact" {
		log.Printf("[resume] Session %s resumed from compaction", shortID(event.SessionID))
	}

	log.Printf("[event] %s: %s | tool=%s | prompt=%s",
		event.Event, shortID(event.SessionID),
		getString(raw, "tool_name"),
		truncate(getString(raw, "prompt"), 60))

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		sessionID := r.URL.Query().Get("session_id")
		msgs := h.store.GetMessages(sessionID)
		writeJSON(w, msgs)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	msg := Message{
		SessionID: getString(raw, "session_id"),
		Role:      getString(raw, "role"),
		Content:   getString(raw, "content"),
		Raw:       raw,
		Timestamp: getString(raw, "timestamp"),
		Uuid:      getString(raw, "uuid"),
	}

	h.store.AddMessage(msg)
	log.Printf("[message] %s: %s | uuid=%s", msg.Role, truncate(msg.Content, 80), shortID(msg.Uuid))

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) handleTranscripts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	// Store transcript with full message detail
	transcript := &Transcript{
		SessionID: getString(raw, "session_id"),
		Cwd:       getString(raw, "cwd"),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	// Parse messages array
	if msgsRaw, ok := raw["messages"].([]interface{}); ok {
		for _, m := range msgsRaw {
			if mMap, ok := m.(map[string]interface{}); ok {
				msg := TranscriptMessage{
					Role:      getString(mMap, "role"),
					Timestamp: getString(mMap, "timestamp"),
					Uuid:      getString(mMap, "uuid"),
					Thinking:  getString(mMap, "thinking"),
					Content:   mMap["content"], // can be interface{} - text or array
				}
				transcript.Messages = append(transcript.Messages, msg)
			}
		}
		transcript.MessageCount = len(transcript.Messages)
	}

	h.store.PutTranscript(transcript)

	// Also store each as a Message for query
	if msgsRaw, ok := raw["messages"].([]interface{}); ok {
		for _, m := range msgsRaw {
			if mMap, ok := m.(map[string]interface{}); ok {
				// Convert content to readable string
				var contentText string
				if content, ok := mMap["content"]; ok {
					switch c := content.(type) {
					case string:
						contentText = c
					case []interface{}:
						for _, item := range c {
							if itemMap, ok := item.(map[string]interface{}); ok {
								itemType := getString(itemMap, "type")
								if itemType == "text" {
									contentText += getString(itemMap, "text") + "\n"
								} else if itemType == "tool_use" {
									contentText += fmt.Sprintf("[Tool: %s]", getString(itemMap, "name"))
								} else if itemType == "tool_result" {
									contentText += "[Tool Result]\n"
								}
							}
						}
					}
				}
				msg := Message{
					SessionID: transcript.SessionID,
					Role:      getString(mMap, "role"),
					Content:   contentText,
					Raw:       mMap,
					Timestamp: getString(mMap, "timestamp"),
					Uuid:      getString(mMap, "uuid"),
				}
				h.store.AddMessage(msg)
			}
		}
	}

	log.Printf("[transcript] session=%s messages=%d cwd=%s",
		shortID(transcript.SessionID), transcript.MessageCount, transcript.Cwd)

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (h *Handler) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		sessionID := r.URL.Query().Get("session_id")
		tools := h.store.GetToolUsage(sessionID)
		writeJSON(w, tools)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	tool := ToolUsage{
		SessionID:  getString(raw, "session_id"),
		Event:      getString(raw, "event"),
		ToolName:   getString(raw, "tool_name"),
		ToolInput:  getMap(raw, "tool_input"),
		IsPreTool:  getBool(raw, "is_pre_tool"),
		StopReason: getString(raw, "stop_reason"),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	h.store.AddToolUsage(tool)

	// Log with input details
	inputMap := tool.ToolInput
	if inputMap != nil {
		keys := make([]string, 0, len(inputMap))
		for k := range inputMap {
			keys = append(keys, k)
		}
		log.Printf("[tool] %s %s input=%v", tool.Event, tool.ToolName, keys)
	} else {
		log.Printf("[tool] %s %s (no input)", tool.Event, tool.ToolName)
	}

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := getString(m, key); value != "" {
			return value
		}
	}
	return ""
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key].(map[string]interface{}); ok {
		return v
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func shortID(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12]
}
