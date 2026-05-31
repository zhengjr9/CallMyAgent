#!/bin/bash
# Setup script for Claude Code Remote Session Hooks
# Installs hook scripts and configures settings.json
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# ── Configuration ──────────────────────────────────
REMOTE_URL="${CLAUDE_REMOTE_URL:-http://localhost:9090}"
REMOTE_TOKEN="${CLAUDE_REMOTE_TOKEN:-}"
SETTINGS_FILE="${CLAUDE_SETTINGS_FILE:-}"

# Auto-detect settings file
if [ -z "$SETTINGS_FILE" ]; then
  # Check project-level first
  if [ -f ".claude/settings.json" ]; then
    SETTINGS_FILE=".claude/settings.json"
  elif [ -f ".claude/settings.local.json" ]; then
    SETTINGS_FILE=".claude/settings.local.json"
  else
    # Default to user-level
    SETTINGS_FILE="$HOME/.claude/settings.json"
    mkdir -p "$(dirname "$SETTINGS_FILE")"
  fi
fi

echo "=========================================="
echo "  Claude Remote Session Hooks Setup"
echo "=========================================="
echo ""
echo "Settings file: $SETTINGS_FILE"
echo "Remote URL:    $REMOTE_URL"
echo ""

# ── Make scripts executable ────────────────────────
chmod +x "$SCRIPT_DIR/scripts/remote-hook.sh"

# ── Build hooks config ─────────────────────────────
HOOK_SCRIPT_PATH="$(cd "$SCRIPT_DIR/scripts" && pwd)/remote-hook.sh"

HOOKS_CONFIG=$(cat <<EOF
{
  "SessionStart": [
    {
      "hooks": [
        {
          "type": "command",
          "command": "$HOOK_SCRIPT_PATH"
        }
      ]
    }
  ],
  "UserPromptSubmit": [
    {
      "hooks": [
        {
          "type": "command",
          "command": "$HOOK_SCRIPT_PATH"
        }
      ]
    }
  ],
  "Stop": [
    {
      "hooks": [
        {
          "type": "command",
          "command": "$HOOK_SCRIPT_PATH"
        }
      ]
    }
  ],
  "PreToolUse": [
    {
      "matcher": "*",
      "hooks": [
        {
          "type": "command",
          "command": "$HOOK_SCRIPT_PATH"
        }
      ]
    }
  ],
  "PostToolUse": [
    {
      "matcher": "*",
      "hooks": [
        {
          "type": "command",
          "command": "$HOOK_SCRIPT_PATH"
        }
      ]
    }
  ],
  "SessionEnd": [
    {
      "hooks": [
        {
          "type": "command",
          "command": "$HOOK_SCRIPT_PATH"
        }
      ]
    }
  ]
}
EOF
)

# ── Merge into settings.json ───────────────────────
if [ -f "$SETTINGS_FILE" ]; then
  echo "Updating existing settings file..."
  # Merge hooks into existing settings
  EXISTING=$(cat "$SETTINGS_FILE")
  UPDATED=$(echo "$EXISTING" | jq --argjson hooks "$HOOKS_CONFIG" '.hooks = $hooks')
  echo "$UPDATED" > "$SETTINGS_FILE"
else
  echo "Creating new settings file..."
  # Create settings with hooks + env
  ENV_CONFIG='{}'
  if [ -n "$REMOTE_URL" ]; then
    ENV_CONFIG=$(echo "$ENV_CONFIG" | jq --arg url "$REMOTE_URL" '. + {"CLAUDE_REMOTE_URL": $url}')
  fi
  if [ -n "$REMOTE_TOKEN" ]; then
    ENV_CONFIG=$(echo "$ENV_CONFIG" | jq --arg tok "$REMOTE_TOKEN" '. + {"CLAUDE_REMOTE_TOKEN": $tok}')
  fi

  jq -n \
    --argjson hooks "$HOOKS_CONFIG" \
    --argjson env "$ENV_CONFIG" \
    '{
      hooks: $hooks,
      env: (if ($env | length) > 0 then $env else null end)
    } | with_entries(select(.value != null))' \
    > "$SETTINGS_FILE"
fi

echo ""
echo "Settings written to: $SETTINGS_FILE"
echo ""

# ── Verify ─────────────────────────────────────────
echo "Verifying configuration..."
HOOK_COUNT=$(cat "$SETTINGS_FILE" | jq '[.hooks | to_entries[] | .value | length] | add // 0')
echo "  Hooks configured: $HOOK_COUNT"

if cat "$SETTINGS_FILE" | jq -e '.hooks' >/dev/null 2>&1; then
  echo "  Status: OK"
else
  echo "  Status: ERROR - hooks not configured properly"
  exit 1
fi

echo ""
echo "=========================================="
echo "  Setup Complete"
echo "=========================================="
echo ""
echo "Configured hooks:"
cat "$SETTINGS_FILE" | jq -r '.hooks | keys[]' | sed 's/^/  - /'
echo ""
echo "To test, start the remote server:"
echo "  cd remote-server && go run . "
echo ""
echo "Then start Claude Code normally:"
echo "  claude"
echo ""
echo "Events will be pushed to: $REMOTE_URL"
