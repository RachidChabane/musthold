from __future__ import annotations

import subprocess
from typing import cast

import pytest

from specbench.application.construct import Constructed, ConstructionInputs, Dropped, DropStage, TaskConstruction
from specbench.application.construct_run import ConstructionRun, Outcome, TaskSettlement, TaskStatus
from specbench.application.reverify import NotReverifiable, reverify
from specbench.application.verify_task import TaskVerifier
from specbench.domain.construction import ModelCallFailed
from specbench.domain.pacing import PauseReason, RunPaused
from specbench.domain.sealing import SealedBundle
from specbench.domain.selection import ProbedTask, Selection
from specbench.domain.verification import VerificationReport
from support.builders import make_bundle, make_task
from support.selections import candidate, probed


class Statuses:
    def __init__(self) -> None:
        self.statuses: dict[str, TaskStatus] = {}
        self.plain: list[str] = []
        self.built: dict[str, Constructed] = {}

    def read(self, task_id: str) -> TaskStatus | None:
        return self.statuses.get(task_id)

    def write(self, status: TaskStatus) -> None:
        self.statuses[status.task_id] = status

    def write_plain(self, constructed: Constructed) -> None:
        self.plain.append(constructed.task.task_id)
        self.built[constructed.task.task_id] = constructed

    def read_plain(self, task_id: str) -> Constructed | None:
        return self.built.get(task_id)

    def write_verification(self, task_id: str, report: VerificationReport) -> None:
        raise AssertionError("an aborted verification has no report")


class Sources:
    def gather(self, task: ProbedTask) -> ConstructionInputs:
        return cast(ConstructionInputs, task)


class Builds:
    def __init__(self, outcome: Constructed | Dropped | Exception) -> None:
        self._outcome = outcome

    def build(self, inputs: ConstructionInputs) -> Constructed | Dropped:
        if isinstance(self._outcome, Exception):
            raise self._outcome
        return self._outcome


class Aborting:
    def __init__(self, error: Exception) -> None:
        self._error = error

    def verify(self, *_: object) -> VerificationReport:
        raise self._error


def run_with(outcome: Constructed | Dropped | Exception, verifier: Exception, statuses: Statuses) -> ConstructionRun:
    return ConstructionRun(
        Sources(), cast(TaskConstruction, Builds(outcome)), settlement(verifier, statuses), statuses, lambda: None
    )


def settlement(verifier: Exception, statuses: Statuses) -> TaskSettlement:
    return TaskSettlement(
        lambda _task: cast(TaskVerifier, Aborting(verifier)),
        cast("object", None),  # type: ignore[arg-type]
        cast("object", None),  # type: ignore[arg-type]
        statuses,
        (subprocess.TimeoutExpired,),
    )


def selection() -> Selection:
    return Selection((probed(candidate("acme/py0")),), (), ())


SELECTED_ID = "ACME-PY0-1"


def constructed() -> Constructed:
    """The task construction built for the selected candidate."""
    return Constructed(make_task(task_id=SELECTED_ID), SealedBundle(make_bundle().files))


def test_a_verification_that_times_out_drops_the_task_and_is_recorded() -> None:
    statuses = Statuses()
    summary = run_with(constructed(), subprocess.TimeoutExpired("pytest", 900), statuses).run(selection(), 1)
    ((task_id, reason),) = summary.dropped
    assert reason.startswith(f"{DropStage.VERIFICATION.value}: verification aborted")
    assert statuses.statuses[task_id].outcome is Outcome.DROPPED and statuses.plain == [SELECTED_ID]


def test_a_pause_in_verification_is_not_a_drop() -> None:
    statuses = Statuses()
    with pytest.raises(RunPaused) as paused:
        run_with(constructed(), RunPaused(PauseReason.WINDOW_CLOSED), statuses).run(selection(), 1)
    assert paused.value.reason is PauseReason.WINDOW_CLOSED and statuses.statuses == {}


def test_a_failed_model_call_leaves_the_task_pending_and_stops_the_language() -> None:
    statuses = Statuses()
    summary = run_with(ModelCallFailed("exited 2"), RuntimeError("unused"), statuses).run(selection(), 1)
    assert summary.sealed == () and summary.dropped == () and statuses.statuses == {}
    assert summary.unfinished[0] == "ACME-PY0-1"


def test_reverify_settles_a_task_dropped_at_verification_from_its_plain_files() -> None:
    statuses = Statuses()
    built = constructed()
    statuses.write_plain(built)
    stale = "merge-passes-all: tests/test_h.py::test_h1 (H) is missing at merge"
    statuses.write(TaskStatus(built.task.task_id, Outcome.DROPPED, DropStage.VERIFICATION.value, stale, None))
    settled = reverify(built.task.task_id, statuses, settlement(subprocess.TimeoutExpired("go", 900), statuses))
    assert settled.task == built.task
    assert settled.status.reason is not None and settled.status.reason.startswith("verification aborted")
    assert statuses.statuses[built.task.task_id] == settled.status


@pytest.mark.parametrize(
    "status",
    [
        None,
        TaskStatus(SELECTED_ID, Outcome.SEALED, None, None, "ab" * 32),
        TaskStatus(SELECTED_ID, Outcome.DROPPED, DropStage.SPEC.value, "spec-draft: invalid", None),
    ],
)
def test_reverify_refuses_a_task_that_was_not_dropped_at_verification(status: TaskStatus | None) -> None:
    statuses = Statuses()
    statuses.write_plain(constructed())
    if status is not None:
        statuses.write(status)
    with pytest.raises(NotReverifiable):
        reverify(SELECTED_ID, statuses, settlement(RuntimeError("unused"), statuses))


def test_reverify_refuses_a_task_without_plain_files() -> None:
    statuses = Statuses()
    statuses.write(TaskStatus(SELECTED_ID, Outcome.DROPPED, DropStage.VERIFICATION.value, "flaky", None))
    with pytest.raises(NotReverifiable):
        reverify(SELECTED_ID, statuses, settlement(RuntimeError("unused"), statuses))
