#!/bin/bash
set -euo pipefail

echo "=========================================="
echo "  CallMyAgent Worker"
echo "=========================================="
echo "Task ID:   ${CLAUDE_TASK_ID:-unknown}"
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
git config --global user.name "Claude Task Bot"
git config --global init.defaultBranch main

# Clone repository
if [ -n "${GIT_REPO:-}" ]; then
    echo "Cloning repository..."
    AUTH_REPO="$GIT_REPO"

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

    CODEX_CMD="codex exec --json --ephemeral -C /workspace/repo"
    if [ -d "/workspace/repo" ]; then
        CODEX_CMD="$CODEX_CMD --add-dir /workspace/repo"
    fi
    CODEX_CMD="$CODEX_CMD -- $PROMPT"

    echo "Executing: $CODEX_CMD"
    set +e
    eval "$CODEX_CMD" > /workspace/codex-result.jsonl 2>/workspace/codex-error.log
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
    opencode -p "$PROMPT" -f json > /workspace/opencode-result.json 2>/workspace/opencode-error.log
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
    hermes -z "$PROMPT" --max-turns ${MAX_TURNS:-20} > /workspace/hermes-result.json 2>/workspace/hermes-error.log
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
    openclaw exec "$PROMPT" > /workspace/openclaw-result.json 2>/workspace/openclaw-error.log
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

    CLAUDE_CMD="claude --permission-mode dontAsk --max-turns ${MAX_TURNS:-20} --max-budget-usd ${BUDGET_USD:-5.00}"
    CLAUDE_CMD="$CLAUDE_CMD --output-format json"
    CLAUDE_CMD="$CLAUDE_CMD --allowedTools 'Bash,Read,Edit,Write,Glob,Grep,WebFetch,WebSearch,TodoWrite,Task'"
    CLAUDE_CMD="$CLAUDE_CMD -p \"$(echo "$PROMPT" | sed 's/"/\\"/g')\""

    echo "Starting Claude..."
    echo "---"

    set +e
    eval "$CLAUDE_CMD" > /workspace/claude-result.json 2>/workspace/claude-error.log
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
        git commit -m "feat: automated development by Claude Task

Task ID: ${CLAUDE_TASK_ID:-unknown}
Engine: $ENGINE
Generated by Claude Code non-interactive mode"

        if [ -n "${GIT_TOKEN:-}" ]; then
            echo "Pushing to remote..."
            AUTH_REPO=$(echo "$GIT_REPO" | sed "s|https://|https://${GIT_TOKEN}@|")
            git remote set-url origin "$AUTH_REPO"
            git push origin HEAD:"${GIT_BRANCH:-main}" 2>&1 || echo "Push failed"
        else
            echo "No GIT_TOKEN set, skipping push"
            git log --oneline -5
        fi
    fi
fi

# Copy results to output volume if mounted
if [ -d "/output" ]; then
    cp /workspace/claude-result.json /workspace/claude-error.log /workspace/codex-result.jsonl /codex-error.log /output/ 2>/dev/null || true
    if [ -d "/workspace/repo" ]; then
        tar czf /output/repo-snapshot.tar.gz -C /workspace repo 2>/dev/null || true
    fi
    echo "Results copied to /output/"
fi

echo "=========================================="
echo "  Task completed (exit $EXIT_CODE)"
echo "=========================================="
exit $EXIT_CODE