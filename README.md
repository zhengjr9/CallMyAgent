# Claude Task - Unified AI Development Platform

## Overview

A Kubernetes-based AI development platform that supports both **Codex** and **Claude** engines for automated code development through multi-round conversation planning.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Web UI (Unified)                          │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐ │
│   │  Tasks      │  │  Sessions   │  │  Transcript Viewer      │ │
│   │  (Meta Chat) │  │  (History)  │  │  (Full Message Tree)    │ │
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
   │ K8s Job    │                       │ Sessions   │
   │ Management │                       │ Transcripts│
   └─────┬──────┘                       └────────────┘
         │
         │ K8s Job
         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Worker Pod                                     │
│   ┌────────────────────────────────────────────────────────────┐ │
│   │ Container: CallMyAgent-worker                              │ │
│   │  ┌────────────┐  ┌────────────┐  ┌──────────────────────┐│ │
│   │  │   Claude   │  │   Codex    │  │   Hooks              ││ │
│   │  │   Engine   │  │   Engine   │  │   (Event Pusher)     ││ │
│   │  └────────────┘  └────────────┘  └──────────────────────┘│ │
│   │  Remote Hooks: SessionStart → Resume detection             │ │
│   │               Stop → Full transcript push                   │ │
│   └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Features

- [x] Meta conversation with Claude/Codex to plan development tasks
- [x] Multi-round chat with final prompt extraction
- [x] Kubernetes Job execution for automated development
- [x] Dual engine support (Claude CLI + Codex CLI)
- [x] Remote hooks for session event capture
- [x] Full transcript storage with universal format
- [x] Session resumption across engine restarts
- [x] Superpower skills in worker container

## Engine Comparison

| Feature | Claude | Codex |
|---------|--------|-------|
| CLI | `claude -p` | `codex exec` |
| Model Config | `ANTHROPIC_API_KEY` | `CODEX_API_KEY` |
| Hook Config | `settings.json` | `requirements.toml` |
| Transcript | `~/.claude/projects/*/*.jsonl` | `~/.codex/history.jsonl` |
| Session ID | UUID | Thread ID (UUID) |
| Non-interactive | `-p "prompt"` | `exec "prompt"` |
| JSON output | `--output-format json` | `--json` |

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
│   ├── scripts/entrypoint.sh
│   ├── skills/           # Superpower skills
│   └── settings/
│       ├── settings.json    # Claude settings
│       └── codex.toml       # Codex config
├── frontend/
│   └── index.html        # Unified Vue 3 SPA
├── hooks/
│   ├── scripts/remote-hook.sh
│   └── setup.sh
└── Makefile
```