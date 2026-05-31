package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

type Handler struct {
	cfg   *Config
	store *TaskStore
}

func NewHandler(cfg *Config, store *TaskStore) *Handler {
	return &Handler{cfg: cfg, store: store}
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

	// Check if we should use CLI-based meta agent
	useCLI := os.Getenv("USE_META_CLI") == "true"
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
		http.Error(w, "failed to call meta agent: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Add user message
	task.Conversation = append(task.Conversation, Message{
		Role:    "user",
		Content: req.Message,
	})

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

	if task.GitRepo == "" {
		http.Error(w, "git repo URL is required", http.StatusBadRequest)
		return
	}

	// Create K8s job (engine passed via task.Engine)
	jobName, err := CreateClaudeJob(h.cfg, task)
	if err != nil {
		log.Printf("Failed to create job: %v", err)
		task.Status = "failed"
		task.ErrorMessage = err.Error()
		h.store.Update(task)
		http.Error(w, "failed to create job: "+err.Error(), http.StatusInternalServerError)
		return
	}

	task.JobName = jobName
	task.Status = "running"
	h.store.Update(task)

	resp := ExecuteResponse{
		TaskID:  task.ID,
		JobName: jobName,
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

	task := h.store.Create(req.Title, req.Description, req.GitRepo, req.GitBranch, req.Engine)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	tasks := h.store.List()
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

	// If job is running, check K8s status
	if task.Status == "running" && task.JobName != "" {
		status, err := GetJobStatus(h.cfg, task.JobName)
		if err == nil && status != task.Status {
			task.Status = status
			h.store.Update(task)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"taskId": task.ID,
		"status": task.Status,
	})
}
