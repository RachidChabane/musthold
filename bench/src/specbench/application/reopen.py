"""`reopen`: return a dropped task to construction, keeping what its earlier attempt left.

Construction skips every task that has a status, so a task dropped under an earlier construction protocol stays
dropped until it is reopened. Reopening sets its status, plain files and reports aside (never deleting them) and
leaves the task unfinished; the next `construct` builds it again. Every model call whose request is unchanged replays
from the archive, so only the calls of the stages that changed are made anew. A sealed task is final and is refused.
"""

from __future__ import annotations

from collections.abc import Sequence
from typing import Protocol

from specbench.application.construct_run import Outcome, TaskStatus


class NotReopenable(Exception):
    """The task has nothing to reopen (no status yet) or is sealed."""


class ReopenableStatuses(Protocol):
    def read(self, task_id: str) -> TaskStatus | None: ...

    def set_aside(self, task_id: str) -> str:
        """Moves the task's status, plain files and reports aside and says where they went."""
        ...


def _require_reopenable(task_id: str, statuses: ReopenableStatuses) -> None:
    status = statuses.read(task_id)
    if status is None:
        raise NotReopenable(f"{task_id} has no status: construction has not finished it, so it is open already")
    if status.outcome is Outcome.SEALED:
        raise NotReopenable(f"{task_id} is sealed, and a sealed task is final")


def reopen(task_id: str, statuses: ReopenableStatuses) -> str:
    """Set a dropped task's last attempt aside; the place it went to."""
    _require_reopenable(task_id, statuses)
    return statuses.set_aside(task_id)


def reopen_all(task_ids: Sequence[str], statuses: ReopenableStatuses) -> dict[str, str]:
    """Reopen every task, or none: each one is checked before any is set aside."""
    for task_id in task_ids:
        _require_reopenable(task_id, statuses)
    return {task_id: statuses.set_aside(task_id) for task_id in dict.fromkeys(task_ids)}
