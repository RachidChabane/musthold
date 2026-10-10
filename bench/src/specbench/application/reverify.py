"""`reverify`: settle again, with no model call, a task that construction built and dropped at verification.

The task's plain files are exactly what construction verified, so verifying them again after a fix to how
verification builds or runs trees gives the verdict construction would now give, and settles the task the same way:
sealed when verified, dropped with the new reason otherwise. A task dropped before verification has no plain files to
verify, and a sealed task is final; both are refused.
"""

from __future__ import annotations

from dataclasses import dataclass

from specbench.application.construct import DropStage
from specbench.application.construct_run import Outcome, StatusStore, TaskSettlement, TaskStatus
from specbench.domain.sealing import CURRENT_BUNDLE_VERSION
from specbench.domain.task import Task


class NotReverifiable(Exception):
    """The task has no verification to repeat: it is unknown, sealed, or dropped before verification."""


@dataclass(frozen=True)
class Reverified:
    task: Task
    status: TaskStatus


def reverify(task_id: str, statuses: StatusStore, settlement: TaskSettlement) -> Reverified:
    status = statuses.read(task_id)
    if status is None or status.outcome is not Outcome.DROPPED or status.stage != DropStage.VERIFICATION.value:
        raise NotReverifiable(f"{task_id} was not dropped at verification")
    built = statuses.read_plain(task_id)
    if built is None:
        raise NotReverifiable(f"{task_id} has no plain task files")
    if built.bundle.schema_version < CURRENT_BUNDLE_VERSION:
        raise NotReverifiable(
            f"{task_id} was built at bundle schema version {built.bundle.schema_version}, which sealing refuses: "
            f"reopen it (bench/script/reopen --task-id {task_id}) and construct it again"
        )
    return Reverified(built.task, settlement.settle(built))
