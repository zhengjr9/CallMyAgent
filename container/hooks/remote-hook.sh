#!/bin/bash
# Claude Code Remote Session Hook v2
# Captures all conversation events and pushes to remote server
#
# Events captured:
#   SessionStart    → register session, optionally resume
#   UserPromptSubmit → capture user message
#   PreToolUse      → capture tool call before execution
#   PostToolUse     → capture tool result after execution
#   Stop            → push full transcript (all messages with full detail)
#   SessionEnd      → mark session as ended
set -euo pipefail

# ── Configuration ──────────────────────────────────
REMOTE_URL="${CLAUDE_REMOTE_URL:-http://localhost:9090}"
API_TOKEN="${CLAUDE_REMOTE_TOKEN:-}"

# ── Read hook input ────────────────────────────────
INPUT=$(cat)

SESSION_ID=$(echo "$INPUT" | jq -r '.session_id // "unknown"')
TRANSCRIPT_PATH=$(echo "$INPUT" | jq -r '.transcript_path // ""')
HOOK_EVENT=$(echo "$INPUT" | jq -r '.hook_event_name // "unknown"')
CWD=$(echo "$INPUT" | jq -r '.cwd // ""')
MODEL=$(echo "$INPUT" | jq -r '.model // ""')
SOURCE=$(echo "$INPUT" | jq -r '.source // ""')
PERMISSION_MODE=$(echo "$INPUT" | jq -r '.permission_mode // "default"')
AGENT_ID=$(echo "$INPUT" | jq -r '.agent_id // ""')
STOP_HOOK_ACTIVE=$(echo "$INPUT" | jq -r '.stop_hook_active // "false"')

# ── Helper: send HTTP request ─────────────────────
send_http() {
  local endpoint="$1"
  local payload="$2"
  local headers=(-H "Content-Type: application/json")
  if [ -n "$API_TOKEN" ]; then
    headers+=(-H "Authorization: Bearer $API_TOKEN")
  fi
  curl -sf --max-time 10 "${headers[@]}" \
    -d "$payload" \
    "${REMOTE_URL}${endpoint}" \
    >/dev/null 2>&1 || true
}

# ── Event: SessionStart ─────────────────────────────
# If session already exists in remote (from prior run), resume it
# Otherwise create new session and pull context
handle_session_start() {
  local check_resp
  check_resp=$(curl -sf --max-time 5 \
    -H "Authorization: Bearer $API_TOKEN" \
    "${REMOTE_URL}/api/sessions/$SESSION_ID" 2>/dev/null || echo "")

  if [ -n "$check_resp" ] && echo "$check_resp" | jq -e '.session_id' >/dev/null 2>&1; then
    # Session exists - this is a resume
    local resume_payload
    resume_payload=$(jq -n \
      --arg sid "$SESSION_ID" \
      --arg cwd "$CWD" \
      --arg model "$MODEL" \
      --arg source "$SOURCE" \
      '{
        session_id: $sid,
        cwd: $cwd,
        model: $model,
        source: $source,
        status: "resumed",
        resumed_at: (now | strftime("%Y-%m-%dT%H:%M:%SZ"))
      }')
    send_http "/api/sessions" "$resume_payload"
    log "Session resumed: $SESSION_ID"
  else
    # New session
    local new_payload
    new_payload=$(jq -n \
      --arg sid "$SESSION_ID" \
      --arg cwd "$CWD" \
      --arg model "$MODEL" \
      --arg source "$SOURCE" \
      '{
        session_id: $sid,
        cwd: $cwd,
        model: $model,
        source: $source,
        status: "active",
        started_at: (now | strftime("%Y-%m-%dT%H:%M:%SZ"))
      }')
    send_http "/api/sessions" "$new_payload"
    log "Session created: $SESSION_ID"
  fi
}

# ── Event: UserPromptSubmit ────────────────────────
handle_user_prompt_submit() {
  local prompt
  prompt=$(echo "$INPUT" | jq -r '.prompt // ""')

  local msg_payload
  msg_payload=$(jq -n \
    --arg sid "$SESSION_ID" \
    --arg role "user" \
    --arg content "$prompt" \
    '{
      session_id: $sid,
      role: $role,
      content: $content,
      timestamp: (now | strftime("%Y-%m-%dT%H:%M:%SZ"))
    }')
  send_http "/api/messages" "$msg_payload"
}

# ── Event: PreToolUse ─────────────────────────────
handle_tool_use() {
  local tool_name tool_input stop_reason
  tool_name=$(echo "$INPUT" | jq -r '.tool_name // ""')
  stop_reason=$(echo "$INPUT" | jq -r '.stop_reason // ""')

  # Build detailed tool input JSON
  local tool_input_json
  tool_input_json=$(echo "$INPUT" | jq -c '.tool_input // {}')

  # Determine if this is pre or post tool
  local event_type="$HOOK_EVENT"
  local is_pre=false
  if [ "$event_type" = "PreToolUse" ]; then
    is_pre=true
  fi

  local tool_payload
  tool_payload=$(jq -n \
    --arg sid "$SESSION_ID" \
    --arg event "$event_type" \
    --arg tool "$tool_name" \
    --argjson input "$tool_input_json" \
    --argjson pre "$is_pre" \
    --arg stop "$stop_reason" \
    '{
      session_id: $sid,
      event: $event,
      tool_name: $tool,
      tool_input: $input,
      is_pre_tool: $pre,
      stop_reason: $stop,
      timestamp: (now | strftime("%Y-%m-%dT%H:%M:%SZ"))
    }')
  send_http "/api/tools" "$tool_payload"
}

# ── Event: PostToolUse ───────────────────────────
handle_post_tool_use() {
  handle_tool_use
}

# ── Event: Stop ──────────────────────────────────
# Push full transcript with detailed message content
handle_stop() {
  # Skip if stop_hook_active (already blocked 8 times)
  if [ "$STOP_HOOK_ACTIVE" = "true" ]; then
    log "Stop hook active, skipping transcript push"
    return
  fi

  if [ -z "$TRANSCRIPT_PATH" ] || [ ! -f "$TRANSCRIPT_PATH" ]; then
    log "No transcript path or file not found: $TRANSCRIPT_PATH"
    return
  fi

  # Parse JSONL transcript with full details
  local transcript_data
  transcript_data=$(cat "$TRANSCRIPT_PATH" | jq -c '
    select(.type == "user" or .type == "assistant") |
    {
      type: .type,
      uuid: .uuid,
      timestamp: .timestamp,
      message: (
        .message // .
      )
    }
  ' 2>/dev/null || echo "")

  if [ -z "$transcript_data" ]; then
    log "No transcript data extracted"
    return
  fi

  # Build detailed messages array
  local messages_json=""
  while IFS= read -r line; do
    [ -z "$line" ] && continue

    # Parse the message object
    local msg_obj
    msg_obj=$(echo "$line" | jq -c '.' 2>/dev/null || echo "")
    [ -z "$msg_obj" ] && continue

    # Extract with full detail
    local role content thinking tool_calls
    role=$(echo "$msg_obj" | jq -r '.message.role // .type')
    thinking=$(echo "$msg_obj" | jq -r '.message.thinking // empty')
    uuid=$(echo "$msg_obj" | jq -r '.uuid')

    # Build content array with full details
    content=$(echo "$msg_obj" | jq -c '
      .message.content // empty |
      if type == "array" then
        [.[] |
          if .type == "text" then
            {type: "text", text: .text}
          elif .type == "tool_use" then
            {type: "tool_use", id: .id, name: .name, input: .input}
          elif .type == "tool_result" then
            {type: "tool_result", tool_use_id: .tool_use_id, content: .content}
          else
            .
          end
        ]
      else
        [{"type": "text", "text": tostring}]
      end
    ' 2>/dev/null || echo "[]")

    local msg_json
    msg_json=$(jq -n \
      --arg role "$role" \
      --argjson content "$content" \
      --arg ts "$(echo "$msg_obj" | jq -r '.timestamp // empty')" \
      --arg uuid "$uuid" \
      --arg think "$thinking" \
      '{
        role: $role,
        content: $content,
        timestamp: $ts,
        uuid: $uuid,
        thinking: (if ($think | length) > 0 then $think else null end)
      }')

    if [ -n "$messages_json" ]; then
      messages_json="$messages_json,$msg_json"
    else
      messages_json="$msg_json"
    fi
  done <<< "$(echo "$transcript_data")"

  if [ -z "$messages_json" ]; then
    log "Failed to build messages array"
    return
  fi

  local transcript_payload
  transcript_payload=$(jq -n \
    --arg sid "$SESSION_ID" \
    --arg cwd "$CWD" \
    --argjson messages "[$messages_json]" \
    '{
      session_id: $sid,
      cwd: $cwd,
      messages: $messages,
      message_count: ($messages | length),
      timestamp: (now | strftime("%Y-%m-%dT%H:%M:%SZ"))
    }')

  send_http "/api/transcripts" "$transcript_payload"
  log "Transcript pushed: $SESSION_ID ($messages_json messages)"

  # Also store each message individually for quick access
  echo "$transcript_data" | while IFS= read -r line; do
    [ -z "$line" ] && continue
    msg_obj=$(echo "$line" | jq -c '.' 2>/dev/null || continue)
    role=$(echo "$msg_obj" | jq -r '.message.role // .type')
    content_raw=$(echo "$msg_obj" | jq -c '.message.content')
    ts=$(echo "$msg_obj" | jq -r '.timestamp // empty')
    uuid=$(echo "$msg_obj" | jq -r '.uuid')

    # Build content text for message store
    content_text=$(echo "$msg_obj" | jq -r '
      .message.content // empty |
      if type == "array" then
        [.[] | if .type == "text" then .text elif .type == "tool_use" then "[Tool: \(.name)]" elif .type == "tool_result" then "[Result]" else .type end] | join("\n")
      else
        tostring
      end
    ' 2>/dev/null || echo "")

    msg_payload=$(jq -n \
      --arg sid "$SESSION_ID" \
      --arg r "$role" \
      --arg ct "$content_text" \
      --argjson raw "$msg_obj" \
      --arg ts "$ts" \
      --arg uid "$uuid" \
      '{
        session_id: $sid,
        role: $r,
        content: $ct,
        raw: $raw,
        timestamp: $ts,
        uuid: $uid
      }')
    send_http "/api/messages" "$msg_payload"
  done
}

# ── Event: SessionEnd ─────────────────────────────
handle_session_end() {
  local end_payload
  end_payload=$(jq -n \
    --arg sid "$SESSION_ID" \
    '{
      session_id: $sid,
      status: "ended",
      ended_at: (now | strftime("%Y-%m-%dT%H:%M:%SZ"))
    }')
  send_http "/api/sessions" "$end_payload"
  log "Session ended: $SESSION_ID"
}

# ── Event: Always - push raw event ────────────────
send_raw_event() {
  local event_payload
  event_payload=$(jq -n \
    --arg sid "$SESSION_ID" \
    --arg event "$HOOK_EVENT" \
    --arg cwd "$CWD" \
    --arg transcript "$TRANSCRIPT_PATH" \
    --argjson raw "$INPUT" \
    '{
      session_id: $sid,
      event: $event,
      cwd: $cwd,
      transcript_path: $transcript,
      raw: $raw,
      timestamp: (now | strftime("%Y-%m-%dT%H:%M:%SZ"))
    }')
  send_http "/api/events" "$event_payload"
}

# ── Logging ───────────────────────────────────────
log() {
  echo "[remote-hook] $*" >&2
}

# ── Route by event ─────────────────────────────────
log "Event: $HOOK_EVENT | Session: $SESSION_ID | Cwd: $CWD"

case "$HOOK_EVENT" in
  SessionStart)
    handle_session_start
    ;;

  UserPromptSubmit)
    handle_user_prompt_submit
    ;;

  PreToolUse)
    handle_tool_use
    ;;

  PostToolUse|PostToolUseFailure)
    handle_post_tool_use
    ;;

  Stop)
    handle_stop
    ;;

  SessionEnd)
    handle_session_end
    ;;

  *)
    log "Unhandled event: $HOOK_EVENT"
    ;;
esac

# Always send raw event
send_raw_event

# Always exit 0 (don't block Claude)
exit 0