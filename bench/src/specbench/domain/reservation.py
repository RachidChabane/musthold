"""How many executions a live gate reserves before it starts.

`r = 1 + ceil(1.25 * E) + 4`: the session itself, the expected executions `E` with a quarter of headroom, and one wave
of four subagents, which is how far a session can overshoot before the adapter's kill takes effect. `E` is the running
median of the executions that completed gates of the same harness, arm and criteria count spent; until a key has
history it is a seed taken from the arm's planning estimate.
"""

from __future__ import annotations

import math
import statistics
from collections.abc import Mapping
from dataclasses import dataclass, field
from typing import Final

from specbench.domain.arms import Arm, Harness
from specbench.domain.canonical import JsonValue, canonical_json, parse_canonical
from specbench.domain.jsonread import RecordError, integer, mapping, sequence

SESSION_EXECUTIONS: Final = 1
HEADROOM_FACTOR: Final = 1.25
WAVE_ALLOWANCE: Final = 4
REFERENCE_CRITERIA: Final = 4
FULL_EVALUATION_EXECUTIONS: Final = 78
"""A full evaluation of a spec with four criteria: about 77 tickets and the session."""
RE_EVALUATION_EXECUTIONS: Final = 30
"""A re-evaluation after a block, at four criteria."""
SEED_EXECUTIONS_AT_REFERENCE_CRITERIA: Final = {
    Arm.TR: RE_EVALUATION_EXECUTIONS,
    Arm.AL: RE_EVALUATION_EXECUTIONS,
    Arm.DP: RE_EVALUATION_EXECUTIONS,
    Arm.CO: RE_EVALUATION_EXECUTIONS,
    Arm.PL: RE_EVALUATION_EXECUTIONS,
    Arm.ALL: FULL_EVALUATION_EXECUTIONS,
    Arm.ALL_NATIVE: FULL_EVALUATION_EXECUTIONS,
}
"""Executions of one gate per arm at four criteria; an arm that is not listed is seeded as a full evaluation."""
EXECUTION_HISTORY_FILE_NAME: Final = "execution_history.json"


def history_key(harness: Harness, arm: Arm, criteria: int) -> str:
    return f"{harness.value}/{arm.value}/{criteria}"


def seed_executions(arm: Arm, criteria: int) -> int:
    """The planning estimate of one gate of `arm` on a spec with `criteria` criteria, scaled linearly from four."""
    if criteria < 1:
        raise RecordError("a spec has at least one criterion")
    seed = SEED_EXECUTIONS_AT_REFERENCE_CRITERIA.get(arm, FULL_EVALUATION_EXECUTIONS)
    return math.ceil(seed * criteria / REFERENCE_CRITERIA)


def reservation_for(expected: int) -> int:
    """The reservation for a gate that is expected to spend `expected` executions besides its session."""
    return SESSION_EXECUTIONS + math.ceil(HEADROOM_FACTOR * expected) + WAVE_ALLOWANCE


@dataclass(frozen=True)
class ExecutionHistory:
    """The executions spent by completed gates, per harness, arm and criteria count."""

    observed: Mapping[str, tuple[int, ...]] = field(default_factory=dict[str, tuple[int, ...]])

    def with_observation(self, harness: Harness, arm: Arm, criteria: int, executions: int) -> ExecutionHistory:
        key = history_key(harness, arm, criteria)
        return ExecutionHistory({**self.observed, key: (*self.observed.get(key, ()), executions)})

    def expected(self, harness: Harness, arm: Arm, criteria: int) -> int:
        """The running median of what this key spent, or the seed while it has no history."""
        seen = self.observed.get(history_key(harness, arm, criteria))
        return statistics.median_high(seen) if seen else seed_executions(arm, criteria)

    def reservation(self, harness: Harness, arm: Arm, criteria: int) -> int:
        return reservation_for(self.expected(harness, arm, criteria))

    def to_json(self) -> str:
        record: dict[str, JsonValue] = {key: list(values) for key, values in sorted(self.observed.items())}
        return canonical_json(record)

    @classmethod
    def from_json(cls, content: str) -> ExecutionHistory:
        body = mapping(parse_canonical(content), "execution history")
        return cls(
            {
                key: tuple(integer(value, f"execution history.{key}") for value in sequence(values, key))
                for key, values in body.items()
            }
        )
