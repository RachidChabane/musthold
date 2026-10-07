"""`ConstructionModel` over the `claude` CLI: one isolated single-shot call per request.

Isolation, as the pilot's adapter established it:

- no tools, no settings source, no MCP server, no slash command, no session persistence;
- the call runs in a fresh empty directory that must still be empty afterwards;
- the nesting variables of an enclosing Claude Code session are removed from the environment;
- the `$schema` meta-schema key, which the CLI rejects, is stripped from the schema argument only;
- the response must be a result whose `modelUsage` names exactly the pinned model, and whose input tokens stay
  within the request's own size plus a fixed slack, so ambient context cannot enter unnoticed.
"""

from __future__ import annotations

import json
import os
import shutil
import tempfile
from collections.abc import Mapping
from pathlib import Path
from typing import Final

from specbench.application.ports import PromptRunner
from specbench.domain.canonical import JsonValue
from specbench.domain.construction import (
    IsolationViolation,
    ModelCallFailed,
    ModelRequest,
    ModelResponse,
    QuotaExhausted,
    response_from_stdout,
)
from specbench.domain.pacing import looks_like_quota_exhausted

CLAUDE_EXECUTABLE: Final = "claude"
NESTING_ENV_VARS: Final = ("CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT")
SCHEMA_DIALECT_KEY: Final = "$schema"
CALL_DIRECTORY_PREFIX: Final = "specbench-call-"


def schema_argument(schema: JsonValue) -> str:
    """The schema as text the CLI accepts: without the `$schema` key."""
    if isinstance(schema, Mapping):
        schema = {key: value for key, value in schema.items() if key != SCHEMA_DIALECT_KEY}
    return json.dumps(schema, sort_keys=True)


def claude_argv(request: ModelRequest) -> list[str]:
    argv = [CLAUDE_EXECUTABLE, "-p", "--model", request.model]
    if request.effort is not None:
        argv += ["--effort", request.effort]
    return [
        *argv,
        "--system-prompt",
        request.system_prompt,
        "--tools",
        "",
        "--setting-sources",
        "",
        "--strict-mcp-config",
        "--disable-slash-commands",
        "--no-session-persistence",
        "--output-format",
        "json",
        "--json-schema",
        schema_argument(request.schema),
    ]


def isolated_environment(environ: Mapping[str, str] | None = None) -> dict[str, str]:
    return {k: v for k, v in (environ if environ is not None else os.environ).items() if k not in NESTING_ENV_VARS}


class ClaudeCliModel:
    """Runs one request per call; the process runner is the only way it reaches the CLI."""

    def __init__(self, runner: PromptRunner, executable: str = CLAUDE_EXECUTABLE) -> None:
        self._runner = runner
        self._executable = executable

    def call(self, request: ModelRequest) -> ModelResponse:
        argv = claude_argv(request)
        argv[0] = self._executable
        call_dir = Path(tempfile.mkdtemp(prefix=CALL_DIRECTORY_PREFIX))
        try:
            result = self._runner.run_with_input(argv, request.prompt, call_dir, isolated_environment())
            leftovers = sorted(path.name for path in call_dir.iterdir())
        finally:
            shutil.rmtree(call_dir, ignore_errors=True)
        if result.returncode != 0:
            message = f"claude exited {result.returncode}: {result.stderr.strip()[:300]}"
            if looks_like_quota_exhausted(result.stderr, result.stdout):
                raise QuotaExhausted(message)
            raise ModelCallFailed(message)
        response = response_from_stdout(request, result.stdout)
        if leftovers:
            raise IsolationViolation(f"the call directory was not empty afterwards: {leftovers}")
        return response
