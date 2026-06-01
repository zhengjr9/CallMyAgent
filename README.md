# CallMyAgent

CallMyAgent is a Kubernetes-based AI development platform for planning, dispatching, and observing AI coding agents. A built-in meta agent turns user requirements into a complete execution prompt, then a worker pod runs the selected engine against a Git repository.

## What It Does

- Uses a Claude-compatible meta agent to refine requirements and produce a `<final-prompt>...</final-prompt>`.
- Supports Claude Code, Codex CLI, OpenCode, Hermes Agent, and OpenClaw as execution engines.
- Runs each task as a Kubernetes Job with isolated workspace, prompt ConfigMap, and optional output PVC.
- Can also run tasks locally through Docker for development and single-node smoke tests.
- Captures Claude Code hook events, session transcripts, messages, and tool calls through the remote hook server.
- Provides a compact Vue UI styled after the new-api admin console: light workspace, left navigation, task/session panes, and engine import cards.

## Architecture

```text
Web UI
  -> Task Server :8080
       -> Meta Agent: Claude Code CLI when available, HTTP Anthropic-compatible API as fallback
       -> Kubernetes Job
            -> Worker image
                 -> claude | codex | opencode | hermes | openclaw
  -> Remote Server :9090
       -> Hook events, transcripts, session history
```

## Engines

| Engine | Command | Install | Configuration |
| --- | --- | --- | --- |
| Claude Code | `claude` | `npm install -g @anthropic-ai/claude-code` | `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL`, `CLAUDE_MODEL` |
| Codex CLI | `codex` | `npm install -g @openai/codex` | `CODEX_API_KEY`/`OPENAI_API_KEY`, `CODEX_BASE_URL`/`OPENAI_BASE_URL`, `CODEX_MODEL`/`OPENAI_MODEL` |
| OpenCode | `opencode` | `curl -fsSL https://opencode.ai/install \| bash` or `npm install -g opencode-ai` | dynamic `callmyagent` custom provider from `OPENAI_*`; optional `OPENCODE_MODEL` |
| Hermes Agent | `hermes` | `curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh \| bash` | `ANTHROPIC_*` by default when present; `HERMES_PROVIDER=custom` for OpenAI-compatible `OPENAI_*` |
| OpenClaw | `openclaw` | `curl -fsSL https://openclaw.ai/install.sh \| bash` or `npm i -g openclaw` | dynamic `callmyagent` custom provider from `OPENAI_*`; optional `OPENCLAW_MODEL` |

References checked while updating this repo:

- OpenCode install/config: https://pkg.go.dev/github.com/sst/opencode and https://opencode.ai/docs
- Hermes Agent quick install: https://github.com/NousResearch/hermes-agent
- OpenClaw install/onboarding: https://clawdocs.org/getting-started/installation/
- Codex CLI: https://github.com/openai/codex
- Claude Code: https://docs.anthropic.com/en/docs/claude-code

## Meta Agent

The task server uses `META_AGENT_MODE`:

- `auto` (default): use local `claude` CLI if it is installed, otherwise call the Anthropic-compatible HTTP API.
- `cli`: force Claude Code CLI.
- `http`: force direct HTTP.

For HTTP mode, set:

```bash
export ANTHROPIC_API_KEY=...
export ANTHROPIC_BASE_URL=https://api.anthropic.com
export CLAUDE_MODEL=claude-sonnet-4-5-20250514
```

For Claude Code CLI mode, install `claude` on the server host or include it in the server image. Remote hook capture can be enabled with:

```bash
export CLAUDE_REMOTE_URL=http://localhost:9090
```

## Quick Start

```bash
make build

FRONTEND_DIR=./frontend PORT=9090 go run ./remote-server &
PORT=8080 META_AGENT_MODE=auto go run ./backend
```

Open http://localhost:8080.

## OpenAI-Compatible Codex Example

Use environment variables instead of writing keys into files:

```bash
export CODEX_BASE_URL=https://one-openapi.cloud/v1
export CODEX_MODEL=deepseek-v4-flash
export CODEX_API_KEY=...
```

The worker normalizes these into `OPENAI_BASE_URL`, `OPENAI_MODEL`, and `OPENAI_API_KEY` for CLIs that expect OpenAI-compatible names.

## Scheduling

CallMyAgent supports two execution modes:

- `SCHEDULER_MODE=job`: create a Kubernetes Job. This is the production/default mode.
- `SCHEDULER_MODE=docker`: run `docker run -d` on the local host. This is useful for local development and page-to-worker smoke tests.

Docker mode uses these settings:

```bash
export SCHEDULER_MODE=docker
export CONTAINER_IMAGE=callmyagent-worker:local
export DOCKER_RUNS_DIR=/tmp/callmyagent-runs
```

The page execution form also sends `schedulerMode`, so a single server can accept either `docker` or `job` per task.

## Sessions And Resume

The task server proxies `/api/sessions`, `/api/events`, `/api/messages`, `/api/transcripts`, and `/api/tools` to the remote session server configured by `REMOTE_SERVER_URL` (default `http://127.0.0.1:9090`). Docker workers receive a container-reachable `CLAUDE_REMOTE_URL` such as `http://host.docker.internal:9090`, so Claude Code hooks can sync transcripts back to the page.

Claude Code sessions are captured through hooks. Codex runs do not use Claude hooks, so the worker converts `codex exec --json` output into the same session/transcript shape after execution. The remote server also exposes:

```text
GET /api/sessions/{session_id}/conversation?target=callmyagent|codex
POST /api/tasks/resume-session
```

The Sessions page uses this to restore an existing Claude or Codex transcript as a new task conversation, so an agent can continue from prior context with the selected engine.

## Git Credentials

The execution page supports private repositories through `.netrc` credentials:

- Paste a Git token in the execution form to generate a runtime-only `.netrc` for the task.
- Or leave the token empty and enable "Use server ~/.netrc" to copy the task server user's `~/.netrc`.

For Docker mode, CallMyAgent writes the generated `.netrc` under `DOCKER_RUNS_DIR/<task>/auth/.netrc` with `0600` permissions and mounts it read-only to `/home/claude/.netrc`. For Kubernetes Job mode, it creates a task-scoped Secret and mounts the same path. Tokens are not returned by the task API and are not passed as Docker environment variables.

Token-only input uses this generated format:

```text
machine <repo-host>
  login x-access-token
  password <token>
```

If you need a different login name or multiple hosts, paste a complete `.netrc` body beginning with `machine`.

## Custom Providers

When `OPENAI_BASE_URL`, `OPENAI_MODEL`, and `OPENAI_API_KEY` are present, the worker creates runtime-only provider configs:

- OpenCode: `/tmp/callmyagent-opencode.json`, provider `callmyagent`, model `callmyagent/<OPENAI_MODEL>`.
- OpenClaw: `/tmp/callmyagent-openclaw.json`, provider `callmyagent`, model `callmyagent/<OPENAI_MODEL>`.
- Hermes: `~/.hermes/config.yaml`, provider `custom`, model `<OPENAI_MODEL>` when `HERMES_PROVIDER=custom` or no Anthropic credentials are present.

If `ANTHROPIC_API_KEY` is set and `HERMES_PROVIDER` is not set, Hermes uses its native `anthropic` provider because that path is more reliable with Anthropic-compatible MiniMax endpoints. OpenCode still normalizes `ANTHROPIC_BASE_URL` to include `/v1` when using Anthropic directly.

## Kubernetes

```bash
kubectl apply -f k8s/rbac.yaml
kubectl apply -f k8s/secret.yaml
kubectl apply -f k8s/pvc.yaml
kubectl apply -f k8s/deployment.yaml
```

Update `k8s/secret.yaml` with real secrets before deploying. Do not commit real keys.

## Local Verification

```bash
go test ./backend
go test ./remote-server
make build-server
```

To verify agent binaries inside the worker image:

```bash
docker build -t callmyagent-worker:local -f container/Dockerfile container/
docker run --rm callmyagent-worker:local bash -lc 'claude --version; codex --version; opencode --version; hermes --version; openclaw --version'
```
