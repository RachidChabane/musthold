"""Case records of a run (`cases/<case-id>.json`): one blocked stop and pointers to the archived files that show it.

A case names its evidence by path, relative to the evidence root of the run; it carries no figure and no
verdict of its own. Whoever renders a case reads the verdicts, the criterion, the tests and the mutant from
the files it points to, and digests every one of them.
"""

from __future__ import annotations

import posixpath
import re
from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum
from typing import Final

from specbench.domain.arms import Arm, Harness
from specbench.domain.cheat import CheatInstruction
from specbench.domain.jsonread import RecordError, boolean, enum_of, integer, mapping, sequence, text
from specbench.domain.languages import Language

CASE_SCHEMA: Final = "specbench.case/1"
CASE_ID_PATTERN: Final = re.compile(r"^[a-z0-9][a-z0-9-]{2,63}$")
CRITERION_PATTERN: Final = re.compile(r"^\S+~[0-9]+$")
PARENT_SEGMENT: Final = ".."
CASE_ARMS: Final = frozenset({Arm.TR, Arm.AL, Arm.DP, Arm.CO, Arm.ALL, Arm.PL, Arm.ALL_NATIVE, Arm.CHEAT})
"""Arms that gate, and so can block a stop; N and T never do."""


class MutantOutcome(Enum):
    KILLED = "killed"
    SURVIVED = "survived"


@dataclass(frozen=True)
class MutantResult:
    test: str
    outcome: MutantOutcome


@dataclass(frozen=True)
class MutantPointer:
    """The mutant the gate reported: the file before and after it, the host answer that proposed it and its witness."""

    file: str
    before: str
    after: str
    answer: str
    index: int
    witness: str | None
    results: tuple[MutantResult, ...]


@dataclass(frozen=True)
class RepairPointer:
    report: str
    diff: str | None


@dataclass(frozen=True)
class EvidencePointer:
    label: str
    path: str


@dataclass(frozen=True)
class CaseRecord:
    case_id: str
    headline: bool
    task_id: str
    repository: str
    language: Language
    harness: Harness
    arm: Arm
    cheat_instruction: CheatInstruction | None
    criterion: str
    claim_events: str | None
    gate_report: str
    mutant: MutantPointer | None
    repair: RepairPointer | None
    evidence: tuple[EvidencePointer, ...]

    @property
    def paths(self) -> tuple[str, ...]:
        """Every evidence path the case names, each once, in the order a reader meets them."""
        named: list[str] = []
        if self.claim_events is not None:
            named.append(self.claim_events)
        named.append(self.gate_report)
        if self.mutant is not None:
            named += [self.mutant.before, self.mutant.after, self.mutant.answer]
            if self.mutant.witness is not None:
                named.append(self.mutant.witness)
        if self.repair is not None:
            named.append(self.repair.report)
            if self.repair.diff is not None:
                named.append(self.repair.diff)
        named += [item.path for item in self.evidence]
        return tuple(dict.fromkeys(named))


def evidence_path(value: object, where: str) -> str:
    """A relative path that stays under the evidence root: never absolute, never through `..`."""
    path = text(value, where)
    if not path or path.startswith("/") or PARENT_SEGMENT in path.split("/") or posixpath.normpath(path) != path:
        raise RecordError(f"{where} must be a normalised path relative to the evidence root")
    return path


def _optional_path(value: object, where: str) -> str | None:
    return None if value is None else evidence_path(value, where)


def _mutant(value: object) -> MutantPointer | None:
    if value is None:
        return None
    body = mapping(value, "mutant")
    return MutantPointer(
        file=text(body.get("file"), "mutant.file"),
        before=evidence_path(body.get("before"), "mutant.before"),
        after=evidence_path(body.get("after"), "mutant.after"),
        answer=evidence_path(body.get("answer"), "mutant.answer"),
        index=integer(body.get("index"), "mutant.index"),
        witness=_optional_path(body.get("witness"), "mutant.witness"),
        results=tuple(
            MutantResult(
                text(row.get("test"), "mutant.results.test"),
                enum_of(MutantOutcome, row.get("outcome"), "mutant.results.outcome"),
            )
            for row in (mapping(item, "mutant result") for item in sequence(body.get("results"), "mutant.results"))
        ),
    )


def _repair(value: object) -> RepairPointer | None:
    if value is None:
        return None
    body = mapping(value, "repair")
    return RepairPointer(
        evidence_path(body.get("report"), "repair.report"), _optional_path(body.get("diff"), "repair.diff")
    )


def _claim(value: object) -> str | None:
    if value is None:
        return None
    return evidence_path(mapping(value, "claim").get("events"), "claim.events")


def _cheat_instruction(value: object) -> CheatInstruction | None:
    return None if value is None else enum_of(CheatInstruction, value, "cheat_instruction")


def case_from_record(record: object) -> CaseRecord:
    body: Mapping[str, object] = mapping(record, "case")
    if body.get("schema") != CASE_SCHEMA:
        raise RecordError(f"schema must be {CASE_SCHEMA}")
    arm = enum_of(Arm, body.get("arm"), "arm")
    if arm not in CASE_ARMS:
        raise RecordError(f"arm {arm.value} never blocks a stop, so it has no case")
    instruction = _cheat_instruction(body.get("cheat_instruction"))
    if (instruction is not None) != (arm is Arm.CHEAT):
        raise RecordError("a cheat instruction is named exactly when the arm is the cheat arm")
    return CaseRecord(
        case_id=text(body.get("case_id"), "case_id", CASE_ID_PATTERN),
        headline=boolean(body.get("headline"), "headline"),
        task_id=text(body.get("task_id"), "task_id"),
        repository=text(body.get("repository"), "repository"),
        language=enum_of(Language, body.get("language"), "language"),
        harness=enum_of(Harness, body.get("harness"), "harness"),
        arm=arm,
        cheat_instruction=instruction,
        criterion=text(body.get("criterion"), "criterion", CRITERION_PATTERN),
        claim_events=_claim(body.get("claim")),
        gate_report=evidence_path(mapping(body.get("gate"), "gate").get("report"), "gate.report"),
        mutant=_mutant(body.get("mutant")),
        repair=_repair(body.get("repair")),
        evidence=tuple(
            EvidencePointer(text(row.get("label"), "evidence.label"), evidence_path(row.get("path"), "evidence.path"))
            for row in (mapping(item, "evidence item") for item in sequence(body.get("evidence"), "evidence"))
        ),
    )
