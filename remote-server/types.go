package main

type Session struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	Model     string `json:"model,omitempty"`
	Source    string `json:"source,omitempty"`
	Status    string `json:"status"` // active, resumed, ended
	StartedAt string `json:"started_at,omitempty"`
	ResumedAt string `json:"resumed_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
}

type HookEvent struct {
	SessionID      string                 `json:"session_id"`
	Event          string                 `json:"event"`
	Cwd            string                 `json:"cwd"`
	TranscriptPath string                 `json:"transcript_path"`
	Raw            map[string]interface{} `json:"raw,omitempty"`
	Timestamp      string                 `json:"timestamp"`
}

type Message struct {
	SessionID string                 `json:"session_id"`
	Role      string                 `json:"role"` // user, assistant
	Content   string                 `json:"content"`
	Raw       map[string]interface{} `json:"raw,omitempty"`
	Timestamp string                 `json:"timestamp"`
	Uuid      string                 `json:"uuid,omitempty"`
}

type Transcript struct {
	SessionID    string                 `json:"session_id"`
	Cwd          string                 `json:"cwd"`
	Messages     []TranscriptMessage    `json:"messages"`
	MessageCount int                    `json:"message_count"`
	Timestamp    string                `json:"timestamp"`
}

type TranscriptMessage struct {
	Role      string          `json:"role"`
	Content   interface{}    `json:"content"` // can be string or ContentBlock[]
	Timestamp string          `json:"timestamp"`
	Uuid      string          `json:"uuid,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
}

type ToolUsage struct {
	SessionID  string                 `json:"session_id"`
	Event      string                 `json:"event"`
	ToolName   string                 `json:"tool_name"`
	ToolInput  map[string]interface{} `json:"tool_input,omitempty"`
	IsPreTool  bool                   `json:"is_pre_tool"`
	StopReason string                 `json:"stop_reason,omitempty"`
	Timestamp  string                 `json:"timestamp"`
}