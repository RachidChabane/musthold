# The one image of every container of the agent-loop study: git, the uv binary, CPython 3.13.14,
# pytest from the hashed lock, and Claude Code from its npm package. The product binaries and the
# plugin trees are mounted at run time, read-only, never built in. script/build passes the source
# images by digest and the Claude Code version.
#
# Nothing here needs root at run time: every file is world-readable and the container runs as
# the host user's uid.
ARG NODE_IMAGE
ARG UV_IMAGE

FROM ${UV_IMAGE} AS uv

FROM ${NODE_IMAGE}
ARG CLAUDE_CODE_VERSION
ARG PYTHON_VERSION=3.13.14
ENV UV_PYTHON_INSTALL_DIR=/opt/python \
    UV_PYTHON_PREFERENCE=only-managed \
    UV_LINK_MODE=copy \
    UV_NO_CACHE=1 \
    DISABLE_AUTOUPDATER=1

RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates \
 && rm -rf /var/lib/apt/lists/*

COPY --from=uv /uv /usr/local/bin/uv

COPY requirements.lock /tmp/requirements.lock
RUN uv python install "${PYTHON_VERSION}" \
 && uv venv /opt/venv --python "${PYTHON_VERSION}" \
 && uv pip install --python /opt/venv/bin/python --require-hashes --no-deps -r /tmp/requirements.lock \
 && ln -s /opt/venv/bin/python /usr/local/bin/python \
 && ln -s /opt/venv/bin/python /usr/local/bin/python3 \
 && ln -s /opt/venv/bin/pytest /usr/local/bin/pytest \
 && rm /tmp/requirements.lock

RUN npm install --global --no-audit --no-fund "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" \
 && npm cache clean --force \
 && claude --version
