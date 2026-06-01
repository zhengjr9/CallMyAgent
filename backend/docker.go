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
	authDir := filepath.Join(runDir, "auth")

	if err := os.RemoveAll(runDir); err != nil {
		return "", fmt.Errorf("clean docker run dir: %w", err)
	}
	for _, dir := range []string{promptDir, workDir, outputDir, authDir} {
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
		"GIT_TERMINAL_PROMPT=0",
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
	if cfg.RemoteServerURL != "" {
		env = append(env, "CLAUDE_REMOTE_URL="+dockerReachableRemoteURL(cfg.RemoteServerURL))
	}

	args := []string{"run", "-d", "--name", runName}
	for _, value := range env {
		args = append(args, "-e", value)
	}
	netrcPath, err := prepareDockerNetrc(task, authDir)
	if err != nil {
		return "", err
	}
	args = append(args,
		"-v", promptDir+":/prompt:ro",
		"-v", workDir+":/workspace",
		"-v", outputDir+":/output",
	)
	if netrcPath != "" {
		args = append(args, "-v", netrcPath+":/home/claude/.netrc:ro")
	}
	args = append(args, cfg.ContainerImage)

	_ = exec.Command("docker", "rm", "-f", runName).Run()
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return runName, nil
}

func dockerReachableRemoteURL(remoteURL string) string {
	remoteURL = strings.TrimSpace(remoteURL)
	for _, host := range []string{"http://127.0.0.1:", "http://localhost:"} {
		if strings.HasPrefix(remoteURL, host) {
			return strings.Replace(remoteURL, host, "http://host.docker.internal:", 1)
		}
	}
	return remoteURL
}

func prepareDockerNetrc(task *Task, authDir string) (string, error) {
	content, err := taskNetrcContent(task)
	if err != nil {
		return "", err
	}
	if content != "" {
		return writeDockerNetrc(authDir, content)
	}
	return "", nil
}

func writeDockerNetrc(authDir, content string) (string, error) {
	netrcPath := filepath.Join(authDir, ".netrc")
	if err := os.WriteFile(netrcPath, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write git .netrc: %w", err)
	}
	return netrcPath, nil
}

func netrcFromToken(repoURL, token string) (string, error) {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(token, "machine ") {
		return token, nil
	}
	host := gitHost(repoURL)
	if host == "" {
		host = "github.com"
	}
	return fmt.Sprintf("machine %s\n  login x-access-token\n  password %s\n", host, token), nil
}

func taskNetrcContent(task *Task) (string, error) {
	switch {
	case strings.TrimSpace(task.GitToken) != "":
		return netrcFromToken(task.GitRepo, task.GitToken)
	case task.UseHostNetrc:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home for ~/.netrc: %w", err)
		}
		content, err := os.ReadFile(filepath.Join(home, ".netrc"))
		if err != nil {
			return "", fmt.Errorf("read ~/.netrc: %w", err)
		}
		return strings.TrimSpace(string(content)) + "\n", nil
	default:
		return "", nil
	}
}

func gitHost(repoURL string) string {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return ""
	}
	if strings.HasPrefix(repoURL, "https://") || strings.HasPrefix(repoURL, "http://") {
		withoutScheme := strings.TrimPrefix(strings.TrimPrefix(repoURL, "https://"), "http://")
		host := strings.SplitN(withoutScheme, "/", 2)[0]
		return strings.Split(host, "@")[len(strings.Split(host, "@"))-1]
	}
	if strings.Contains(repoURL, "@") && strings.Contains(repoURL, ":") {
		afterAt := strings.SplitN(repoURL, "@", 2)[1]
		return strings.SplitN(afterAt, ":", 2)[0]
	}
	return ""
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

func GetDockerRunLogs(containerName string) (string, error) {
	out, err := exec.Command("docker", "logs", containerName).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker logs: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
