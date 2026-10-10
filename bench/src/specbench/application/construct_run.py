"""`construct`: build, verify and seal the selected tasks, a language at a time, replacing a dropped task by a reserve.

Nothing here is a model call or a heavy job itself: a task is built by `TaskConstruction` (paced, archived model
calls), then settled by `TaskSettlement`: verified by `TaskVerifier` (one heavy job at a time, under mem-guard in the
composition root) and sealed, or dropped. A pause (closed window, spent budget, quota) or an unsafe machine propagates
as an exception; every call already made is archived and every finished task has a status, so a rerun continues where
this one stopped.
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from enum import Enum
from typing import Protocol

from specbench.application.construct import (
    Constructed,
    ConstructionInputs,
    Dropped,
    DropStage,
    TaskConstruction,
)
from specbench.application.corpus import seal_task
from specbench.application.ports import CorpusRepository, SealStore
from specbench.application.verify_task import TaskVerifier
from specbench.domain.construction import IsolationViolation, ModelCallFailed
from specbench.domain.languages import Language
from specbench.domain.oracle import oracle_from_bundle
from specbench.domain.selection import ProbedTask, Selection
from specbench.domain.task import Task
from specbench.domain.verification import CheckStatus, VerificationReport

MAX_REASON_PROBLEMS = 3


class Outcome(Enum):
    SEALED = "sealed"
    DROPPED = "dropped"


@dataclass(frozen=True)
class TaskStatus:
    task_id: str
    outcome: Outcome
    stage: str | None
    reason: str | None
    bundle_sha256: str | None


class ConstructionSources(Protocol):
    def gather(self, task: ProbedTask) -> ConstructionInputs: ...


class StatusStore(Protocol):
    """Durable per-task outcomes, outside the repository, so a rerun skips what is finished."""

    def read(self, task_id: str) -> TaskStatus | None: ...

    def write(self, status: TaskStatus) -> None: ...

    def write_plain(self, constructed: Constructed) -> None: ...

    def read_plain(self, task_id: str) -> Constructed | None: ...

    def write_verification(self, task_id: str, report: VerificationReport) -> None: ...


@dataclass(frozen=True)
class RunSummary:
    sealed: tuple[str, ...]
    dropped: tuple[tuple[str, str], ...]
    unfinished: tuple[str, ...]

    @property
    def complete(self) -> bool:
        return not self.unfinished


def _failing(report: VerificationReport) -> str:
    problems = [
        f"{check.name.value}: {problem}"
        for check in report.checks
        if check.status is CheckStatus.FAILED
        for problem in check.problems[:MAX_REASON_PROBLEMS]
    ]
    return "; ".join(problems[:MAX_REASON_PROBLEMS]) or "a verification check failed"


def drop(statuses: StatusStore, task_id: str, stage: str, reason: str) -> TaskStatus:
    status = TaskStatus(task_id, Outcome.DROPPED, stage, reason, None)
    statuses.write(status)
    return status


class TaskSettlement:
    """Verifies a constructed task and settles it: sealed into the corpus when verified, dropped with its reason otherwise."""

    def __init__(
        self,
        verifier_for: Callable[[Task], TaskVerifier],
        corpus: CorpusRepository,
        seals: SealStore,
        statuses: StatusStore,
        recoverable: tuple[type[Exception], ...] = (),
    ) -> None:
        self._verifier_for = verifier_for
        self._corpus = corpus
        self._seals = seals
        self._statuses = statuses
        self._recoverable = recoverable

    def settle(self, built: Constructed) -> TaskStatus:
        task_id = built.task.task_id
        try:
            report = self._verifier_for(built.task).verify(built.task, oracle_from_bundle(built.bundle), built.bundle)
        except self._recoverable as error:
            return drop(self._statuses, task_id, DropStage.VERIFICATION.value, f"verification aborted: {error}")
        self._statuses.write_verification(task_id, report)
        if not report.verified:
            return drop(self._statuses, task_id, DropStage.VERIFICATION.value, _failing(report))
        entry = seal_task(self._corpus, self._seals, built.task.to_canonical_json(), built.bundle)
        status = TaskStatus(task_id, Outcome.SEALED, None, None, entry.bundle_sha256)
        self._statuses.write(status)
        return status


class ConstructionRun:
    def __init__(
        self,
        sources: ConstructionSources,
        construction: TaskConstruction,
        settlement: TaskSettlement,
        statuses: StatusStore,
        checkpoint: Callable[[], None],
    ) -> None:
        self._sources = sources
        self._construction = construction
        self._settlement = settlement
        self._statuses = statuses
        self._checkpoint = checkpoint

    def run(self, selection: Selection, tasks_per_language: int) -> RunSummary:
        sealed: list[str] = []
        dropped: list[tuple[str, str]] = []
        unfinished: list[str] = []
        for language in Language:
            queue = [t for t in (*selection.tasks, *selection.reserves) if t.candidate.language is language]
            sealed_here = 0
            for task in queue:
                if sealed_here >= tasks_per_language:
                    break
                status = self._statuses.read(task.task_id) or self._process(task)
                if status is None:
                    unfinished.append(task.task_id)
                    break
                if status.outcome is Outcome.SEALED:
                    sealed.append(task.task_id)
                    sealed_here += 1
                else:
                    dropped.append((task.task_id, f"{status.stage}: {status.reason}"))
            if sealed_here < tasks_per_language:
                unfinished.append(f"{language.value}: {sealed_here} of {tasks_per_language} sealed")
        return RunSummary(tuple(sealed), tuple(dropped), tuple(unfinished))

    def _process(self, task: ProbedTask) -> TaskStatus | None:
        """Build, verify and seal one task; None when a model call failed and the task stays pending."""
        self._checkpoint()
        try:
            built = self._construction.build(self._sources.gather(task))
        except IsolationViolation:
            raise
        except ModelCallFailed:
            return None
        if isinstance(built, Dropped):
            return drop(self._statuses, task.task_id, built.stage.value, built.reason)
        self._statuses.write_plain(built)
        self._checkpoint()
        return self._settlement.settle(built)
