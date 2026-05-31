# CallMyAgent - Unified AI Development Platform

## Overview

CallMyAgent is a Kubernetes-based AI development platform that supports **5 AI engines** for automated code development through natural conversation planning. It provides a web UI for task management, meta conversation with AI to refine requirements, and automated execution via Kubernetes Jobs.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Web UI (Vue 3 SPA)                        │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐ │
│   │  Tasks      │  │  Sessions   │  │  Transcript Viewer      │ │
│   │  Planning   │  │  History    │  │  (Full Message Tree)    │ │
│   └─────────────┘  └─────────────┘  └─────────────────────────┘ │
└────────────────────────────┬────────────────────────────────────┘
                            │ HTTP API
         ┌──────────────────┴──────────────────┐
         │                                     │
   ┌─────┴──────┐                       ┌──────┴─────┐
   │   Task     │                       │   Remote    │
   │   Server   │                       │   Server    │
   │  :8080     │                       │  :9090      │
   │            │                       │            │
   │ Meta Chat  │                       │ Hook Events│
   │ (CLI+Hooks)│                       │ Sessions   │
   │ K8s Job    │                       │ Transcripts│
   │ Management │                       │            │
   └─────┬──────┘                       └────────────┘
         │
         │ K8s Job
         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Worker Pod (callmyagent-worker)               │
│   ┌────────────────────────────────────────────────────────────┐ │
│   │ Container: linux/amd64                                      │ │
│   │  ┌────────────┐  ┌────────────┐  ┌──────────────────────┐│ │
│   │  │   Claude   │  │   Codex    │  │   OpenCode/Hermes/    ││ │
│   │  │   CLI      │  │   CLI      │  │   OpenClaw CLI       ││ │
│   │  └────────────┘  └────────────┘  └──────────────────────┘│ │
│   │  Remote Hooks: SessionStart → Resume detection             │ │
│   │               Stop → Full transcript push                   │ │
│   │  Skills: superpower capabilities                            │ │
│   └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Features

- [x] **Meta Agent** - Built-in Claude Code CLI with remote hooks for rich task planning with actual tool access
- [x] Multi-round chat with final prompt extraction
- [x] **5 Engine Support** - Claude, Codex, OpenCode, Hermes, OpenClaw
- [x] Kubernetes Job execution for automated development
- [x] Remote hooks for session event capture
- [x] Full transcript storage with universal format
- [x] Session resumption across engine restarts
- [x] Superpower skills in worker container

## Engine Comparison

| Engine | CLI | Install | Non-interactive |
|--------|-----|---------|----------------|
| Claude | `claude -p "prompt"` | npm install -g @anthropic-ai/claude-code | `--output-format json` |
| Codex | `codex exec --json --ephemeral` | npm install -g @opencode/codex | `--json` |
| OpenCode | `opencode -p "prompt" -f json` | curl script | `-f json` |
| Hermes | `hermes -z "prompt" --max-turns N` | curl script | structured output |
| OpenClaw | `openclaw exec "prompt"` | npm install -g openclaw | json output |

## Meta Agent Design

The meta agent uses **Claude Code CLI directly** with remote hooks instead of HTTP API calls. This gives it:

1. **Real tool access** - Can read files, glob patterns, run bash commands during planning
2. **Streaming events** - Every tool use, message, and session event is pushed via hooks
3. **Rich context** - Full transcript available for analysis
4. **Consistent UX** - Same interface as execution engines

### Meta Agent Flow

```
User Input → Frontend → Task Server → Claude Code CLI (local)
                                              ↓
                                    Remote Hook Events
                                              ↓
                                    Remote Server (store)
                                              ↓
                                    Frontend Sessions View
```

The meta agent runs Claude Code in non-interactive mode with:
- `--output-format json` for structured output
- Hooks configured to push events to remote server
- Custom system prompt for task refinement

## API Endpoints

### Task Server (Port 8080)
- `POST /api/tasks` - Create task
- `GET /api/tasks` - List tasks
- `POST /api/tasks/chat` - Chat with meta Claude
- `POST /api/tasks/execute` - Execute via K8s Job
- `GET /api/tasks/{id}` - Get task details

### Remote Server (Port 9090)
- `POST /api/events` - Hook event capture
- `POST /api/sessions` - Register/resume session
- `GET /api/sessions` - List sessions
- `GET /api/sessions/{id}/transcript` - Full transcript
- `GET /api/sessions/{id}/messages` - Session messages
- `GET /api/sessions/{id}/tools` - Tool calls

## Quick Start

```bash
# Build everything
make build

# Start servers
FRONTEND_DIR=./frontend ./build/remote-server &
PORT=8080 ./build/server &

# Open UI
open http://localhost:8080
```

## Environment Variables

| Variable | Server | Description |
|----------|--------|-------------|
| `ANTHROPIC_API_KEY` | Both | Claude API key |
| `ANTHROPIC_AUTH_TOKEN` | Both | Auth token for proxy |
| `ANTHROPIC_BASE_URL` | Both | API endpoint |
| `CLAUDE_MODEL` | Both | Model name |
| `CODEX_API_KEY` | Worker | Codex API key |
| `CLAUDE_REMOTE_URL` | Hook | Remote server URL |
| `KUBECONFIG` | Server | Kubernetes config |

## File Structure

```
.
├── backend/              # Task server (Go)
│   ├── main.go
│   ├── handler.go        # HTTP handlers
│   ├── store.go          # Memory task store
│   ├── k8s.go            # K8s Job creation
│   ├── claude.go         # Claude API client
│   └── types.go          # Data types
├── remote-server/        # Session/hook server (Go)
│   ├── main.go
│   ├── handler.go
│   ├── store.go          # Universal session store
│   └── types.go
├── container/            # Worker container
│   ├── Dockerfile
│   ├── scripts/
│   │   ├── entrypoint.sh
│   │   └── remote-hook.sh
│   ├── skills/           # Superpower skills
│   └── settings/
│       ├── settings.json    # Claude settings
│       └── codex.toml       # Codex config
├── frontend/
│   └── index.html        # Vue 3 SPA
├── hooks/
│   ├── remote-hook.sh
│   └── example-settings.json
└── Makefile
```
