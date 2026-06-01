package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

type Handler struct {
	cfg   *Config
	store *TaskStore
}

func NewHandler(cfg *Config, store *TaskStore) *Handler {
	return &Handler{cfg: cfg, store: store}
}

func (h *Handler) handleRemoteAPI(w http.ResponseWriter, r *http.Request) {
	remoteURL := strings.TrimRight(h.cfg.RemoteServerURL, "/")
	if remoteURL == "" {
		http.Error(w, "remote server is not configured", http.StatusBadGateway)
		return
	}
	target, err := url.Parse(remoteURL)
	if err != nil {
		http.Error(w, "invalid remote server URL", http.StatusBadGateway)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("remote proxy error: %v", err)
		http.Error(w, "remote server unavailable: "+err.Error(), http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}

func (h *Handler) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listTasks(w, r)
	case http.MethodPost:
		h.createTask(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/tasks/"), "/")
	taskID := parts[0]

	if taskID == "" {
		http.Error(w, "missing task id", http.StatusBadRequest)
		return
	}

	// Handle /api/tasks/{id}/status
	if len(parts) > 1 && parts[1] == "status" {
		h.getTaskStatus(w, r, taskID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getTask(w, r, taskID)
	case http.MethodDelete:
		h.deleteTask(w, r, taskID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	task := h.store.Get(req.TaskID)
	if task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	// Add the user message before calling the meta agent so the planner sees it.
	task.Conversation = append(task.Conversation, Message{
		Role:    "user",
		Content: req.Message,
	})

	// Prefer the built-in Claude Code CLI meta agent when available; fall back to HTTP.
	metaMode := getEnv("META_AGENT_MODE", "auto")
	useCLI := os.Getenv("USE_META_CLI") == "true" || metaMode == "cli"
	if metaMode == "auto" {
		_, cliErr := exec.LookPath("claude")
		useCLI = cliErr == nil
	}
	remoteURL := os.Getenv("CLAUDE_REMOTE_URL")

	var reply string
	var err error

	if useCLI {
		// Use Claude Code CLI with remote hooks
		reply, err = RunMetaAgent(h.cfg, task.Conversation, remoteURL)
	} else {
		// Use direct HTTP API
		reply, err = CallClaudeAPI(h.cfg, task.Conversation)
	}

	if err != nil {
		log.Printf("Meta agent error: %v", err)
		task.Conversation = task.Conversation[:len(task.Conversation)-1]
		h.store.Update(task)
		http.Error(w, "failed to call meta agent: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Add assistant reply
	task.Conversation = append(task.Conversation, Message{
		Role:    "assistant",
		Content: reply,
	})

	// Check if this is the final prompt
	isFinal := false
	if strings.Contains(reply, "<final-prompt>") {
		isFinal = true
		start := strings.Index(reply, "<final-prompt>")
		end := strings.Index(reply, "</final-prompt>")
		if start != -1 && end != -1 {
			task.FinalPrompt = reply[start+14 : end]
			task.Status = "ready"
		}
	}

	h.store.Update(task)

	resp := ChatResponse{
		TaskID:  task.ID,
		Reply:   reply,
		IsFinal: isFinal,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleChatStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	task := h.store.Get(req.TaskID)
	if task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	send := func(event string, payload any) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	task.Conversation = append(task.Conversation, Message{Role: "user", Content: req.Message})
	h.store.Update(task)
	_ = send("start", map[string]string{"taskId": task.ID})

	var reply string
	var err error
	metaMode := getEnv("META_AGENT_MODE", "auto")
	useCLI := os.Getenv("USE_META_CLI") == "true" || metaMode == "cli"
	if metaMode == "auto" {
		_, cliErr := exec.LookPath("claude")
		useCLI = cliErr == nil
	}

	if useCLI {
		reply, err = RunMetaAgent(h.cfg, task.Conversation, os.Getenv("CLAUDE_REMOTE_URL"))
		if err == nil {
			_ = send("delta", map[string]string{"text": reply})
		}
	} else {
		reply, err = CallClaudeAPIStream(h.cfg, task.Conversation, func(delta string) error {
			return send("delta", map[string]string{"text": delta})
		})
	}
	if err != nil {
		log.Printf("Meta stream error: %v", err)
		task.Conversation = task.Conversation[:len(task.Conversation)-1]
		h.store.Update(task)
		_ = send("error", map[string]string{"error": err.Error()})
		return
	}

	task.Conversation = append(task.Conversation, Message{Role: "assistant", Content: reply})
	isFinal := false
	if strings.Contains(reply, "<final-prompt>") {
		isFinal = true
		start := strings.Index(reply, "<final-prompt>")
		end := strings.Index(reply, "</final-prompt>")
		if start != -1 && end != -1 {
			task.FinalPrompt = strings.TrimSpace(reply[start+14 : end])
			task.Status = "ready"
		}
	}
	h.store.Update(task)
	_ = send("done", ChatResponse{TaskID: task.ID, Reply: reply, IsFinal: isFinal})
}

func (h *Handler) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	task := h.store.Get(req.TaskID)
	if task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	if task.Status != "ready" && task.Status != "chatting" {
		http.Error(w, "task is not in executable state", http.StatusBadRequest)
		return
	}

	// Override git repo/branch/engine if provided
	if req.GitRepo != "" {
		task.GitRepo = req.GitRepo
	}
	if req.GitBranch != "" {
		task.GitBranch = req.GitBranch
	}
	if req.Engine != "" {
		task.Engine = req.Engine
	}
	if task.Engine == "" {
		task.Engine = "claude"
	}
	if !IsSupportedEngine(task.Engine) {
		http.Error(w, "unsupported engine: "+task.Engine, http.StatusBadRequest)
		return
	}
	if req.MaxTurns > 0 {
		task.MaxTurns = req.MaxTurns
	}
	if req.BudgetUSD > 0 {
		task.BudgetUSD = req.BudgetUSD
	}
	if req.SchedulerMode != "" {
		task.SchedulerMode = normalizeSchedulerMode(req.SchedulerMode)
	}
	if task.SchedulerMode == "" {
		task.SchedulerMode = normalizeSchedulerMode(h.cfg.SchedulerMode)
	}
	task.GitToken = strings.TrimSpace(req.GitToken)
	task.UseHostNetrc = req.UseHostNetrc

	if task.GitRepo == "" && task.SchedulerMode != "docker" {
		http.Error(w, "git repo URL is required", http.StatusBadRequest)
		return
	}

	runName, schedulerMode, err := createScheduledRun(h.cfg, task)
	if err != nil {
		log.Printf("Failed to schedule task: %v", err)
		task.Status = "failed"
		task.ErrorMessage = err.Error()
		h.store.Update(task)
		http.Error(w, "failed to schedule task: "+err.Error(), http.StatusInternalServerError)
		return
	}

	task.JobName = runName
	task.SchedulerMode = schedulerMode
	task.Status = "running"
	h.store.Update(task)

	resp := ExecuteResponse{
		TaskID:  task.ID,
		JobName: runName,
		Status:  "running",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}
	if req.Engine != "" && !IsSupportedEngine(req.Engine) {
		http.Error(w, "unsupported engine: "+req.Engine, http.StatusBadRequest)
		return
	}

	task := h.store.Create(req.Title, req.Description, req.GitRepo, req.GitBranch, req.Engine)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

func (h *Handler) handleEngines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(engineCatalog)
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	tasks := h.store.List()
	for _, task := range tasks {
		h.refreshTaskStatus(task)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func (h *Handler) getTask(w http.ResponseWriter, r *http.Request, taskID string) {
	task := h.store.Get(taskID)
	if task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func (h *Handler) deleteTask(w http.ResponseWriter, r *http.Request, taskID string) {
	h.store.Delete(taskID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getTaskStatus(w http.ResponseWriter, r *http.Request, taskID string) {
	task := h.store.Get(taskID)
	if task == nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	h.refreshTaskStatus(task)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"taskId": task.ID,
		"status": task.Status,
	})
}

func (h *Handler) refreshTaskStatus(task *Task) {
	if task.Status != "running" || task.JobName == "" {
		return
	}
	status, err := getScheduledRunStatus(h.cfg, task)
	if err == nil && status != task.Status {
		task.Status = status
		h.store.Update(task)
	}
}
