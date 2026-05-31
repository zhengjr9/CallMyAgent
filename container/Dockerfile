FROM node:20-slim

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl jq bash ca-certificates unzip wget \
    && rm -rf /var/lib/apt/lists/*

# ── Claude CLI ────────────────────────────────────
RUN npm install -g @anthropic-ai/claude-code@latest \
    && npm cache clean --force

# ── OpenCode CLI ──────────────────────────────────
RUN curl -fsSL https://raw.githubusercontent.com/opencode-ai/opencode/refs/heads/main/install | bash \
    || true  # OpenCode may not support all platforms

# ── Hermes CLI ────────────────────────────────────
RUN curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash \
    || true  # Hermes may not support all platforms

# ── OpenClaw CLI ────────────────────────────────
RUN npm install -g openclaw || true  # OpenClaw may not be available

# Create directories
RUN mkdir -p /home/claude/.claude /home/codex/.codex /scripts /workspace /output /skills

# Copy superpower skills (from container/skills/)
COPY skills/ /home/claude/.claude/

# Copy settings (from container/settings/)
COPY settings/settings.json /home/claude/.claude/settings.json
RUN if [ -f settings/codex.toml ]; then \
    cp settings/codex.toml /home/codex/.codex/config.toml; \
    fi

# Copy entrypoint script (from container/scripts/)
COPY scripts/entrypoint.sh /scripts/entrypoint.sh
RUN chmod +x /scripts/entrypoint.sh

# Copy remote hook script (from container/hooks/)
COPY hooks/remote-hook.sh /scripts/remote-hook.sh
RUN chmod +x /scripts/remote-hook.sh

# Environment
ENV HOME=/home/claude
ENV CODEX_HOME=/home/codex/.codex
ENV CLAUDE_CODE_MAX_TURNS=50

WORKDIR /workspace

USER root

ENTRYPOINT ["/bin/bash", "/scripts/entrypoint.sh"]