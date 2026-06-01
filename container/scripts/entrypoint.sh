#!/bin/bash
set -euo pipefail

echo "=========================================="
echo "  CallMyAgent Worker"
echo "=========================================="
echo "Task ID:   ${CALLMYAGENT_TASK_ID:-unknown}"
echo "Git Repo:  ${GIT_REPO:-not set}"
echo "Git Branch: ${GIT_BRANCH:-main}"
echo "Engine:    ${TASK_ENGINE:-claude}"
echo "Max Turns: ${MAX_TURNS:-20}"
echo "Budget:    ${BUDGET_USD:-5.00}"
echo "=========================================="

# Read prompt
PROMPT_FILE="/prompt/prompt.txt"
if [ ! -f "$PROMPT_FILE" ]; then
    echo "ERROR: Prompt file not found at $PROMPT_FILE"
    exit 1
fi

PROMPT=$(cat "$PROMPT_FILE")
echo "Prompt loaded (${#PROMPT} chars)"

# Configure git
git config --global user.email "CallMyAgent@automated"
git config --global user.name "CallMyAgent Bot"
git config --global init.defaultBranch main

# Clone repository
if [ -n "${GIT_REPO:-}" ]; then
    echo "Cloning repository..."
    AUTH_REPO="$GIT_REPO"

    if [ -f "${HOME:-/home/claude}/.netrc" ]; then
        echo "Git credentials: using ${HOME:-/home/claude}/.netrc"
    fi

    if [ -n "${GIT_TOKEN:-}" ]; then
        AUTH_REPO=$(echo "$GIT_REPO" | sed "s|https://|https://${GIT_TOKEN}@|")
    fi

    git clone --branch "${GIT_BRANCH:-main}" --depth 1 "$AUTH_REPO" /workspace/repo
    cd /workspace/repo
    echo "Repository cloned to /workspace/repo"
else
    echo "No git repo specified, working in /workspace"
    cd /workspace
fi

# Write prompt to a file for reference
echo "$PROMPT" > /workspace/claude-prompt.txt

# Determine which engine to use
ENGINE="${TASK_ENGINE:-claude}"
TASK_ID="${CALLMYAGENT_TASK_ID:-unknown}"

# Normalize OpenAI-compatible credentials for Codex and other OpenAI-style engines.
if [ -n "${CODEX_API_KEY:-}" ] && [ -z "${OPENAI_API_KEY:-}" ]; then
    export OPENAI_API_KEY="$CODEX_API_KEY"
fi
if [ -n "${CODEX_BASE_URL:-}" ] && [ -z "${OPENAI_BASE_URL:-}" ]; then
    export OPENAI_BASE_URL="$CODEX_BASE_URL"
fi
if [ -n "${CODEX_MODEL:-}" ] && [ -z "${OPENAI_MODEL:-}" ]; then
    export OPENAI_MODEL="$CODEX_MODEL"
fi

OPENAI_COMPAT_BASE_URL="${OPENAI_BASE_URL:-}"
OPENAI_COMPAT_MODEL="${OPENAI_MODEL:-deepseek-v4-flash}"

json_escape() {
    printf '%s' "${1:-}" | jq -Rsa .
}

write_opencode_config() {
    if [ -z "${OPENAI_COMPAT_BASE_URL:-}" ] || [ -z "${OPENAI_API_KEY:-}" ]; then
        return
    fi

    export CALLMYAGENT_API_KEY="${CALLMYAGENT_API_KEY:-$OPENAI_API_KEY}"
    export OPENCODE_CONFIG="${OPENCODE_CONFIG:-/tmp/callmyagent-opencode.json}"

    local base_json model_json
    base_json=$(json_escape "$OPENAI_COMPAT_BASE_URL")
    model_json=$(json_escape "$OPENAI_COMPAT_MODEL")

    cat > "$OPENCODE_CONFIG" << EOF
{
  "\$schema": "https://opencode.ai/config.json",
  "provider": {
    "callmyagent": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "CallMyAgent",
      "options": {
        "baseURL": $base_json,
        "apiKey": "$CALLMYAGENT_API_KEY"
      },
      "models": {
        $model_json: {
          "name": $model_json
        }
      }
    }
  }
}
EOF
    chmod 600 "$OPENCODE_CONFIG"
}

write_hermes_config() {
    if [ -z "${OPENAI_COMPAT_BASE_URL:-}" ] || [ -z "${OPENAI_API_KEY:-}" ]; then
        return
    fi

    mkdir -p "${HERMES_HOME:-/root/.hermes}"
    local hermes_config="${HERMES_CONFIG_PATH:-${HERMES_HOME:-/root/.hermes}/config.yaml}"
    cat > "$hermes_config" << EOF
model:
  default: "$OPENAI_COMPAT_MODEL"
  provider: "custom"
  base_url: "$OPENAI_COMPAT_BASE_URL"
  api_key: "$OPENAI_API_KEY"
terminal:
  backend: "local"
  cwd: "."
  timeout: 180
EOF
    chmod 600 "$hermes_config"
}

write_openclaw_config() {
    if [ -z "${OPENAI_COMPAT_BASE_URL:-}" ] || [ -z "${OPENAI_API_KEY:-}" ]; then
        return
    fi

    export CALLMYAGENT_API_KEY="${CALLMYAGENT_API_KEY:-$OPENAI_API_KEY}"
    export OPENCLAW_CONFIG_PATH="${OPENCLAW_CONFIG_PATH:-/tmp/callmyagent-openclaw.json}"
    export OPENCLAW_STATE_DIR="${OPENCLAW_STATE_DIR:-/tmp/callmyagent-openclaw}"
    mkdir -p "$(dirname "$OPENCLAW_CONFIG_PATH")" "$OPENCLAW_STATE_DIR"

    local base_json model_json
    base_json=$(json_escape "$OPENAI_COMPAT_BASE_URL")
    model_json=$(json_escape "$OPENAI_COMPAT_MODEL")

    cat > "$OPENCLAW_CONFIG_PATH" << EOF
{
  "\$schema": "https://docs.openclaw.ai/schema/openclaw.json",
  "env": {
    "shellEnv": {
      "enabled": false
    }
  },
  "models": {
    "mode": "merge",
    "pricing": {
      "enabled": false
    },
    "providers": {
      "callmyagent": {
        "baseUrl": $base_json,
        "apiKey": "$CALLMYAGENT_API_KEY",
        "auth": "token",
        "api": "openai-completions",
        "contextWindow": 128000,
        "contextTokens": 128000,
        "maxTokens": 8192,
        "timeoutSeconds": 600,
        "models": [
          {
            "id": $model_json,
            "name": $model_json,
            "api": "openai-completions",
            "contextWindow": 128000,
            "contextTokens": 128000,
            "maxTokens": 8192,
            "input": ["text"],
            "compat": {
              "supportsTools": true,
              "supportsStrictMode": false,
              "supportsDeveloperRole": false
            }
          }
        ]
      }
    }
  },
  "agents": {
    "defaults": {
      "model": "callmyagent/$OPENAI_COMPAT_MODEL",
      "workspace": "/workspace",
      "timeoutSeconds": 600,
      "verboseDefault": "off"
    }
  }
}
EOF
    chmod 600 "$OPENCLAW_CONFIG_PATH"
}

# ── Remote hook setup ─────────────────────────────
# If CLAUDE_REMOTE_URL is set, configure hooks to push events
if [ -n "${CLAUDE_REMOTE_URL:-}" ]; then
    echo "Configuring remote hooks: $CLAUDE_REMOTE_URL"
    export CLAUDE_REMOTE_URL

    # Write hooks config - merge with existing settings if present
    mkdir -p /home/claude/.claude
    if [ -f /home/claude/.claude/settings.json ] && [ -s /home/claude/.claude/settings.json ]; then
        # Backup original
        cp /home/claude/.claude/settings.json /home/claude/.claude/settings.json.bak
        # Extract permissions and model from original, add hooks
        PERMS=$(jq -r '.permissions' /home/claude/.claude/settings.json.bak)
        MODEL=$(jq -r '.model // "claude-sonnet-4-5-20250514"' /home/claude/.claude/settings.json.bak)
        cat > /home/claude/.claude/settings.json << EOF
{
  "permissions": $PERMS,
  "model": "$MODEL",
  "hooks": {
    "SessionStart": [{ "hooks": [{ "type": "command", "command": "/scripts/remote-hook.sh" }] }],
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "/scripts/remote-hook.sh" }] }],
    "Stop": [{ "hooks": [{ "type": "command", "command": "/scripts/remote-hook.sh" }] }],
    "PreToolUse": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "/scripts/remote-hook.sh" }] }],
    "PostToolUse": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "/scripts/remote-hook.sh" }] }],
    "SessionEnd": [{ "hooks": [{ "type": "command", "command": "/scripts/remote-hook.sh" }] }]
  },
  "env": {
    "CLAUDE_REMOTE_URL": "${CLAUDE_REMOTE_URL}",
    "CLAUDE_REMOTE_TOKEN": "${CLAUDE_REMOTE_TOKEN:-}"
  }
}
EOF
    else
        cat > /home/claude/.claude/settings.json << 'HOOKS_EOF'
{
  "hooks": {
    "SessionStart": [{ "hooks": [{ "type": "command", "command": "/scripts/remote-hook.sh" }] }],
    "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "/scripts/remote-hook.sh" }] }],
    "Stop": [{ "hooks": [{ "type": "command", "command": "/scripts/remote-hook.sh" }] }],
    "PreToolUse": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "/scripts/remote-hook.sh" }] }],
    "PostToolUse": [{ "matcher": "*", "hooks": [{ "type": "command", "command": "/scripts/remote-hook.sh" }] }],
    "SessionEnd": [{ "hooks": [{ "type": "command", "command": "/scripts/remote-hook.sh" }] }]
  },
  "env": {
    "CLAUDE_REMOTE_URL": "'"$CLAUDE_REMOTE_URL"'",
    "CLAUDE_REMOTE_TOKEN": "'"${CLAUDE_REMOTE_TOKEN:-}"'"
  }
}
HOOKS_EOF
    fi

    # Make sure hook script is executable
    chmod +x /scripts/remote-hook.sh
    echo "Hooks configured with remote URL: $CLAUDE_REMOTE_URL"
fi

# ── Execute based on engine ──────────────────────────
EXIT_CODE=0

case "$ENGINE" in
  codex)
    echo ""
    echo "=== Running Codex Engine ==="
    echo "PROMPT: ${PROMPT:0:200}..."

    CODEX_CMD=(codex exec --json --ephemeral --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox)
    if [ -n "${CODEX_MODEL:-${OPENAI_MODEL:-}}" ]; then
        CODEX_CMD+=(-m "${CODEX_MODEL:-${OPENAI_MODEL:-}}")
    fi
    if [ -n "${CODEX_BASE_URL:-${OPENAI_BASE_URL:-}}" ]; then
        CODEX_CMD+=(
            -c 'model_provider="callmyagent"'
            -c 'model_providers.callmyagent.name="CallMyAgent OpenAI-compatible"'
            -c "model_providers.callmyagent.base_url=\"${CODEX_BASE_URL:-${OPENAI_BASE_URL:-}}\""
            -c 'model_providers.callmyagent.wire_api="responses"'
            -c 'model_providers.callmyagent.env_key="CODEX_API_KEY"'
            -c 'model_providers.callmyagent.supports_websockets=false'
        )
    fi
    if [ -d "/workspace/repo" ]; then
        CODEX_CMD+=(-C /workspace/repo --add-dir /workspace/repo)
    fi
    CODEX_CMD+=(-- "$PROMPT")

    echo "Executing: ${CODEX_CMD[*]}"
    set +e
    "${CODEX_CMD[@]}" > /workspace/codex-result.jsonl 2>/workspace/codex-error.log
    EXIT_CODE=$?
    set -e

    echo "Codex finished with exit code: $EXIT_CODE"
    if [ -f /workspace/codex-result.jsonl ]; then
        echo "Result saved to /workspace/codex-result.jsonl ($(wc -l < /workspace/codex-result.jsonl) lines)"
    fi
    ;;

  opencode)
    echo ""
    echo "=== Running OpenCode Engine ==="
    echo "PROMPT: ${PROMPT:0:200}..."

    set +e
    write_opencode_config
    if [ -z "${OPENCODE_MODEL:-}" ] && [ -n "${OPENAI_COMPAT_BASE_URL:-}" ] && [ -n "${OPENAI_API_KEY:-}" ]; then
        OPENCODE_MODEL="callmyagent/$OPENAI_COMPAT_MODEL"
    elif [ -n "${ANTHROPIC_BASE_URL:-}" ] && [[ "${ANTHROPIC_BASE_URL}" != */v1 ]]; then
        export ANTHROPIC_BASE_URL="${ANTHROPIC_BASE_URL%/}/v1"
    fi
    OPENCODE_ARGS=(opencode run --dangerously-skip-permissions --format json)
    if [ -n "${OPENCODE_MODEL:-}" ]; then
        OPENCODE_ARGS+=(--model "$OPENCODE_MODEL")
    elif [ -n "${CLAUDE_MODEL:-}" ]; then
        OPENCODE_ARGS+=(--model "anthropic/$CLAUDE_MODEL")
    else
        OPENCODE_ARGS+=(--model "anthropic/claude-sonnet-4-5-20250514")
    fi
    if opencode run --help >/dev/null 2>&1; then
        "${OPENCODE_ARGS[@]}" "$PROMPT" > /workspace/opencode-result.json 2>/workspace/opencode-error.log
    else
        opencode -p "$PROMPT" -f json > /workspace/opencode-result.json 2>/workspace/opencode-error.log
    fi
    EXIT_CODE=$?
    set -e

    echo "OpenCode finished with exit code: $EXIT_CODE"
    if [ -f /workspace/opencode-result.json ]; then
        echo "Result saved to /workspace/opencode-result.json"
    fi
    ;;

  hermes)
    echo ""
    echo "=== Running Hermes Engine ==="
    echo "PROMPT: ${PROMPT:0:200}..."

    set +e
    if [ -z "${HERMES_PROVIDER:-}" ] && [ -n "${ANTHROPIC_API_KEY:-}" ]; then
        HERMES_PROVIDER="anthropic"
        HERMES_MODEL="${HERMES_MODEL:-${CLAUDE_MODEL:-claude-sonnet-4-20250514}}"
    else
        write_hermes_config
        HERMES_PROVIDER="${HERMES_PROVIDER:-custom}"
        HERMES_MODEL="${HERMES_MODEL:-$OPENAI_COMPAT_MODEL}"
    fi
    hermes -z "$PROMPT" --provider "$HERMES_PROVIDER" -m "$HERMES_MODEL" --yolo > /workspace/hermes-result.json 2>/workspace/hermes-error.log
    EXIT_CODE=$?
    set -e

    echo "Hermes finished with exit code: $EXIT_CODE"
    if [ -f /workspace/hermes-result.json ]; then
        echo "Result saved to /workspace/hermes-result.json"
    fi
    ;;

  openclaw)
    echo ""
    echo "=== Running OpenClaw Engine ==="
    echo "PROMPT: ${PROMPT:0:200}..."

    set +e
    if openclaw agent --help >/dev/null 2>&1; then
        write_openclaw_config
        OPENCLAW_MODEL="${OPENCLAW_MODEL:-callmyagent/$OPENAI_COMPAT_MODEL}"
        openclaw agent --local --json --session-key "callmyagent-${TASK_ID}" --model "$OPENCLAW_MODEL" --message "$PROMPT" > /workspace/openclaw-result.json 2>/workspace/openclaw-error.log
    else
        openclaw exec "$PROMPT" > /workspace/openclaw-result.json 2>/workspace/openclaw-error.log
    fi
    EXIT_CODE=$?
    set -e

    echo "OpenClaw finished with exit code: $EXIT_CODE"
    if [ -f /workspace/openclaw-result.json ]; then
        echo "Result saved to /workspace/openclaw-result.json"
    fi
    ;;

  claude|*)
    echo ""
    echo "=== Running Claude Engine ==="
    echo "PROMPT: ${PROMPT:0:200}..."

    echo "Starting Claude..."
    echo "---"

    set +e
    claude \
        --permission-mode dontAsk \
        --max-turns "${MAX_TURNS:-20}" \
        --max-budget-usd "${BUDGET_USD:-5.00}" \
        --output-format json \
        --allowedTools "Bash,Read,Edit,Write,Glob,Grep,WebFetch,WebSearch,TodoWrite,Task" \
        -p "$PROMPT" > /workspace/claude-result.json 2>/workspace/claude-error.log
    EXIT_CODE=$?
    set -e

    echo "---"
    echo "Claude finished with exit code: $EXIT_CODE"

    if [ -f /workspace/claude-result.json ]; then
        echo "Result saved to /workspace/claude-result.json"
        COST=$(jq -r '.total_cost_usd // "N/A"' /workspace/claude-result.json 2>/dev/null || echo "N/A")
        echo "Total cost: $COST"
    fi
    ;;
esac

# ── Push changes if in a git repo ─────────────────
if [ -d "/workspace/repo/.git" ] && [ -n "${GIT_REPO:-}" ]; then
    cd /workspace/repo

    # Check for changes (untracked files are not detected by git diff --quiet)
    CHANGES=$(git status --porcelain)
    if [ -z "$CHANGES" ]; then
        echo "No changes to commit"
    else
        echo "Committing changes..."
        git add -A
        git commit -m "feat: automated development by CallMyAgent

Task ID: $TASK_ID
Engine: $ENGINE
Generated by CallMyAgent"

        if [ -n "${GIT_TOKEN:-}" ] || [ -f "${HOME:-/home/claude}/.netrc" ]; then
            echo "Pushing to remote..."
            if [ -n "${GIT_TOKEN:-}" ]; then
                AUTH_REPO=$(echo "$GIT_REPO" | sed "s|https://|https://${GIT_TOKEN}@|")
                git remote set-url origin "$AUTH_REPO"
            fi
            git push origin HEAD:"${GIT_BRANCH:-main}" 2>&1 || echo "Push failed"
        else
            echo "No GIT_TOKEN or ~/.netrc set, skipping push"
            git log --oneline -5
        fi
    fi
fi

# Copy results to output volume if mounted
if [ -d "/output" ]; then
    cp /workspace/*-result.* /workspace/*-error.log /output/ 2>/dev/null || true
    if [ -d "/workspace/repo" ]; then
        tar czf /output/repo-snapshot.tar.gz -C /workspace repo 2>/dev/null || true
    fi
    echo "Results copied to /output/"
fi

echo "=========================================="
echo "  Task completed (exit $EXIT_CODE)"
echo "=========================================="
exit $EXIT_CODE
