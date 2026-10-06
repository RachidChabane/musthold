"""Execution validation (VALID_x): what the tests of other criteria say about a mutant.

A mutant that a test bound to another criterion kills, or errors on, is not evidence about
the criterion it is judged against; execution, not a judgment, decides it. The class
VALID_x and the isolation table of the hand-written items use the one predicate below.
"""

from __future__ import annotations

from collections.abc import Collection, Mapping
from typing import Final

from pilot.domain.outcomes import KillResult

VETOING_RESULTS: Final = (KillResult.KILLED, KillResult.ERROR)


def other_binding_veto(results: Mapping[str, KillResult], bindings: Mapping[str, str], criterion: str) -> bool:
    """True when a test bound to a criterion other than `criterion` has a reproduced kill or an error."""
    return any(
        result in VETOING_RESULTS and bindings.get(nodeid) != criterion for nodeid, result in results.items()
    )


def valid_x_criteria(
    valid_w: Collection[str],
    f0_passed: bool,
    results: Mapping[str, KillResult],
    bindings: Mapping[str, str],
) -> frozenset[str]:
    """VALID_x,c: VALID_w,c, F0 passed, and no other binding's test kills or errors on the mutant."""
    if not f0_passed:
        return frozenset()
    return frozenset(ref for ref in valid_w if not other_binding_veto(results, bindings, ref))
