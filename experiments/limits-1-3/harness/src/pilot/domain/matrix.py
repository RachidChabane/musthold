"""The tests x mutants matrix classifier (PREREG 8): signatures, binary verdict, label, DC.

Pure functions over kill sets; nothing here knows where the kills came from.
"""

from __future__ import annotations

from collections import defaultdict
from collections.abc import Iterable, Mapping, Sequence
from dataclasses import dataclass
from enum import Enum

from pilot.domain.analysis_constants import THETA_OTHER, THETA_OWN
from pilot.domain.alignment_aggregate import AlignmentVerdict

AMBIGUOUS_LABEL = "AMBIGUOUS"
NO_LABEL = "NONE"


class Signature(Enum):
    UNDEFINED_OWN = "UNDEFINED_OWN"
    INERT = "INERT"
    OFF_TARGET = "OFF_TARGET"
    PARTIAL = "PARTIAL"
    SPECIFIC = "SPECIFIC"
    OVERBROAD = "OVERBROAD"

    @property
    def verdict(self) -> AlignmentVerdict:
        if self is Signature.UNDEFINED_OWN:
            return AlignmentVerdict.UNDETERMINED
        if self in (Signature.SPECIFIC, Signature.OVERBROAD):
            return AlignmentVerdict.ALIGNED
        return AlignmentVerdict.MISALIGNED


@dataclass(frozen=True)
class PoolMutant:
    """A VALID mutant of `criterion` with the tests that killed it."""

    criterion: int
    killed_by: frozenset[str]


@dataclass(frozen=True)
class TestEvidence:
    """Kill shares of one test, independent of its declared criterion."""

    __test__ = False

    nodeid: str
    valid_shares: Mapping[int, float]
    none_share: float

    @property
    def kills_anything(self) -> bool:
        return self.none_share > 0 or any(share > 0 for share in self.valid_shares.values())

    @property
    def criteria_killed(self) -> tuple[int, ...]:
        return tuple(sorted(c for c, share in self.valid_shares.items() if share > 0))


@dataclass(frozen=True)
class TestSignature:
    __test__ = False

    nodeid: str
    declared: int
    signature: Signature
    own: float | None
    other: float
    none: float
    dc: float | None
    criteria_killed: tuple[int, ...]

    @property
    def verdict(self) -> AlignmentVerdict:
        return self.signature.verdict

    @property
    def criterion_label(self) -> str:
        """The single criterion whose VALID mutants the test kills; NONE or AMBIGUOUS otherwise."""
        criteria = list(self.criteria_killed)
        if not criteria:
            return NO_LABEL
        return str(criteria[0]) if len(criteria) == 1 else AMBIGUOUS_LABEL


def build_evidence(
    nodeids: Iterable[str],
    valid_pool: Sequence[PoolMutant],
    unattributed: Sequence[frozenset[str]],
) -> dict[str, TestEvidence]:
    totals: dict[int, int] = defaultdict(int)
    kills: dict[str, dict[int, int]] = defaultdict(lambda: defaultdict(int))
    none_kills: dict[str, int] = defaultdict(int)
    for mutant in valid_pool:
        totals[mutant.criterion] += 1
        for nodeid in mutant.killed_by:
            kills[nodeid][mutant.criterion] += 1
    for killed_by in unattributed:
        for nodeid in killed_by:
            none_kills[nodeid] += 1
    return {
        nodeid: TestEvidence(
            nodeid,
            {c: kills[nodeid][c] / totals[c] for c in sorted(totals)},
            none_kills[nodeid] / len(unattributed) if unattributed else 0.0,
        )
        for nodeid in nodeids
    }


def classify_test(
    evidence: TestEvidence,
    declared: int,
    theta_own: float = THETA_OWN,
    theta_other: float = THETA_OTHER,
) -> TestSignature:
    shares = evidence.valid_shares
    others = [share for criterion, share in shares.items() if criterion != declared]
    other = max(others, default=0.0)
    own = shares.get(declared)
    dc = None if own is None else own - (sum(others) / len(others) if others else 0.0)
    signature = _signature(evidence, own, other, theta_own, theta_other)
    return TestSignature(
        evidence.nodeid, declared, signature, own, other, evidence.none_share, dc, evidence.criteria_killed
    )


def _signature(
    evidence: TestEvidence, own: float | None, other: float, theta_own: float, theta_other: float
) -> Signature:
    if own is None:
        return Signature.UNDEFINED_OWN
    if not evidence.kills_anything:
        return Signature.INERT
    if own == 0:
        return Signature.OFF_TARGET
    if own < theta_own:
        return Signature.PARTIAL
    return Signature.SPECIFIC if other <= theta_other else Signature.OVERBROAD


def classify_tests(
    evidence: Mapping[str, TestEvidence], declared: Mapping[str, int]
) -> dict[str, TestSignature]:
    return {nodeid: classify_test(evidence[nodeid], declared[nodeid]) for nodeid in declared}


def mean_dc(signatures: Iterable[TestSignature]) -> float | None:
    values = [s.dc for s in signatures if s.dc is not None]
    return sum(values) / len(values) if values else None
