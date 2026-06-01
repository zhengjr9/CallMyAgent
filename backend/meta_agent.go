package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

const metaAgentPrompt = `You are a development task planning assistant called CallMyAgent Meta. Your job is to help users refine their development requirements through conversation.

Your capabilities:
- You have full tool access: Read, Write, Edit, Bash, Glob, Grep
- You can explore the codebase to understand patterns
- You can run commands to verify your understanding

Your workflow:
1. Greet the user and ask clarifying questions about their task
2. Understand the scope, tech stack, and constraints
3. For complex tasks, explore the repository structure if a git repo is provided
4. After 2-5 rounds of conversation, produce a FINAL prompt wrapped in <final-prompt>...</final-prompt>

The FINAL prompt should:
- Be self-contained and unambiguous
- Include specific file paths, function names, patterns to follow
- Specify exact deliverables
- Be actionable by another AI agent

When you output the final prompt, also say "任务规划完成" or "Task planning complete".

Respond in Chinese unless the user explicitly requests English.`

func RunMetaAgent(cfg *Config, messages []Message, remoteURL string) (string, error) {
	// Create temp settings file for this session
	settingsFile := "/tmp/callmyagent-meta-settings.json"

	// Build settings with hooks
	settings := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []string{
				"Bash",
				"Read",
				"Write",
				"Edit",
				"Glob",
				"Grep",
				"WebFetch",
				"WebSearch",
			},
		},
		"model": cfg.Model,
		"hooks": map[string]interface{}{
			"SessionStart": []map[string]interface{}{
				{"hooks": []map[string]interface{}{
					{"type": "command", "command": fmt.Sprintf("/scripts/remote-hook.sh %s", remoteURL)},
				}},
			},
			"UserPromptSubmit": []map[string]interface{}{
				{"hooks": []map[string]interface{}{
					{"type": "command", "command": fmt.Sprintf("/scripts/remote-hook.sh %s", remoteURL)},
				}},
			},
			"Stop": []map[string]interface{}{
				{"hooks": []map[string]interface{}{
					{"type": "command", "command": fmt.Sprintf("/scripts/remote-hook.sh %s", remoteURL)},
				}},
			},
			"PreToolUse": []map[string]interface{}{
				{"matcher": "*", "hooks": []map[string]interface{}{
					{"type": "command", "command": fmt.Sprintf("/scripts/remote-hook.sh %s", remoteURL)},
				}},
			},
			"PostToolUse": []map[string]interface{}{
				{"matcher": "*", "hooks": []map[string]interface{}{
					{"type": "command", "command": fmt.Sprintf("/scripts/remote-hook.sh %s", remoteURL)},
				}},
			},
			"SessionEnd": []map[string]interface{}{
				{"hooks": []map[string]interface{}{
					{"type": "command", "command": fmt.Sprintf("/scripts/remote-hook.sh %s", remoteURL)},
				}},
			},
		},
		"env": map[string]interface{}{
			"CLAUDE_REMOTE_URL": remoteURL,
		},
	}

	settingsBytes, _ := json.Marshal(settings)
	if err := os.WriteFile(settingsFile, settingsBytes, 0644); err != nil {
		return "", fmt.Errorf("write settings: %w", err)
	}
	defer os.Remove(settingsFile)

	// Build conversation as a prompt
	var prompt strings.Builder
	prompt.WriteString(metaAgentPrompt + "\n\n")

	for _, msg := range messages {
		if msg.Role == "user" {
			prompt.WriteString(fmt.Sprintf("\n[用户]: %s\n", msg.Content))
		} else {
			prompt.WriteString(fmt.Sprintf("\n[助手]: %s\n", msg.Content))
		}
	}
	prompt.WriteString("\n[助手]: ")

	// Run claude CLI
	cmd := exec.Command("claude",
		"--permission-mode", "dontAsk",
		"--output-format", "json",
		"--model", cfg.Model,
		"--no-input-prompt",
		"-p", prompt.String(),
	)

	cmd.Env = append(os.Environ(),
		fmt.Sprintf("ANTHROPIC_API_KEY=%s", cfg.AnthropicAPIKey),
		fmt.Sprintf("ANTHROPIC_BASE_URL=%s", cfg.AnthropicBaseURL),
		fmt.Sprintf("ANTHROPIC_AUTH_TOKEN=%s", cfg.ClaudeAPIToken),
	)
	if cfg.ClaudeBaseURL != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("CLAUDE_BASE_URL=%s", cfg.ClaudeBaseURL))
	}

	if remoteURL != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("CLAUDE_REMOTE_URL=%s", remoteURL))
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start claude: %w", err)
	}

	// Read output line by line
	scanner := bufio.NewScanner(stdout)
	var fullResponse strings.Builder

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Try to parse as JSON
		var result map[string]interface{}
		if err := json.Unmarshal(line, &result); err != nil {
			// Not JSON, just append
			fullResponse.Write(line)
			continue
		}

		// Check for text content
		if content, ok := result["content"]; ok {
			if contentArr, ok := content.([]interface{}); ok {
				for _, c := range contentArr {
					if cMap, ok := c.(map[string]interface{}); ok {
						if cMap["type"] == "text" && cMap["text"] != nil {
							fullResponse.WriteString(cMap["text"].(string))
						}
					}
				}
			}
		}

		// Check if this is the final response
		if result["stop_reason"] != nil {
			break
		}
	}

	// Read stderr for errors
	stderrScanner := bufio.NewScanner(stderr)
	var stderrLines strings.Builder
	for stderrScanner.Scan() {
		stderrLines.Write(stderrScanner.Bytes())
		stderrLines.WriteString("\n")
	}

	if err := cmd.Wait(); err != nil {
		stderrStr := stderrLines.String()
		if stderrStr != "" {
			log.Printf("[meta] stderr: %s", stderrStr)
		}
		// Don't fail on non-zero exit if we got output
		if fullResponse.Len() == 0 {
			return "", fmt.Errorf("claude exit %v: %s", err, stderrStr)
		}
	}

	return fullResponse.String(), nil
}
