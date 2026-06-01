package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	// Support CLAUDE_API_TOKEN or ANTHROPIC_AUTH_TOKEN
	claudeToken := os.Getenv("CLAUDE_API_TOKEN")
	if claudeToken == "" {
		claudeToken = os.Getenv("ANTHROPIC_AUTH_TOKEN")
	}

	cfg := &Config{
		Port:             getEnv("PORT", "8080"),
		AnthropicAPIKey:  os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicBaseURL: getEnv("ANTHROPIC_BASE_URL", "https://api.anthropic.com"),
		CodexAPIKey:      firstEnv("CODEX_API_KEY", "OPENAI_API_KEY"),
		CodexBaseURL:     firstEnv("CODEX_BASE_URL", "OPENAI_BASE_URL"),
		CodexModel:       firstEnv("CODEX_MODEL", "OPENAI_MODEL"),
		ContainerImage:   getEnv("CONTAINER_IMAGE", "callmyagent-worker:latest"),
		SchedulerMode:    getEnv("SCHEDULER_MODE", "job"),
		DockerRunsDir:    getEnv("DOCKER_RUNS_DIR", os.TempDir()+"/callmyagent-runs"),
		K8sNamespace:     getEnv("K8S_NAMESPACE", "default"),
		GitRepoURL:       os.Getenv("GIT_REPO_URL"),
		GitBranch:        getEnv("GIT_BRANCH", "main"),
		ClaudeAPIToken:   claudeToken,
		ClaudeBaseURL:    getEnv("CLAUDE_BASE_URL", ""),
		SkillsDir:        getEnv("SKILLS_DIR", "/skills"),
		MaxConversations: 5,
		OutputPVC:        os.Getenv("OUTPUT_PVC"),
		Model:            os.Getenv("CLAUDE_MODEL"),
		RemoteServerURL:  getEnv("REMOTE_SERVER_URL", "http://127.0.0.1:9090"),
	}

	store := NewTaskStore()
	handler := NewHandler(cfg, store)

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/tasks", handler.handleTasks)
	mux.HandleFunc("/api/tasks/", handler.handleTaskByID)
	mux.HandleFunc("/api/tasks/chat", handler.handleChat)
	mux.HandleFunc("/api/tasks/chat/stream", handler.handleChatStream)
	mux.HandleFunc("/api/tasks/execute", handler.handleExecute)
	mux.HandleFunc("/api/sessions", handler.handleRemoteAPI)
	mux.HandleFunc("/api/sessions/", handler.handleRemoteAPI)
	mux.HandleFunc("/api/events", handler.handleRemoteAPI)
	mux.HandleFunc("/api/messages", handler.handleRemoteAPI)
	mux.HandleFunc("/api/transcripts", handler.handleRemoteAPI)
	mux.HandleFunc("/api/tools", handler.handleRemoteAPI)
	mux.HandleFunc("/api/engines", handler.handleEngines)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Serve frontend static files
	frontendDir := getEnv("FRONTEND_DIR", "./frontend")
	if _, err := os.Stat(frontendDir + "/index.html"); err != nil && frontendDir == "./frontend" {
		if _, parentErr := os.Stat("../frontend"); parentErr == nil {
			frontendDir = "../frontend"
		}
	}
	fs := http.FileServer(http.Dir(frontendDir))
	mux.Handle("/", fs)

	log.Printf("CallMyAgent server starting on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		log.Fatal(err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}
