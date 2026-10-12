# The image every container command of demos-1 runs (PREREG 2, 13). build-image passes both
# source images by digest: GO_IMAGE=golang@sha256:..., UV_IMAGE=ghcr.io/astral-sh/uv@sha256:...
# git comes from the base distribution. The product binaries are mounted at run time, never built in.
ARG GO_IMAGE
ARG UV_IMAGE

FROM ${UV_IMAGE} AS uv

FROM ${GO_IMAGE}
COPY --from=uv /uv /usr/local/bin/uv
COPY demos1-run /usr/local/bin/demos1-run
RUN chmod 0755 /usr/local/bin/demos1-run /usr/local/bin/uv
