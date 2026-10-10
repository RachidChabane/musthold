"""`reopen`: return a dropped task to construction, keeping what its earlier attempt left.

Construction skips every task that has a status, so a task dropped under an earlier construction protocol stays
dropped until it is reopened. Reopening sets its status, plain files and reports aside (never deleting them) and
leaves the task unfinished; the next `construct` builds it again. Every model call whose request is unchanged replays
from the archive, so only the calls of the stages that changed are made anew. A sealed task is final and is refused.
"""

from __future__ import annotations

from typing import Protocol

from specbench.application.construct_run import Outcome, TaskStatus


class NotReopenable(Exception):
    """The task has nothing to reopen (no status yet) or is sealed."""


class ReopenableStatuses(Protocol):
    def read(self, task_id: str) -> TaskStatus | None: ...

    def set_aside(self, task_id: str) -> str:
        """Moves the task's status, plain files and reports aside and says where they went."""
        ...


def reopen(task_id: str, statuses: ReopenableStatuses) -> str:
    """Set a dropped task's last attempt aside; the place it went to."""
    status = statuses.read(task_id)
    if status is None:
        raise NotReopenable(f"{task_id} has no status: construction has not finished it, so it is open already")
    if status.outcome is Outcome.SEALED:
        raise NotReopenable(f"{task_id} is sealed, and a sealed task is final")
    return statuses.set_aside(task_id)
