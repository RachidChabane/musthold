"""The attempt record of PREREG 10: what `attempt.json` holds, and how an interrupted attempt resumes from it."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass, field
from typing import Final, cast

from demos1.domain.jsonvalue import (
    JsonObject,
    JsonValue,
    as_object,
    boolean,
    children,
    integer,
    optional_boolean,
    optional_child,
    optional_integer,
    optional_text,
    text,
)
from demos1.domain.outcomes import CommandEnd
from demos1.domain.scan_output import FindingRoute, StatementFacts

RECORD_VERSION: Final = 1


def _float(document: JsonObject, name: str) -> float | None:
    value = document.get(name)
    if isinstance(value, bool) or not isinstance(value, int | float):
        return None
    return float(value)


@dataclass
class RunRecord:
    """One run of a container command: one container name."""

    name: str
    command: JsonObject
    network: bool
    started_at: str
    ended_at: str
    host_exit: int
    stdout: str
    stderr: str
    log: str
    client_stderr: str
    wall_seconds: float | None = None
    memory_peak_bytes: int | None = None
    timed_out: bool | None = None
    exit_code: int | None = None
    oom_killed: bool | None = None
    interrupted: bool = False
    killed_at_cap: bool = False
    stopped_by_operator: bool = False

    def to_json(self) -> JsonObject:
        return {
            "name": self.name,
            "command": self.command,
            "network": self.network,
            "started_at": self.started_at,
            "ended_at": self.ended_at,
            "host_exit": self.host_exit,
            "stdout": self.stdout,
            "stderr": self.stderr,
            "log": self.log,
            "client_stderr": self.client_stderr,
            "wall_seconds": self.wall_seconds,
            "memory_peak_bytes": self.memory_peak_bytes,
            "timed_out": self.timed_out,
            "exit_code": self.exit_code,
            "oom_killed": self.oom_killed,
            "interrupted": self.interrupted,
            "killed_at_cap": self.killed_at_cap,
            "stopped_by_operator": self.stopped_by_operator,
        }

    @classmethod
    def from_json(cls, document: JsonObject) -> RunRecord:
        return cls(
            name=text(document, "name"),
            command=as_object(document.get("command") or {}),
            network=boolean(document, "network"),
            started_at=text(document, "started_at"),
            ended_at=text(document, "ended_at"),
            host_exit=integer(document, "host_exit"),
            stdout=text(document, "stdout"),
            stderr=text(document, "stderr"),
            log=text(document, "log"),
            client_stderr=text(document, "client_stderr"),
            wall_seconds=_float(document, "wall_seconds"),
            memory_peak_bytes=optional_integer(document, "memory_peak_bytes"),
            timed_out=optional_boolean(document, "timed_out"),
            exit_code=optional_integer(document, "exit_code"),
            oom_killed=optional_boolean(document, "oom_killed"),
            interrupted=document.get("interrupted") is True,
            killed_at_cap=document.get("killed_at_cap") is True,
            stopped_by_operator=document.get("stopped_by_operator") is True,
        )


@dataclass
class CommandRecord:
    """Every run of one container command, and how its last run ended once it is settled."""

    runs: list[RunRecord] = field(default_factory=list[RunRecord])
    settled: bool = False
    interrupted_out: bool = False

    def next_run(self) -> int:
        return len(self.runs) + 1

    def interruptions(self) -> int:
        return sum(1 for run in self.runs if run.interrupted)

    def final(self) -> RunRecord | None:
        return self.runs[-1] if self.runs else None

    def end(self) -> CommandEnd:
        final = self.final()
        if final is None:
            return CommandEnd(
                None, oom_killed=False, timed_out=False, killed_at_cap=False, interrupted_out=self.interrupted_out
            )
        return CommandEnd(
            exit_code=final.exit_code,
            oom_killed=final.oom_killed is True,
            timed_out=final.timed_out is True,
            killed_at_cap=final.killed_at_cap,
            interrupted_out=self.interrupted_out,
        )

    def to_json(self) -> JsonObject:
        return {
            "runs": [run.to_json() for run in self.runs],
            "settled": self.settled,
            "interrupted_out": self.interrupted_out,
        }

    @classmethod
    def from_json(cls, document: JsonObject) -> CommandRecord:
        return cls(
            runs=[RunRecord.from_json(run) for run in children(document, "runs")],
            settled=document.get("settled") is True,
            interrupted_out=document.get("interrupted_out") is True,
        )


@dataclass
class StageRecord:
    """One setup stage: its container command (if it has one), its details, and its failing outcome if any."""

    stage: str
    command: CommandRecord = field(default_factory=CommandRecord)
    done: bool = False
    outcome: str | None = None
    details: dict[str, JsonValue] = field(default_factory=dict[str, JsonValue])

    def to_json(self) -> JsonObject:
        return {
            "stage": self.stage,
            "command": self.command.to_json(),
            "done": self.done,
            "outcome": self.outcome,
            "details": self.details,
        }

    @classmethod
    def from_json(cls, document: JsonObject) -> StageRecord:
        return cls(
            stage=text(document, "stage"),
            command=CommandRecord.from_json(as_object(document.get("command") or {"runs": []})),
            done=document.get("done") is True,
            outcome=optional_text(document, "outcome"),
            details=dict(as_object(document.get("details") or {})),
        )


@dataclass
class FindingRecord:
    """One test that verifies nothing, as one scan reported it, with its replay."""

    rank: int
    test: str
    sentence: str | None
    site_file: str | None
    site_line: int | None
    route: FindingRoute | None
    folder: str
    replay: CommandRecord = field(default_factory=CommandRecord)
    replay_result: str | None = None
    pull_request_status: str | None = None
    """For a B pick: added, modified or renamed when the test's file is a changed test path; else None."""

    def to_json(self) -> JsonObject:
        return {
            "rank": self.rank,
            "test": self.test,
            "sentence": self.sentence,
            "site": {"file": self.site_file, "line": self.site_line},
            "route": self.route.to_json() if self.route is not None else None,
            "folder": self.folder,
            "replay": self.replay.to_json(),
            "replay_result": self.replay_result,
            "pull_request_status": self.pull_request_status,
        }

    @classmethod
    def from_json(cls, document: JsonObject) -> FindingRecord:
        site = as_object(document.get("site") or {})
        route = optional_child(document, "route")
        return cls(
            rank=integer(document, "rank"),
            test=text(document, "test"),
            sentence=optional_text(document, "sentence"),
            site_file=optional_text(site, "file"),
            site_line=optional_integer(site, "line"),
            route=FindingRoute.from_json(route) if route is not None else None,
            folder=text(document, "folder"),
            replay=CommandRecord.from_json(as_object(document.get("replay") or {"runs": []})),
            replay_result=optional_text(document, "replay_result"),
            pull_request_status=optional_text(document, "pull_request_status"),
        )


@dataclass
class ScanRecord:
    scan: str
    command: CommandRecord = field(default_factory=CommandRecord)
    outcome: str | None = None
    stderr_tail: str | None = None
    moved: str | None = None
    scan_json_sha256: str | None = None
    statement: StatementFacts | None = None
    headline_text: str | None = None
    headline: JsonObject | None = None
    findings: list[FindingRecord] = field(default_factory=list[FindingRecord])
    findings_listed: bool = False
    done: bool = False
    measures: dict[str, JsonValue] = field(default_factory=dict[str, JsonValue])
    """The size of the attempt directory after the scan's last container command."""

    def to_json(self) -> JsonObject:
        return {
            "scan": self.scan,
            "measures": self.measures,
            "command": self.command.to_json(),
            "outcome": self.outcome,
            "stderr_tail": self.stderr_tail,
            "moved": self.moved,
            "scan_json_sha256": self.scan_json_sha256,
            "from_scan_json": self.statement.to_json() if self.statement is not None else None,
            "headline_text": self.headline_text,
            "from_headline": self.headline,
            "findings": [finding.to_json() for finding in self.findings],
            "findings_listed": self.findings_listed,
            "done": self.done,
        }

    @classmethod
    def from_json(cls, document: JsonObject) -> ScanRecord:
        statement = optional_child(document, "from_scan_json")
        return cls(
            scan=text(document, "scan"),
            command=CommandRecord.from_json(as_object(document.get("command") or {"runs": []})),
            outcome=optional_text(document, "outcome"),
            stderr_tail=optional_text(document, "stderr_tail"),
            moved=optional_text(document, "moved"),
            scan_json_sha256=optional_text(document, "scan_json_sha256"),
            statement=StatementFacts.from_json(statement) if statement is not None else None,
            headline_text=optional_text(document, "headline_text"),
            headline=optional_child(document, "from_headline"),
            findings=[FindingRecord.from_json(finding) for finding in children(document, "findings")],
            findings_listed=document.get("findings_listed") is True,
            done=document.get("done") is True,
            measures=dict(as_object(document.get("measures") or {})),
        )


@dataclass
class AttemptRecord:
    attempt: str
    list_name: str
    rank: int
    identity: JsonObject
    environment: JsonObject
    stages: list[StageRecord] = field(default_factory=list[StageRecord])
    outcome: str | None = None
    outcome_log: str | None = None
    scans: list[ScanRecord] = field(default_factory=list[ScanRecord])
    cleanup: JsonObject | None = None
    stopped: str | None = None
    """Set when the disk cap stopped the attempt: the outcome and the stage or scan it stopped at."""
    notes: list[str] = field(default_factory=list[str])

    def stage(self, name: str) -> StageRecord:
        for stage in self.stages:
            if stage.stage == name:
                return stage
        record = StageRecord(name)
        self.stages.append(record)
        return record

    def scan(self, name: str) -> ScanRecord:
        for scan in self.scans:
            if scan.scan == name:
                return scan
        record = ScanRecord(name)
        self.scans.append(record)
        return record

    def finished(self) -> bool:
        return self.outcome is not None and self.cleanup is not None

    def runs(self) -> list[RunRecord]:
        commands = [stage.command for stage in self.stages]
        for scan in self.scans:
            commands.append(scan.command)
            commands += [finding.replay for finding in scan.findings]
        return [run for command in commands for run in command.runs]

    def tmpdirs(self) -> list[str]:
        """The TMPDIR of every container command the attempt ran."""
        found: list[str] = []
        for run in self.runs():
            environment = run.command.get("environment")
            if isinstance(environment, Mapping):
                tmpdir = cast(Mapping[str, JsonValue], environment).get("TMPDIR")
                if isinstance(tmpdir, str) and tmpdir not in found:
                    found.append(tmpdir)
        return found

    def stages_reached(self) -> list[str]:
        return [stage.stage for stage in self.stages if stage.command.runs or stage.done]

    def to_json(self) -> JsonObject:
        return {
            "record_version": RECORD_VERSION,
            "attempt": self.attempt,
            "list": self.list_name,
            "rank": self.rank,
            "identity": self.identity,
            "environment": self.environment,
            "stages_reached": list(self.stages_reached()),
            "stages": [stage.to_json() for stage in self.stages],
            "outcome": self.outcome,
            "outcome_log": self.outcome_log,
            "scans": [scan.to_json() for scan in self.scans],
            "cleanup": self.cleanup,
            "stopped": self.stopped,
            "notes": list(self.notes),
        }

    @classmethod
    def from_json(cls, document: Mapping[str, JsonValue]) -> AttemptRecord:
        notes = document.get("notes")
        return cls(
            attempt=text(document, "attempt"),
            list_name=text(document, "list"),
            rank=integer(document, "rank"),
            identity=as_object(document.get("identity") or {}),
            environment=as_object(document.get("environment") or {}),
            stages=[StageRecord.from_json(stage) for stage in children(document, "stages")],
            outcome=optional_text(document, "outcome"),
            outcome_log=optional_text(document, "outcome_log"),
            scans=[ScanRecord.from_json(scan) for scan in children(document, "scans")],
            cleanup=optional_child(document, "cleanup"),
            stopped=optional_text(document, "stopped"),
            notes=[str(note) for note in notes] if isinstance(notes, list) else [],
        )
