"""The nightly cap of model executions, handed out to the jobs that share one harness in priority order.

The order is the pilot, the canaries' reserve, construction (while a selected task is unsealed), then the sample. A
share is a job's total allowance for the night: the job compares it with its own ledger, so what the job already spent
is not subtracted from it. What the higher-priority jobs spent or claimed is.
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum
from typing import Final

DEFAULT_NIGHTLY_CAP: Final = 250
DEFAULT_CONSTRUCT_ALLOWANCE: Final = 120
DEFAULT_CLAUDE_CANARY_RESERVE: Final = 1
DEFAULT_CODEX_CANARY_RESERVE: Final = 0


class NightJob(Enum):
    PILOT = "pilot"
    CANARIES = "canaries"
    CONSTRUCT = "construct"
    SAMPLE = "sample"


@dataclass(frozen=True)
class NightInputs:
    """What one harness's cap is shared among tonight.

    `canary_reserve` is the executions the canaries measured. `construct_allowance` is construction's own ceiling for
    the night, and `construct_pending` says whether a selected task is still unsealed.
    """

    cap: int
    pilot_spent: int
    canary_reserve: int
    construct_allowance: int
    construct_spent: int
    construct_pending: bool

    def __post_init__(self) -> None:
        negative = (self.cap, self.pilot_spent, self.canary_reserve, self.construct_allowance, self.construct_spent)
        if any(value < 0 for value in negative):
            raise ValueError("a night's executions are never negative")


def _left(cap: int, *claims: int) -> int:
    return max(0, cap - sum(claims))


def canary_claim(inputs: NightInputs) -> int:
    return min(inputs.canary_reserve, _left(inputs.cap, inputs.pilot_spent))


def construct_share(inputs: NightInputs) -> int:
    if not inputs.construct_pending:
        return 0
    return min(inputs.construct_allowance, _left(inputs.cap, inputs.pilot_spent, canary_claim(inputs)))


def construct_claim(inputs: NightInputs) -> int:
    """What construction keeps out of the sample's reach: its share while it has work, and at least what it spent."""
    return max(inputs.construct_spent, construct_share(inputs))


def sample_share(inputs: NightInputs) -> int:
    return _left(inputs.cap, inputs.pilot_spent, canary_claim(inputs), construct_claim(inputs))


def share_of(job: NightJob, inputs: NightInputs) -> int:
    """The executions `job` may spend tonight on the harness `inputs` describes."""
    if job is NightJob.PILOT:
        return inputs.cap
    if job is NightJob.CANARIES:
        return canary_claim(inputs)
    if job is NightJob.CONSTRUCT:
        return construct_share(inputs)
    return sample_share(inputs)
