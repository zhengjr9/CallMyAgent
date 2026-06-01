package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func CreateDockerRun(cfg *Config, task *Task) (string, error) {
	runName := fmt.Sprintf("callmyagent-%s", task.ID)
	runDir := filepath.Join(cfg.DockerRunsDir, task.ID)
	promptDir := filepath.Join(runDir, "prompt")
	workDir := filepath.Join(runDir, "workspace")
	outputDir := filepath.Join(runDir, "output")

	for _, dir := range []string{promptDir, workDir, outputDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", fmt.Errorf("create docker run dir: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(promptDir, "prompt.txt"), []byte(taskPrompt(task)), 0o600); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}

	maxTurns, budgetUSD := taskLimits(task)
	env := []string{
		"CALLMYAGENT_TASK_ID=" + task.ID,
		"GIT_REPO=" + task.GitRepo,
		"GIT_BRANCH=" + task.GitBranch,
		"TASK_ENGINE=" + task.Engine,
		fmt.Sprintf("MAX_TURNS=%d", maxTurns),
		fmt.Sprintf("BUDGET_USD=%.2f", budgetUSD),
	}
	if cfg.AnthropicAPIKey != "" {
		env = append(env, "ANTHROPIC_API_KEY="+cfg.AnthropicAPIKey)
	}
	if cfg.AnthropicBaseURL != "" {
		env = append(env, "ANTHROPIC_BASE_URL="+cfg.AnthropicBaseURL)
	}
	if cfg.CodexAPIKey != "" {
		env = append(env, "OPENAI_API_KEY="+cfg.CodexAPIKey, "CODEX_API_KEY="+cfg.CodexAPIKey)
	}
	if cfg.CodexBaseURL != "" {
		env = append(env, "OPENAI_BASE_URL="+cfg.CodexBaseURL, "CODEX_BASE_URL="+cfg.CodexBaseURL)
	}
	if cfg.CodexModel != "" {
		env = append(env, "OPENAI_MODEL="+cfg.CodexModel, "CODEX_MODEL="+cfg.CodexModel)
	}
	if cfg.ClaudeAPIToken != "" {
		env = append(env, "CLAUDE_API_TOKEN="+cfg.ClaudeAPIToken)
	}

	args := []string{"run", "-d", "--name", runName}
	for _, value := range env {
		args = append(args, "-e", value)
	}
	args = append(args,
		"-v", promptDir+":/prompt:ro",
		"-v", workDir+":/workspace",
		"-v", outputDir+":/output",
		cfg.ContainerImage,
	)

	_ = exec.Command("docker", "rm", "-f", runName).Run()
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return runName, nil
}

func GetDockerRunStatus(containerName string) (string, error) {
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Status}} {{.State.ExitCode}}", containerName).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker inspect: %w: %s", err, strings.TrimSpace(string(out)))
	}

	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return "pending", nil
	}
	switch fields[0] {
	case "created":
		return "pending", nil
	case "running", "restarting", "paused":
		return "running", nil
	case "exited", "dead":
		if len(fields) > 1 && fields[1] == "0" {
			return "completed", nil
		}
		return "failed", nil
	default:
		return "pending", nil
	}
}
