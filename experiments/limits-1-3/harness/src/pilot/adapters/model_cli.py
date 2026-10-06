"""Model adapters behind one `ModelPort`: Claude CLI and Codex CLI (PREREG 4.2 to 4.4).

Template Method: `_CliModel.call` owns resume, retry and raw storage; each subclass
supplies its argv, its isolation checks and its response parser.
"""

from __future__ import annotations

import json
import os
import shutil
import tempfile
from abc import ABC, abstractmethod
from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from pilot.adapters.process import ProcessResult, ProcessRunner
from pilot.domain.constants import CLAUDE_NESTING_ENV_VARS, MODEL_CALL_DEADLINE_SECONDS, MODEL_RETRY_COUNT
from pilot.domain.isolation import (
    codex_item_violations,
    leftover_files_violation,
    num_turns_violation,
    pinned_model_violation,
    token_violation,
)
from pilot.domain.model_call import (
    CanaryBaseline,
    MissingResult,
    ModelOutcome,
    ModelRequest,
    ModelResult,
    TokenUsage,
    outcome_from_mapping,
)

CLAUDE_EXECUTABLE = "claude"
CODEX_EXECUTABLE = "codex"
CODEX_OUTPUT_FILENAME = "out.json"
CODEX_DISABLED_FEATURES = (
    "shell_tool",
    "hooks",
    "plugins",
    "apps",
    "browser_use",
    "computer_use",
    "in_app_browser",
    "skill_search",
    "tool_suggest",
    "sleep_tool",
    "shell_snapshot",
)
REQUEST_FILENAME = "request.txt"
ARGV_FILENAME = "argv.json"
SCHEMA_FILENAME = "schema.json"
STDOUT_FILENAME = "stdout.txt"
STDERR_FILENAME = "stderr.txt"
EVENTS_FILENAME = "events.jsonl"
CHECKS_FILENAME = "checks.json"
RESULT_FILENAME = "result.json"
ATTEMPT_DIR_TEMPLATE = "attempt-{number}"
CALL_DIR_PREFIX = "pilot-call-"


@dataclass(frozen=True)
class ParsedCall:
    """What a CLI response yielded, before the baseline-dependent checks."""

    payload: Any = None
    total_input_tokens: int = 0
    output_tokens: int = 0
    cost_usd: float | None = None
    num_turns: int | None = None
    failures: list[str] = field(default_factory=list)
    violations: list[str] = field(default_factory=list)


class _CliModel(ABC):
    stdout_filename = STDOUT_FILENAME
    artifact_names: tuple[str, ...] = ()

    def __init__(
        self,
        process: ProcessRunner,
        raw_dir: Path,
        call_parent: Path | None = None,
    ) -> None:
        self._process = process
        self._raw_dir = raw_dir
        self._call_parent = call_parent

    def call(self, request: ModelRequest, baseline: CanaryBaseline | None) -> ModelOutcome:
        recorded = self._raw_dir / request.call_id / RESULT_FILENAME
        if recorded.is_file():
            return outcome_from_mapping(json.loads(recorded.read_text(encoding="utf-8")))
        failures: list[str] = []
        violations: list[str] = []
        first = self._attempts_on_disk(request) + 1
        for attempt in range(first, first + MODEL_RETRY_COUNT + 1):
            result, attempt_failures, attempt_violations = self._attempt(
                request, baseline, attempt
            )
            failures += attempt_failures
            violations += attempt_violations
            if result is not None:
                return self._record(request, result)
        return self._record(request, MissingResult(tuple(failures), tuple(violations)))

    def _attempts_on_disk(self, request: ModelRequest) -> int:
        directory = self._raw_dir / request.call_id
        if not directory.is_dir():
            return 0
        return sum(1 for path in directory.iterdir() if path.is_dir())

    def _record(self, request: ModelRequest, outcome: ModelOutcome) -> ModelOutcome:
        path = self._raw_dir / request.call_id / RESULT_FILENAME
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(outcome.to_mapping(), indent=2, sort_keys=True), encoding="utf-8")
        return outcome

    def _attempt(
        self, request: ModelRequest, baseline: CanaryBaseline | None, number: int
    ) -> tuple[ModelResult | None, list[str], list[str]]:
        attempt_dir = self._raw_dir / request.call_id / ATTEMPT_DIR_TEMPLATE.format(number=number)
        attempt_dir.mkdir(parents=True, exist_ok=True)
        call_dir = Path(tempfile.mkdtemp(prefix=CALL_DIR_PREFIX, dir=self._call_parent))
        try:
            parsed = self._run_and_store(request, attempt_dir, call_dir)
        finally:
            shutil.rmtree(call_dir, ignore_errors=True)
        violations = parsed.violations + self._baseline_violations(request, parsed, baseline)
        failures = list(parsed.failures)
        self._store_checks(attempt_dir, failures, violations)
        if failures or violations:
            return None, failures, violations
        return self._result(parsed, number, request), [], []

    def _run_and_store(
        self, request: ModelRequest, attempt_dir: Path, call_dir: Path
    ) -> ParsedCall:
        stdin_path = attempt_dir / REQUEST_FILENAME
        stdin_path.write_text(self.stdin_text(request), encoding="utf-8")
        (attempt_dir / SCHEMA_FILENAME).write_text(request.schema_text, encoding="utf-8")
        argv = self.argv(request, call_dir, attempt_dir / SCHEMA_FILENAME)
        (attempt_dir / ARGV_FILENAME).write_text(json.dumps(argv, indent=2), encoding="utf-8")
        process_result = self._process.run(
            argv,
            call_dir,
            stdin_path=stdin_path,
            env=self.environment(),
            deadline_seconds=MODEL_CALL_DEADLINE_SECONDS,
        )
        self._store_streams(attempt_dir, process_result)
        self._store_artifacts(call_dir, attempt_dir)
        parsed = self.parse(process_result, call_dir, request)
        leftover = self.leftover_violation(call_dir)
        if leftover:
            parsed.violations.append(leftover)
        return parsed

    def _store_streams(self, attempt_dir: Path, result: ProcessResult) -> None:
        (attempt_dir / self.stdout_filename).write_text(result.stdout, encoding="utf-8")
        (attempt_dir / STDERR_FILENAME).write_text(result.stderr, encoding="utf-8")

    def _store_artifacts(self, call_dir: Path, attempt_dir: Path) -> None:
        for name in self.artifact_names:
            if (call_dir / name).is_file():
                shutil.copy(call_dir / name, attempt_dir / name)

    @staticmethod
    def _store_checks(attempt_dir: Path, failures: list[str], violations: list[str]) -> None:
        checks = {"failures": failures, "isolation_violations": violations}
        (attempt_dir / CHECKS_FILENAME).write_text(json.dumps(checks, indent=2), encoding="utf-8")

    @staticmethod
    def _baseline_violations(
        request: ModelRequest, parsed: ParsedCall, baseline: CanaryBaseline | None
    ) -> list[str]:
        found = [
            token_violation(parsed.total_input_tokens, baseline, len(request.item)),
            num_turns_violation(parsed.num_turns, baseline),
        ]
        return [v for v in found if v]

    @staticmethod
    def _result(parsed: ParsedCall, attempt: int, request: ModelRequest) -> ModelResult:
        return ModelResult(
            payload=parsed.payload,
            usage=TokenUsage(parsed.total_input_tokens, parsed.output_tokens),
            cost_usd=parsed.cost_usd,
            num_turns=parsed.num_turns,
            attempt=attempt,
            provenance=request.pin.provenance,
        )

    @abstractmethod
    def argv(self, request: ModelRequest, call_dir: Path, schema_path: Path) -> list[str]: ...

    @abstractmethod
    def parse(self, result: ProcessResult, call_dir: Path, request: ModelRequest) -> ParsedCall: ...

    def stdin_text(self, request: ModelRequest) -> str:
        return request.item

    def environment(self) -> Mapping[str, str] | None:
        return None

    def leftover_violation(self, call_dir: Path) -> str | None:
        return None


class ClaudeCli(_CliModel):
    """`claude -p` in a fresh empty directory; the single `modelUsage` key must equal the pin."""

    def argv(self, request: ModelRequest, call_dir: Path, schema_path: Path) -> list[str]:
        return [
            CLAUDE_EXECUTABLE,
            "-p",
            "--model",
            request.pin.model,
            "--effort",
            request.effort.value,
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
            request.schema_text,
        ]

    def environment(self) -> Mapping[str, str]:
        return {k: v for k, v in os.environ.items() if k not in CLAUDE_NESTING_ENV_VARS}

    def leftover_violation(self, call_dir: Path) -> str | None:
        return leftover_files_violation(p.name for p in call_dir.iterdir())

    def parse(self, result: ProcessResult, call_dir: Path, request: ModelRequest) -> ParsedCall:
        if result.returncode != 0:
            return ParsedCall(failures=[f"claude exited {result.returncode}"])
        try:
            data = json.loads(result.stdout)
        except ValueError:
            return ParsedCall(failures=["stdout is not JSON"])
        if not isinstance(data, dict) or data.get("is_error") or "structured_output" not in data:
            return ParsedCall(failures=["error result or no structured_output"])
        usage = data.get("usage", {})
        pin_violation = pinned_model_violation(data.get("modelUsage", {}), request.pin.model)
        return ParsedCall(
            payload=data["structured_output"],
            total_input_tokens=sum(
                usage.get(key, 0)
                for key in ("input_tokens", "cache_creation_input_tokens", "cache_read_input_tokens")
            ),
            output_tokens=usage.get("output_tokens", 0),
            cost_usd=data.get("total_cost_usd"),
            num_turns=data.get("num_turns"),
            violations=[pin_violation] if pin_violation else [],
        )


class CodexCli(_CliModel):
    """`codex exec` in a fresh empty directory; only agent_message and reasoning items allowed."""

    stdout_filename = EVENTS_FILENAME
    artifact_names = (CODEX_OUTPUT_FILENAME,)

    def stdin_text(self, request: ModelRequest) -> str:
        return f"{request.system_prompt}\n\n{request.item}"

    def argv(self, request: ModelRequest, call_dir: Path, schema_path: Path) -> list[str]:
        features = [arg for name in CODEX_DISABLED_FEATURES for arg in ("--disable", name)]
        return [
            CODEX_EXECUTABLE,
            "exec",
            "--ephemeral",
            "--skip-git-repo-check",
            "--ignore-user-config",
            "--ignore-rules",
            "-s",
            "read-only",
            "-C",
            str(call_dir),
            "-m",
            request.pin.model,
            "-c",
            f"model_reasoning_effort={request.effort.value}",
            "-c",
            "web_search=disabled",
            *features,
            "--output-schema",
            str(schema_path),
            "-o",
            CODEX_OUTPUT_FILENAME,
            "--json",
        ]

    def parse(self, result: ProcessResult, call_dir: Path, request: ModelRequest) -> ParsedCall:
        events = _json_lines(result.stdout)
        item_types = [e["item"]["type"] for e in events if isinstance(e.get("item"), dict)]
        violations = codex_item_violations(item_types)
        output = call_dir / CODEX_OUTPUT_FILENAME
        payload, failures = self._payload(result, output)
        usage = _codex_usage(events)
        return ParsedCall(
            payload=payload,
            total_input_tokens=usage.input_tokens,
            output_tokens=usage.output_tokens,
            failures=failures,
            violations=violations,
        )

    @staticmethod
    def _payload(result: ProcessResult, output: Path) -> tuple[Any, list[str]]:
        if result.returncode != 0:
            return None, [f"codex exited {result.returncode}"]
        if not output.is_file():
            return None, ["no output file"]
        try:
            return json.loads(output.read_text(encoding="utf-8")), []
        except ValueError:
            return None, ["output file is not JSON"]


def _json_lines(text: str) -> list[dict[str, Any]]:
    events = []
    for line in text.splitlines():
        try:
            parsed = json.loads(line)
        except ValueError:
            continue
        if isinstance(parsed, dict):
            events.append(parsed)
    return events


def _codex_usage(events: Sequence[Mapping[str, Any]]) -> TokenUsage:
    completed = [e.get("usage", {}) for e in events if e.get("type") == "turn.completed"]
    return TokenUsage(
        input_tokens=sum(u.get("input_tokens", 0) for u in completed),
        output_tokens=sum(u.get("output_tokens", 0) for u in completed),
    )
