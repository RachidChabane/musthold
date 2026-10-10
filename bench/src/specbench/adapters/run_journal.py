"""A verification's per-run record on disk: what each run of each step reported, and how its runner's processes ended.

`TranscriptRecorder` decorates the `ProcessRunner` an oracle runner uses and keeps every invocation; `FileRunJournal`
takes the invocations of each run when the verifier records the run's outcomes, and rewrites the journal file after
every run, so a verification that stops midway still leaves what it saw. The file is bounded and deterministic: only
the tests that did not pass are listed, process output is kept as a short tail and only for the processes that exited
non-zero in runs where a test did not pass, and the materialized tree and other scratch paths are replaced by
placeholders.
"""

from __future__ import annotations

import os
import re
import tempfile
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.application.ports import ProcessResult, ProcessRunner
from specbench.domain.canonical import JsonValue, stable_json
from specbench.domain.oracle import Outcome

TREE_PLACEHOLDER: Final = "<tree>"
SCRATCH_PLACEHOLDER: Final = "<scratch>"
TAIL_LINES: Final = 30
LINE_LENGTH: Final = 300
LISTED_OUTCOMES: Final = 200
ENCODING: Final = "utf-8"


@dataclass(frozen=True)
class Invocation:
    argv: tuple[str, ...]
    cwd: Path | None
    result: ProcessResult


class TranscriptRecorder:
    """A `ProcessRunner` decorator that keeps each invocation until it is drained."""

    def __init__(self, inner: ProcessRunner) -> None:
        self._inner = inner
        self._pending: list[Invocation] = []

    def run(self, argv: Sequence[str], cwd: Path | None = None, env: Mapping[str, str] | None = None) -> ProcessResult:
        result = self._inner.run(argv, cwd, env)
        self._pending.append(Invocation(tuple(argv), cwd, result))
        return result

    def drain(self) -> list[Invocation]:
        drained, self._pending = self._pending, []
        return drained


def _scratch_pattern() -> re.Pattern[str]:
    roots = sorted({tempfile.gettempdir(), os.path.realpath(tempfile.gettempdir())}, key=len, reverse=True)
    return re.compile("(?:" + "|".join(re.escape(root) for root in roots) + r")/[^/\s\"']+")


class _Scrubber:
    """Replaces the run's tree, then any other scratch directory, by placeholders."""

    def __init__(self, tree: Path | None) -> None:
        self._trees = sorted({str(tree), os.path.realpath(tree)}, key=len, reverse=True) if tree is not None else []
        self._scratch = _scratch_pattern()

    def __call__(self, text: str) -> str:
        for tree in self._trees:
            text = text.replace(tree, TREE_PLACEHOLDER)
        return self._scratch.sub(SCRATCH_PLACEHOLDER, text)


def _tail(text: str, scrub: _Scrubber) -> list[JsonValue]:
    lines = scrub(text).splitlines()[-TAIL_LINES:]
    return [line[:LINE_LENGTH] for line in lines]


def _invocation_record(invocation: Invocation, with_output: bool) -> dict[str, JsonValue]:
    scrub = _Scrubber(invocation.cwd)
    record: dict[str, JsonValue] = {
        "argv": [scrub(argument) for argument in invocation.argv],
        "returncode": invocation.result.returncode,
    }
    if with_output:
        record["stdout_tail"] = _tail(invocation.result.stdout, scrub)
        record["stderr_tail"] = _tail(invocation.result.stderr, scrub)
    return record


def run_record(step: str, run: int, outcomes: Mapping[str, Outcome], invocations: Sequence[Invocation]) -> JsonValue:
    """One run of one step: its tests that did not pass (at most `LISTED_OUTCOMES`) and its runner's processes."""
    not_passed = sorted((test_id, o.value) for test_id, o in outcomes.items() if o is not Outcome.PASSED)
    listed: dict[str, JsonValue] = dict(not_passed[:LISTED_OUTCOMES])
    return {
        "invocations": [
            _invocation_record(i, with_output=bool(not_passed) and i.result.returncode != 0) for i in invocations
        ],
        "not_passed": listed,
        "not_passed_total": len(not_passed),
        "run": run,
        "step": step,
        "tests": len(outcomes),
    }


class FileRunJournal:
    """`RunJournal` written to one JSON file, rewritten whole after each run; a new journal starts the file afresh."""

    def __init__(self, path: Path, transcripts: TranscriptRecorder) -> None:
        self._path = path
        self._transcripts = transcripts
        self._runs: list[JsonValue] = []

    def record(self, step: str, run: int, outcomes: Mapping[str, Outcome]) -> None:
        self._runs.append(run_record(step, run, outcomes, self._transcripts.drain()))
        self._path.parent.mkdir(parents=True, exist_ok=True)
        self._path.write_text(stable_json({"runs": self._runs}), encoding=ENCODING)
