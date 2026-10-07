from __future__ import annotations

import subprocess
from typing import cast

import pytest

from specbench.application.construct import Constructed, ConstructionInputs, Dropped, DropStage, TaskConstruction
from specbench.application.construct_run import ConstructionRun, Outcome, TaskStatus
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

    def read(self, task_id: str) -> TaskStatus | None:
        return self.statuses.get(task_id)

    def write(self, status: TaskStatus) -> None:
        self.statuses[status.task_id] = status

    def write_plain(self, constructed: Constructed) -> None:
        self.plain.append(constructed.task.task_id)

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
        Sources(),
        cast(TaskConstruction, Builds(outcome)),
        lambda _task: cast(TaskVerifier, Aborting(verifier)),
        cast("object", None),  # type: ignore[arg-type]
        cast("object", None),  # type: ignore[arg-type]
        statuses,
        lambda: None,
        (subprocess.TimeoutExpired,),
    )


def selection() -> Selection:
    return Selection((probed(candidate("acme/py0")),), (), ())


def constructed() -> Constructed:
    return Constructed(make_task(), SealedBundle(make_bundle().files))


def test_a_verification_that_times_out_drops_the_task_and_is_recorded() -> None:
    statuses = Statuses()
    summary = run_with(constructed(), subprocess.TimeoutExpired("pytest", 900), statuses).run(selection(), 1)
    ((task_id, reason),) = summary.dropped
    assert reason.startswith(f"{DropStage.VERIFICATION.value}: verification aborted")
    assert statuses.statuses[task_id].outcome is Outcome.DROPPED and statuses.plain == ["ACME-PARKFEE-12"]


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
