FROM node:20-slim

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git curl jq bash ca-certificates unzip wget \
    && rm -rf /var/lib/apt/lists/*

# ── Claude CLI ────────────────────────────────────
RUN npm install -g @anthropic-ai/claude-code@latest \
    && npm cache clean --force

# ── Codex CLI ─────────────────────────────────────
RUN npm install -g @openai/codex@latest \
    && npm cache clean --force

# ── OpenCode CLI ──────────────────────────────────
RUN curl -fsSL https://opencode.ai/install | bash \
    || npm install -g opencode-ai \
    || true

# ── Hermes CLI ────────────────────────────────────
RUN curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash \
    || true

# ── OpenClaw CLI ────────────────────────────────
RUN curl -fsSL https://openclaw.ai/install.sh | bash \
    || npm install -g openclaw \
    || true

RUN if [ -x /root/.opencode/bin/opencode ]; then \
    ln -sf /root/.opencode/bin/opencode /usr/local/bin/opencode; \
    fi

RUN if [ -x /usr/bin/node ]; then ln -sf /usr/bin/node /usr/local/bin/node; fi \
    && if [ -x /usr/bin/npm ]; then ln -sf /usr/bin/npm /usr/local/bin/npm; fi

RUN CODEX_VERSION="$(node -p "require('/usr/local/lib/node_modules/@openai/codex/package.json').version")" \
    && ARCH="$(node -p "process.arch === 'arm64' ? 'arm64' : 'x64'")" \
    && npm install -g --prefix /usr/local "@openai/codex-linux-${ARCH}@npm:@openai/codex@${CODEX_VERSION}-linux-${ARCH}" \
    || true

# Create directories
RUN mkdir -p /home/claude/.claude /home/codex/.codex /scripts /workspace /output /skills

# Copy superpower skills (from container/skills/)
COPY skills/ /home/claude/.claude/

# Copy settings (from container/settings/)
COPY settings/settings.json /home/claude/.claude/settings.json
COPY settings/ /tmp/callmyagent-settings/
RUN if [ -f /tmp/callmyagent-settings/codex.toml ]; then \
    cp /tmp/callmyagent-settings/codex.toml /home/codex/.codex/config.toml; \
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
