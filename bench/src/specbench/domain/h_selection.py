"""Selection of H from blind pools by execution (ADR 0012): verdicts per candidate, seeded choice, refill demand, record.

The writer never sees an outcome. Construction runs every candidate at merge and at base, `VERIFICATION_RUNS` times
each, on the verifier's own trees and runners, and keeps per criterion the candidates that meet verification's own
rules for H. Everything here is a pure function of the candidates, their outcomes and the seed; violation and planted
outcomes are never an input.
"""

from __future__ import annotations

import json
from collections import Counter
from collections.abc import Iterable, Mapping, Sequence
from dataclasses import dataclass, field
from enum import StrEnum
from typing import Final

from specbench.domain.canonical import JsonValue
from specbench.domain.changed_tests import test_ids
from specbench.domain.h_tests import POOL_ROUND, HFile, HTest, TestKind, python_pruned
from specbench.domain.jsonread import RecordError, mapping, sequence, text
from specbench.domain.languages import Language
from specbench.domain.mining import seeded_key
from specbench.domain.oracle import Outcome, Part, oracle_from_bundle
from specbench.domain.sealing import SealedBundle, SealingError
from specbench.domain.spec import criterion_id
from specbench.domain.verification import stabilise

MAX_KEPT_PER_CRITERION: Final = 3
"""More kept tests per criterion raise the chance that a violation also fails a test of another criterion."""
MIN_KEPT_PER_CRITERION: Final = 2
"""A violation's diagonal never rests on a single test."""
MAX_REFILL_ROUNDS: Final = 2
H_SELECTION_PATH: Final = "construction/h-selection.json"
H_CANDIDATES_PREFIX: Final = "construction/h-candidates/"
H_SELECTION_SCHEMA: Final = "specbench.construction.h-selection/1"
SELECTION_LABEL: Final = "h-selection"
_ONE_TEST: Final = "candidate"


class Verdict(StrEnum):
    KEPT = "kept"
    REJECTED_MERGE = "rejected-merge"
    REJECTED_BASE_PASS = "rejected-base-pass"
    REJECTED_FLAKY = "rejected-flaky"
    REJECTED_FILE = "rejected-file"
    SURPLUS = "surplus"


MERGE_REJECTS: Final = frozenset({Verdict.REJECTED_MERGE, Verdict.REJECTED_FILE})
"""Rejections that say the merge disagrees with a candidate: the selection's exposure to the merge."""


def stable_outcome(runs: Sequence[Outcome]) -> Outcome:
    """The verifier's rule for one test: the common outcome of its runs, FLAKY when they differ, MISSING when none ran."""
    if not runs:
        return Outcome.MISSING
    return stabilise([{_ONE_TEST: outcome} for outcome in runs], [_ONE_TEST])[_ONE_TEST]


@dataclass(frozen=True)
class CandidateRuns:
    """What every run of one candidate reported: at merge, and at base (none when its file was rejected)."""

    merge: tuple[Outcome, ...]
    base: tuple[Outcome, ...] = ()

    @property
    def at_merge(self) -> Outcome:
        return stable_outcome(self.merge)

    @property
    def at_base(self) -> Outcome:
        return stable_outcome(self.base)


@dataclass(frozen=True)
class HExecution:
    """The runs of the candidates of one or more writings, and the candidate files that did not collect or build."""

    runs: Mapping[str, CandidateRuns] = field(default_factory=dict[str, CandidateRuns])
    unbuildable: frozenset[str] = frozenset()

    def merged(self, other: HExecution) -> HExecution:
        return HExecution({**self.runs, **other.runs}, self.unbuildable | other.unbuildable)


def uncollected_files(tests: Sequence[HTest], merge_runs: Mapping[str, Sequence[Outcome]]) -> frozenset[str]:
    """Files none of whose tests any merge run reported: the file failed to collect or import."""
    by_file: dict[str, list[HTest]] = {}
    for test in tests:
        by_file.setdefault(test.path, []).append(test)
    return frozenset(
        path
        for path, own in by_file.items()
        if all(outcome is Outcome.MISSING for test in own for outcome in merge_runs.get(test.test_id, ()))
    )


def rejection(test: HTest, runs: CandidateRuns, unbuildable: frozenset[str], judged_at_base: bool) -> Verdict | None:
    """Why verification would refuse this test, or None when it meets every rule verification applies to H.

    Merge must pass it in every run; base must fail it in every run when its criterion is new or revised; it is
    flaky on neither tree. A file that does not collect or build at merge takes all its tests with it.
    """
    if test.path in unbuildable:
        return Verdict.REJECTED_FILE
    if Outcome.FLAKY in (runs.at_merge, runs.at_base):
        return Verdict.REJECTED_FLAKY
    if runs.at_merge is not Outcome.PASSED:
        return Verdict.REJECTED_MERGE
    if judged_at_base and runs.at_base is Outcome.PASSED:
        return Verdict.REJECTED_BASE_PASS
    return None


def selection_rank(seed: int, task_id: str, test: HTest) -> tuple[int, str]:
    """Property tests first, then the seeded hash of the task and the test id."""
    return (0 if test.kind is TestKind.PROPERTY else 1, seeded_key(seed, f"{SELECTION_LABEL}:{task_id}:{test.test_id}"))


@dataclass(frozen=True)
class HSelection:
    """The verdict of every candidate, and the criteria left with fewer than the minimum of kept tests."""

    tests: tuple[HTest, ...]
    verdicts: Mapping[str, Verdict]
    criteria: tuple[int, ...]

    @property
    def kept(self) -> frozenset[str]:
        return frozenset(t for t, v in self.verdicts.items() if v is Verdict.KEPT)

    def kept_count(self, number: int) -> int:
        return sum(self.verdicts[t.test_id] is Verdict.KEPT for t in self.tests if t.number == number)

    @property
    def deficient(self) -> tuple[int, ...]:
        return tuple(n for n in self.criteria if self.kept_count(n) < MIN_KEPT_PER_CRITERION)

    def verdict_counts(self, number: int) -> Counter[Verdict]:
        return Counter(self.verdicts[t.test_id] for t in self.tests if t.number == number)


def select_h(
    tests: Sequence[HTest],
    execution: HExecution,
    criteria: Sequence[int],
    judged_at_base: frozenset[int],
    seed: int,
    task_id: str,
) -> HSelection:
    """Per criterion, at most `MAX_KEPT_PER_CRITERION` of the candidates that verification would accept, by rank."""
    verdicts: dict[str, Verdict] = {}
    survivors: dict[int, list[HTest]] = {}
    for test in tests:
        runs = execution.runs.get(test.test_id, CandidateRuns(()))
        verdict = rejection(test, runs, execution.unbuildable, test.number in judged_at_base)
        if verdict is None:
            survivors.setdefault(test.number, []).append(test)
        else:
            verdicts[test.test_id] = verdict
    for own in survivors.values():
        ranked = sorted(own, key=lambda t: selection_rank(seed, task_id, t))
        for position, test in enumerate(ranked):
            verdicts[test.test_id] = Verdict.KEPT if position < MAX_KEPT_PER_CRITERION else Verdict.SURPLUS
    ordered = tuple(sorted(tests, key=lambda t: t.test_id))
    return HSelection(ordered, {t.test_id: verdicts[t.test_id] for t in ordered}, tuple(sorted(set(criteria))))


def shortfall(selection: HSelection) -> str:
    """Why the deficient criteria could not be filled: each one's kept count and its rejections by class."""
    parts: list[str] = []
    for number in selection.deficient:
        counts = selection.verdict_counts(number)
        classes = ", ".join(f"{v.value} {counts[v]}" for v in Verdict if v is not Verdict.KEPT and counts[v])
        parts.append(
            f"criterion {number} keeps {selection.kept_count(number)} H tests, {MIN_KEPT_PER_CRITERION} required"
            + (f" ({classes})" if classes else "")
        )
    return "; ".join(parts)


def sealed_h_files(language: Language, files: Sequence[HFile], kept: frozenset[str]) -> tuple[HFile, ...]:
    """The H files sealed for a selection: only kept tests run.

    A Python runner runs whole files, so the other tests are pruned by their `ast` span. A Go runner selects tests by
    name, so a Go file stays whole and only the manifest is restricted. A file with no kept test is dropped.
    """
    sealed: list[HFile] = []
    for file in files:
        if not any(t in kept for t in test_ids(language, file.path, file.content)):
            continue
        if language is Language.PYTHON:
            pruned = python_pruned(file.path, file.content, kept)
            sealed.append(HFile(file.path, pruned if pruned is not None else file.content))
        else:
            sealed.append(file)
    return tuple(sealed)


@dataclass(frozen=True)
class HRound:
    """One writer call's place in the selection: its round, the criteria it was asked for, and its call ids."""

    round: int
    criteria: tuple[int, ...]
    call_ids: tuple[str, ...]


@dataclass(frozen=True)
class HSelectionRecord:
    """Everything the selection did, sealed with the task so every keep and drop is auditable."""

    task_id: str
    seed: int
    rounds: tuple[HRound, ...]
    selection: HSelection
    execution: HExecution

    @property
    def refilled(self) -> frozenset[int]:
        return frozenset(n for r in self.rounds if r.round != POOL_ROUND for n in r.criteria)

    def merge_rejects(self, number: int) -> int:
        counts = self.selection.verdict_counts(number)
        return sum(counts[v] for v in MERGE_REJECTS)

    @property
    def exposed(self) -> frozenset[int]:
        """Criteria whose H the merge shaped: a candidate the merge rejected, or a refill."""
        return self.refilled | frozenset(n for n in self.selection.criteria if self.merge_rejects(n))

    @property
    def divergences(self) -> tuple[str, ...]:
        """Pruned candidates that failed at merge: where the merge and the writer's reading of the spec differ."""
        return tuple(t.test_id for t in self.selection.tests if self.selection.verdicts[t.test_id] in MERGE_REJECTS)

    def to_record(self) -> dict[str, JsonValue]:
        task = self.task_id
        selection = self.selection
        candidates: list[JsonValue] = []
        for test in selection.tests:
            runs = self.execution.runs.get(test.test_id, CandidateRuns(()))
            candidates.append(
                {
                    "base": [o.value for o in runs.base],
                    "criterion": criterion_id(task, test.number),
                    "file": test.path,
                    "id": test.test_id,
                    "kind": test.kind.value,
                    "merge": [o.value for o in runs.merge],
                    "round": test.round,
                    "verdict": selection.verdicts[test.test_id].value,
                }
            )
        criteria: list[JsonValue] = [
            {
                "criterion": criterion_id(task, n),
                "exposed": n in self.exposed,
                "kept": selection.kept_count(n),
                "merge_rejects": self.merge_rejects(n),
                "refilled": n in self.refilled,
            }
            for n in selection.criteria
        ]
        rounds: list[JsonValue] = [
            {"calls": list(r.call_ids), "criteria": [criterion_id(task, n) for n in r.criteria], "round": r.round}
            for r in self.rounds
        ]
        return {
            "candidates": candidates,
            "criteria": criteria,
            "divergences": list(self.divergences),
            "rounds": rounds,
            "schema": H_SELECTION_SCHEMA,
            "seed": self.seed,
            "task_id": task,
        }


@dataclass(frozen=True)
class SealedHSelection:
    """What later steps read back from a sealed selection record: the kept tests and the exposed criteria."""

    kept: frozenset[str]
    exposed: frozenset[str]


def h_selection_from_record(record: object) -> SealedHSelection:
    body = mapping(record, "h selection")
    if body.get("schema") != H_SELECTION_SCHEMA:
        raise RecordError(f"h selection schema must be {H_SELECTION_SCHEMA}")
    kept: set[str] = set()
    for raw in sequence(body.get("candidates"), "candidates"):
        candidate = mapping(raw, "candidate")
        if Verdict(text(candidate.get("verdict"), "verdict")) is Verdict.KEPT:
            kept.add(text(candidate.get("id"), "id"))
    exposed = {
        text(mapping(raw, "criterion").get("criterion"), "criterion")
        for raw in sequence(body.get("criteria"), "criteria")
        if mapping(raw, "criterion").get("exposed") is True
    }
    return SealedHSelection(frozenset(kept), frozenset(exposed))


def read_h_selection(bundle: SealedBundle) -> SealedHSelection:
    if H_SELECTION_PATH not in bundle.files:
        raise SealingError(f"bundle lacks {H_SELECTION_PATH}: its H was not selected by execution; rebuild the task")
    try:
        return h_selection_from_record(json.loads(bundle.files[H_SELECTION_PATH]))
    except (RecordError, ValueError) as error:
        raise SealingError(f"{H_SELECTION_PATH} is malformed: {error}") from error


def require_selected_h(bundle: SealedBundle) -> None:
    """Sealing refuses a bundle whose H tests are not exactly the tests its selection record kept."""
    selected = read_h_selection(bundle).kept
    sealed = frozenset(t.test_id for t in oracle_from_bundle(bundle).tests_of(Part.H))
    if sealed != selected:
        unselected = sorted(sealed - selected)[:3]
        absent = sorted(selected - sealed)[:3]
        raise SealingError(
            f"the oracle's H tests are not the tests selected by execution (unselected {unselected}, absent {absent})"
        )


def exposed_criteria(bundle: SealedBundle) -> frozenset[str]:
    """The criteria whose H the merge shaped, or none for a bundle that carries no selection record."""
    return read_h_selection(bundle).exposed if H_SELECTION_PATH in bundle.files else frozenset()


def candidate_files(rounds: Iterable[Sequence[HFile]]) -> dict[str, bytes]:
    """The unpruned candidate files of every writing, under their own bundle prefix."""
    return {f"{H_CANDIDATES_PREFIX}{f.path}": f.content.encode() for files in rounds for f in files}
