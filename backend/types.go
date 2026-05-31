package main

type Config struct {
	Port             string
	AnthropicAPIKey  string
	AnthropicBaseURL string
	ContainerImage   string
	K8sNamespace     string
	GitRepoURL       string
	GitBranch        string
	ClaudeAPIToken   string
	ClaudeBaseURL    string
	SkillsDir        string
	MaxConversations int
	OutputPVC        string
	Model            string
}

type Task struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	GitRepo      string    `json:"gitRepo"`
	GitBranch    string    `json:"gitBranch"`
	Engine       string    `json:"engine"` // "claude", "codex", "opencode", "hermes", "openclaw"
	Status       string    `json:"status"` // pending, chatting, ready, running, completed, failed
	Conversation []Message `json:"conversation"`
	FinalPrompt  string    `json:"finalPrompt,omitempty"`
	JobName       string    `json:"jobName,omitempty"`
	ErrorMessage  string    `json:"errorMessage,omitempty"`
	CreatedAt    string    `json:"createdAt"`
	UpdatedAt    string    `json:"updatedAt"`
}

type Message struct {
	Role    string `json:"role"` // user, assistant
	Content string `json:"content"`
}

type ChatRequest struct {
	TaskID  string `json:"taskId"`
	Message string `json:"message"`
}

type ChatResponse struct {
	TaskID  string `json:"taskId"`
	Reply   string `json:"reply"`
	IsFinal bool   `json:"isFinal"`
}

type ExecuteRequest struct {
	TaskID    string `json:"taskId"`
	GitRepo   string `json:"gitRepo"`
	GitBranch string `json:"gitBranch"`
	Engine    string `json:"engine"` // "claude" or "codex"
	MaxTurns  int    `json:"maxTurns,omitempty"`
	BudgetUSD float64 `json:"budgetUsd,omitempty"`
}

type ExecuteResponse struct {
	TaskID  string `json:"taskId"`
	JobName string `json:"jobName"`
	Status  string `json:"status"`
}

type CreateTaskRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	GitRepo     string `json:"gitRepo,omitempty"`
	GitBranch   string `json:"gitBranch,omitempty"`
	Engine      string `json:"engine,omitempty"`
}

// Anthropic API types
type AnthropicRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
}

type AnthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}
