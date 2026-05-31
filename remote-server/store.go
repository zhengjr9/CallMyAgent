package main

import (
	"fmt"
	"sync"
	"time"
)

type Store struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	events      []HookEvent
	messages    []Message
	transcripts map[string]*Transcript
	tools       []ToolUsage
}

func NewStore() *Store {
	return &Store{
		sessions:    make(map[string]*Session),
		transcripts: make(map[string]*Transcript),
	}
}

func (s *Store) PutSession(sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.sessions[sess.SessionID]; ok {
		if sess.Status != "" {
			existing.Status = sess.Status
		}
		if sess.EndedAt != "" {
			existing.EndedAt = sess.EndedAt
		}
		if sess.ResumedAt != "" {
			existing.ResumedAt = sess.ResumedAt
		}
		if sess.Model != "" {
			existing.Model = sess.Model
		}
		if sess.Cwd != "" {
			existing.Cwd = sess.Cwd
		}
	} else {
		if sess.StartedAt == "" {
			sess.StartedAt = time.Now().UTC().Format(time.RFC3339)
		}
		s.sessions[sess.SessionID] = sess
	}
}

func (s *Store) GetSession(id string) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[id]
}

func (s *Store) ListSessions() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Session, 0, len(s.sessions))
	for _, t := range s.sessions {
		result = append(result, t)
	}
	return result
}

func (s *Store) AddEvent(e HookEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	if len(s.events) > 20000 {
		s.events = s.events[len(s.events)-20000:]
	}
}

func (s *Store) GetEvents(sessionID string) []HookEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sessionID == "" {
		return s.events
	}
	var result []HookEvent
	for _, e := range s.events {
		if e.SessionID == sessionID {
			result = append(result, e)
		}
	}
	return result
}

func (s *Store) AddMessage(m Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, m)
	if len(s.messages) > 100000 {
		s.messages = s.messages[len(s.messages)-100000:]
	}
}

func (s *Store) GetMessages(sessionID string) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sessionID == "" {
		return s.messages
	}
	var result []Message
	for _, m := range s.messages {
		if m.SessionID == sessionID {
			result = append(result, m)
		}
	}
	return result
}

func (s *Store) PutTranscript(t *Transcript) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transcripts[t.SessionID] = t
}

func (s *Store) GetTranscript(sessionID string) *Transcript {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.transcripts[sessionID]
}

func (s *Store) AddToolUsage(t ToolUsage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = append(s.tools, t)
	if len(s.tools) > 20000 {
		s.tools = s.tools[len(s.tools)-20000:]
	}
}

func (s *Store) GetToolUsage(sessionID string) []ToolUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if sessionID == "" {
		return s.tools
	}
	var result []ToolUsage
	for _, t := range s.tools {
		if t.SessionID == sessionID {
			result = append(result, t)
		}
	}
	return result
}

func (s *Store) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	active := 0
	for _, sess := range s.sessions {
		if sess.Status == "active" || sess.Status == "resumed" {
			active++
		}
	}
	return map[string]interface{}{
		"total_sessions":  len(s.sessions),
		"active_sessions": active,
		"total_events":    len(s.events),
		"total_messages":  len(s.messages),
		"total_tools":     len(s.tools),
		"total_transcripts": len(s.transcripts),
	}
}

func (s *Store) FormatSummary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fmt.Sprintf("Sessions: %d, Events: %d, Messages: %d, Tools: %d, Transcripts: %d",
		len(s.sessions), len(s.events), len(s.messages), len(s.tools), len(s.transcripts))
}