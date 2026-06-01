package main

var engineCatalog = []EngineInfo{
	{
		ID:             "claude",
		Name:           "Claude Code",
		Command:        "claude",
		Install:        []string{"npm install -g @anthropic-ai/claude-code"},
		Config:         []string{"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "CLAUDE_MODEL"},
		DocsURL:        "https://docs.anthropic.com/en/docs/claude-code",
		RepositoryURL:  "https://github.com/anthropics/claude-code",
		NonInteractive: `claude --permission-mode dontAsk --output-format json -p "$PROMPT"`,
		RequiresAPIKeys: []string{
			"ANTHROPIC_API_KEY",
		},
	},
	{
		ID:             "codex",
		Name:           "Codex CLI",
		Command:        "codex",
		Install:        []string{"npm install -g @openai/codex"},
		Config:         []string{"OPENAI_API_KEY or CODEX_API_KEY", "OPENAI_BASE_URL or CODEX_BASE_URL", "OPENAI_MODEL or CODEX_MODEL"},
		DocsURL:        "https://github.com/openai/codex",
		RepositoryURL:  "https://github.com/openai/codex",
		NonInteractive: `codex exec --json --ephemeral -C /workspace/repo -- "$PROMPT"`,
		RequiresAPIKeys: []string{
			"CODEX_API_KEY",
		},
	},
	{
		ID:             "opencode",
		Name:           "OpenCode",
		Command:        "opencode",
		Install:        []string{"curl -fsSL https://opencode.ai/install | bash", "npm install -g opencode-ai"},
		Config:         []string{"OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_MODEL", "OPENCODE_MODEL (optional)"},
		DocsURL:        "https://opencode.ai/docs",
		RepositoryURL:  "https://github.com/sst/opencode",
		NonInteractive: `opencode run --model "$OPENCODE_MODEL" "$PROMPT"`,
		RequiresAPIKeys: []string{
			"ANTHROPIC_API_KEY or OPENAI_API_KEY",
		},
	},
	{
		ID:             "hermes",
		Name:           "Hermes Agent",
		Command:        "hermes",
		Install:        []string{"curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash", "pipx install hermes-agent"},
		Config:         []string{"ANTHROPIC_API_KEY + ANTHROPIC_BASE_URL", "or HERMES_PROVIDER=custom with OPENAI_API_KEY + OPENAI_BASE_URL + OPENAI_MODEL"},
		DocsURL:        "https://hermes-ai.net/en/docs/installation/",
		RepositoryURL:  "https://github.com/NousResearch/hermes-agent",
		NonInteractive: `hermes -z "$PROMPT" --max-turns "$MAX_TURNS"`,
		RequiresAPIKeys: []string{
			"Provider API key",
		},
	},
	{
		ID:             "openclaw",
		Name:           "OpenClaw",
		Command:        "openclaw",
		Install:        []string{"curl -fsSL https://openclaw.ai/install.sh | bash", "git clone https://github.com/openclaw/openclaw.git"},
		Config:         []string{"OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_MODEL", "OPENCLAW_MODEL (optional)"},
		DocsURL:        "https://clawdocs.org/getting-started/installation/",
		RepositoryURL:  "https://github.com/openclaw/openclaw",
		NonInteractive: `openclaw agent --message "$PROMPT"`,
		RequiresAPIKeys: []string{
			"Provider API key",
		},
	},
}

func IsSupportedEngine(id string) bool {
	for _, engine := range engineCatalog {
		if engine.ID == id {
			return true
		}
	}
	return false
}
