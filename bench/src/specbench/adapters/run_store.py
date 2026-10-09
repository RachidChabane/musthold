"""The run directory on disk: raw call records, the call ledger, the manifest, the deviation log and the trunk records."""

from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Final

from specbench.domain.call_ledger import EMPTY_LEDGER, CallLedger
from specbench.domain.canonical import JsonValue, canonical_json, stable_json
from specbench.domain.deviations import (
    DeviationEntry,
    parse_deviations,
    render_deviations,
)
from specbench.domain.jsonread import enum_of, integer, mapping, optional_text, sequence, text
from specbench.domain.manifest import (
    CALL_ARGV,
    CALL_EVENTS,
    CALL_META,
    CALL_STDIN,
    CALL_STDOUT,
    Manifest,
    RunLayout,
    parse_manifest,
)
from specbench.domain.records import TrunkRecord, trunk_from_record
from specbench.domain.reservation import ExecutionHistory
from specbench.domain.sessions import CallRecord, SessionOutcome

ENCODING: Final = "utf-8"
FAILURES_FILE_NAME: Final = "failures.jsonl"


def write_atomically(path: Path, content: str) -> None:
    """Write beside the target and rename over it, so a process killed mid-write leaves the previous file intact."""
    path.parent.mkdir(parents=True, exist_ok=True)
    pending = path.with_name(f".{path.name}.pending")
    pending.write_text(content, encoding=ENCODING)
    os.replace(pending, path)


class FileCallStore:
    """`CallStore`: one directory per call holding argv, stdin, stdout, events and meta."""

    def __init__(self, layout: RunLayout) -> None:
        self._layout = layout

    def exists(self, call_id: str) -> bool:
        return (self._layout.call(call_id) / CALL_META).is_file()

    def write(self, record: CallRecord) -> None:
        directory = self._layout.call(record.call_id)
        directory.mkdir(parents=True, exist_ok=True)
        (directory / CALL_ARGV).write_text(stable_json(list(record.argv)), encoding=ENCODING)
        (directory / CALL_STDIN).write_text(record.stdin, encoding=ENCODING)
        (directory / CALL_STDOUT).write_text(record.stdout, encoding=ENCODING)
        (directory / CALL_EVENTS).write_text("".join(f"{event}\n" for event in record.events), encoding=ENCODING)
        meta = {
            "cli_version": record.cli_version,
            "executions": record.executions,
            "exit_code": record.exit_code,
            "model_reported": record.model_reported,
            "outcome": record.outcome.value,
            "session_id": record.session_id,
        }
        (directory / CALL_META).write_text(canonical_json(meta), encoding=ENCODING)

    def read(self, call_id: str) -> CallRecord:
        directory = self._layout.call(call_id)
        meta = mapping(json.loads((directory / CALL_META).read_text(encoding=ENCODING)), "meta")
        argv = sequence(json.loads((directory / CALL_ARGV).read_text(encoding=ENCODING)), "argv")
        return CallRecord(
            call_id=call_id,
            argv=tuple(text(a, "argv item") for a in argv),
            stdin=(directory / CALL_STDIN).read_text(encoding=ENCODING),
            stdout=(directory / CALL_STDOUT).read_text(encoding=ENCODING),
            events=tuple((directory / CALL_EVENTS).read_text(encoding=ENCODING).splitlines()),
            exit_code=integer(meta.get("exit_code"), "exit_code", -1_000),
            session_id=text(meta.get("session_id"), "session_id"),
            model_reported=optional_text(meta.get("model_reported"), "model_reported"),
            outcome=enum_of(SessionOutcome, meta.get("outcome", SessionOutcome.COMPLETED.value), "outcome"),
            executions=integer(meta.get("executions", 1), "executions", 0),
            cli_version=optional_text(meta.get("cli_version"), "cli_version"),
        )


class FileLedgerStore:
    """`LedgerStore`: the nightly call budget ledger as canonical JSON."""

    def __init__(self, layout: RunLayout) -> None:
        self._path = layout.call_ledger

    def load(self) -> CallLedger:
        if not self._path.is_file():
            return EMPTY_LEDGER
        return CallLedger.from_record(mapping(json.loads(self._path.read_text(encoding=ENCODING)), "ledger"))

    def save(self, ledger: CallLedger) -> None:
        write_atomically(self._path, canonical_json(ledger.to_record()))


class FileExecutionHistoryStore:
    """`ExecutionHistoryStore`: what completed gates spent, in the run directory."""

    def __init__(self, layout: RunLayout) -> None:
        self._path = layout.execution_history

    def load(self) -> ExecutionHistory:
        if not self._path.is_file():
            return ExecutionHistory()
        return ExecutionHistory.from_json(self._path.read_text(encoding=ENCODING))

    def save(self, history: ExecutionHistory) -> None:
        write_atomically(self._path, history.to_json())


class FileEventStream:
    """`EventStream`: the events a session streamed to `calls/<call-id>/events.jsonl` as it ran."""

    def __init__(self, layout: RunLayout) -> None:
        self._layout = layout

    def lines(self, call_id: str) -> tuple[str, ...]:
        path = self._layout.call(call_id) / CALL_EVENTS
        return tuple(path.read_text(encoding=ENCODING).splitlines()) if path.is_file() else ()


class RunStore:
    """Reads and writes the records of one run directory."""

    def __init__(self, layout: RunLayout) -> None:
        self._layout = layout

    def write_manifest(self, manifest: Manifest) -> None:
        self._layout.root.mkdir(parents=True, exist_ok=True)
        self._layout.manifest.write_text(manifest.to_canonical_json(), encoding=ENCODING)

    def read_manifest(self) -> Manifest:
        return parse_manifest(self._layout.manifest.read_text(encoding=ENCODING))

    def read_deviations(self) -> list[DeviationEntry]:
        path = self._layout.deviations
        return parse_deviations(path.read_text(encoding=ENCODING)) if path.is_file() else []

    def append_deviation(self, entry: DeviationEntry) -> None:
        self._layout.root.mkdir(parents=True, exist_ok=True)
        entries = [*self.read_deviations(), entry]
        self._layout.deviations.write_text(render_deviations(entries), encoding=ENCODING)

    def write_trunk(self, record: dict[str, JsonValue], path: Path) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(canonical_json(record), encoding=ENCODING)

    def read_trunks(self) -> list[TrunkRecord]:
        """Every trunk record, ordered by task and harness so that analysis does not depend on directory order."""
        trunks = [
            trunk_from_record(json.loads(path.read_text(encoding=ENCODING)))
            for path in sorted(self._layout.trunks.glob("*/*.json"))
        ]
        return sorted(trunks, key=lambda t: (t.task_id, t.harness.value))

    def write_results(self, results_json: str, report: str) -> None:
        self._layout.results.write_text(results_json, encoding=ENCODING)
        self._layout.report.write_text(report, encoding=ENCODING)


class FileFailureLog:
    """`FailureLog`: one canonical JSON line per failed call, in the run directory."""

    def __init__(self, layout: RunLayout) -> None:
        self._path = layout.root / FAILURES_FILE_NAME

    def append(self, call_id: str, key: str, message: str) -> None:
        self._path.parent.mkdir(parents=True, exist_ok=True)
        with self._path.open("a", encoding=ENCODING) as handle:
            handle.write(canonical_json({"call_id": call_id, "key": key, "message": message}))
