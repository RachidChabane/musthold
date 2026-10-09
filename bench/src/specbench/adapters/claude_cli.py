"""`ConstructionModel` over the pinned `claude` CLI: one isolated single-shot call per request.

Isolation, as the pilot's adapter established it:

- no tools, no settings source, no MCP server, no slash command, no session persistence;
- the call runs in a fresh empty directory that must still be empty afterwards;
- the nesting variables of an enclosing Claude Code session are removed from the environment, and the CLI's
  self-update is off in it;
- the `$schema` meta-schema key, which the CLI rejects, is stripped from the schema argument only;
- the response must be a result whose `modelUsage` names exactly the pinned model, and whose input tokens stay
  within the baseline of an empty call at that model and effort plus the request's own size plus a fixed slack, and the
  baseline itself stays under a ceiling, so ambient context cannot enter unnoticed.
"""

from __future__ import annotations

import json
import os
import shutil
import tempfile
from collections.abc import Mapping
from pathlib import Path
from typing import Final

from specbench.adapters.pinned_claude_code import updater_disabled
from specbench.application.ports import PromptRunner
from specbench.domain.canonical import JsonValue
from specbench.domain.construction import (
    IsolationViolation,
    ModelCallFailed,
    ModelRequest,
    ModelResponse,
    QuotaExhausted,
    baseline_from_stdout,
    response_from_stdout,
)
from specbench.domain.pacing import looks_like_quota_exhausted

NESTING_ENV_VARS: Final = ("CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT")
SCHEMA_DIALECT_KEY: Final = "$schema"
CALL_DIRECTORY_PREFIX: Final = "specbench-call-"


def schema_argument(schema: JsonValue) -> str:
    """The schema as text the CLI accepts: without the `$schema` key."""
    if isinstance(schema, Mapping):
        schema = {key: value for key, value in schema.items() if key != SCHEMA_DIALECT_KEY}
    return json.dumps(schema, sort_keys=True)


def claude_argv(executable: str, request: ModelRequest) -> list[str]:
    argv = [executable, "-p", "--model", request.model]
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

    def __init__(self, runner: PromptRunner, executable: str) -> None:
        self._runner = runner
        self._executable = executable

    def call(self, request: ModelRequest, baseline_input_tokens: int) -> ModelResponse:
        return response_from_stdout(request, self._run(request), baseline_input_tokens)

    def measure_baseline(self, request: ModelRequest) -> ModelResponse:
        return baseline_from_stdout(request, self._run(request))

    def _run(self, request: ModelRequest) -> str:
        """The stdout of one isolated run, once the process and the call directory are known to be clean."""
        argv = claude_argv(self._executable, request)
        call_dir = Path(tempfile.mkdtemp(prefix=CALL_DIRECTORY_PREFIX))
        try:
            result = self._runner.run_with_input(
                argv, request.prompt, call_dir, updater_disabled(isolated_environment())
            )
            leftovers = sorted(path.name for path in call_dir.iterdir())
        finally:
            shutil.rmtree(call_dir, ignore_errors=True)
        if result.returncode != 0:
            message = f"claude exited {result.returncode}: {result.stderr.strip()[:300]}"
            if looks_like_quota_exhausted(result.stderr, result.stdout):
                raise QuotaExhausted(message)
            raise ModelCallFailed(message)
        if leftovers:
            raise IsolationViolation(f"the call directory was not empty afterwards: {leftovers}")
        return result.stdout
