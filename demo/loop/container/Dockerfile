# The image of every container of a live run of the agent loop: the project's
# Python test runner and git. The product binaries, the Claude Code CLI and the
# plugin trees are mounted at run time, read-only, never built in. build-image
# passes the base image by digest.
ARG BASE_IMAGE
FROM ${BASE_IMAGE}
ARG PYTEST_VERSION
RUN apt-get update \
 && apt-get install -y --no-install-recommends git ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && pip install --no-cache-dir "pytest==${PYTEST_VERSION}"
