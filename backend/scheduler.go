package main

import (
	"fmt"
	"strings"
)

func taskPrompt(task *Task) string {
	if task.FinalPrompt != "" {
		return task.FinalPrompt
	}

	var sb strings.Builder
	for _, msg := range task.Conversation {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", msg.Role, msg.Content))
	}
	return sb.String()
}

func taskLimits(task *Task) (int, float64) {
	maxTurns := 20
	budgetUSD := 5.0
	if task.FinalPrompt != "" {
		maxTurns = 50
		budgetUSD = 10.0
	}
	if task.MaxTurns > 0 {
		maxTurns = task.MaxTurns
	}
	if task.BudgetUSD > 0 {
		budgetUSD = task.BudgetUSD
	}
	return maxTurns, budgetUSD
}

func normalizeSchedulerMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "job", "k8s", "kubernetes":
		return "job"
	case "docker", "local":
		return "docker"
	default:
		return mode
	}
}

func createScheduledRun(cfg *Config, task *Task) (string, string, error) {
	mode := normalizeSchedulerMode(task.SchedulerMode)
	if mode == "" {
		mode = normalizeSchedulerMode(cfg.SchedulerMode)
	}

	switch mode {
	case "job":
		name, err := CreateClaudeJob(cfg, task)
		return name, "job", err
	case "docker":
		name, err := CreateDockerRun(cfg, task)
		return name, "docker", err
	default:
		return "", mode, fmt.Errorf("unsupported scheduler mode: %s", mode)
	}
}

func getScheduledRunStatus(cfg *Config, task *Task) (string, error) {
	switch normalizeSchedulerMode(task.SchedulerMode) {
	case "docker":
		return GetDockerRunStatus(task.JobName)
	default:
		return GetJobStatus(cfg, task.JobName)
	}
}

func getScheduledRunLogs(cfg *Config, task *Task) (string, error) {
	switch normalizeSchedulerMode(task.SchedulerMode) {
	case "docker":
		return GetDockerRunLogs(task.JobName)
	default:
		return GetJobLogs(cfg, task.JobName)
	}
}
