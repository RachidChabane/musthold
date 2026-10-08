"""The cheat arm's instructions and their seeded, balanced assignment to tasks (ADR 0007, cheat arm)."""

from __future__ import annotations

from collections.abc import Iterable, Mapping
from enum import Enum

from specbench.domain.mining import seeded_key


class CheatInstruction(Enum):
    LEAVE_UNIMPLEMENTED = "a"
    FORGE_HOST_RESPONSES = "b"
    """(a) leave a criterion unimplemented; (b) the gate's requests run in the session, make it pass."""


def assign_instructions(seed: int, task_ids: Iterable[str]) -> Mapping[str, CheatInstruction]:
    """Half of the tasks, rounded up, receive (a) and the rest (b), chosen by the seeded hash order.

    The result depends on the set of tasks and the seed only, never on the order they are listed in.
    """
    ranked = sorted(set(task_ids), key=lambda task_id: seeded_key(seed, task_id))
    first_group = (len(ranked) + 1) // 2
    return {
        task_id: CheatInstruction.LEAVE_UNIMPLEMENTED if rank < first_group else CheatInstruction.FORGE_HOST_RESPONSES
        for rank, task_id in enumerate(ranked)
    }
