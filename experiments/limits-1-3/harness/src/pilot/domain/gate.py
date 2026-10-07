"""The gate rules (PREREG 9.2, ADR 0005 applied to the pilot): one implementation, every policy.

Rules of the primary policy G3, in precedence BLOCK > ESCALATE > PASS:

1. traceability not pass: BLOCK
2. the reference fails a test in S: BLOCK
3. a VALID_x mutant (COLLATERAL included) survives every test in S: BLOCK
4. no VALID_x mutant killed by S, which includes an empty pool: ESCALATE `no_valid_mutant`
5. a surviving mutant UNDETERMINED for the criterion, or VALID_w but not VALID_x: ESCALATE
6. a test in S is MISALIGNED or UNDETERMINED: ESCALATE
7. otherwise PASS; pass credit is a VALID_x killed by S, so rule 4 also catches a pool whose mutants S
   neither kills nor lets survive (every result an error)

The sensitivities G3-prereg (the rules of the first protocol: rule 4 blocks when no VALID mutant is killed
by S, over VALID_w) and G3-noiso (VALID_w in place of VALID_x) change the evidence basis or
the outcome of rule 4 only. A mutant is killed by a test with a reproduced kill and survives
a test whose result is survived or flaky; an error is neither.
"""

from __future__ import annotations

from collections.abc import Collection, Mapping
from dataclasses import dataclass
from enum import Enum, IntEnum
from typing import Final

from pilot.domain.alignment_aggregate import AlignmentVerdict


class GateOutcome(IntEnum):
    PASS = 0
    ESCALATE = 1
    BLOCK = 2


class Rule(Enum):
    TRACEABILITY = "traceability"
    REFERENCE_PASSES = "reference_passes"
    SURVIVING_VALID = "surviving_valid"
    NO_VALID_MUTANT = "no_valid_mutant"
    NO_KILLED_VALID = "no_killed_valid"
    SURVIVING_UNDETERMINED = "surviving_undetermined"
    ALIGNMENT = "alignment"

    @property
    def outcome(self) -> GateOutcome:
        return RULE_OUTCOME[self]


RULE_OUTCOME: Final = {
    Rule.TRACEABILITY: GateOutcome.BLOCK,
    Rule.REFERENCE_PASSES: GateOutcome.BLOCK,
    Rule.SURVIVING_VALID: GateOutcome.BLOCK,
    Rule.NO_VALID_MUTANT: GateOutcome.ESCALATE,
    Rule.NO_KILLED_VALID: GateOutcome.BLOCK,
    Rule.SURVIVING_UNDETERMINED: GateOutcome.ESCALATE,
    Rule.ALIGNMENT: GateOutcome.ESCALATE,
}


class EscalationHandling(Enum):
    KEEP = "keep"
    AS_BLOCK = "as_block"
    AS_PASS = "as_pass"


class EvidenceBasis(Enum):
    """Which mutants the blocking rules read: VALID_x (isolated by execution) or VALID_w (judged)."""

    VALID_X = "VALID_x"
    VALID_W = "VALID_w"


@dataclass(frozen=True)
class GatePolicy:
    name: str
    rules: frozenset[Rule]
    escalation: EscalationHandling = EscalationHandling.KEEP
    basis: EvidenceBasis = EvidenceBasis.VALID_X
    admissible: bool = True

    def finalize(self, outcome: GateOutcome) -> GateOutcome:
        if outcome is not GateOutcome.ESCALATE or self.escalation is EscalationHandling.KEEP:
            return outcome
        return GateOutcome.BLOCK if self.escalation is EscalationHandling.AS_BLOCK else GateOutcome.PASS

    @property
    def label(self) -> str:
        return self.name if self.admissible else f"{self.name} (not an admissible product policy)"


_BLOCKING_AND_ESCALATING = frozenset(
    {
        Rule.TRACEABILITY,
        Rule.REFERENCE_PASSES,
        Rule.SURVIVING_VALID,
        Rule.NO_VALID_MUTANT,
        Rule.SURVIVING_UNDETERMINED,
        Rule.ALIGNMENT,
    }
)
_PREREG_RULES = (_BLOCKING_AND_ESCALATING - {Rule.NO_VALID_MUTANT}) | {Rule.NO_KILLED_VALID}
G0 = GatePolicy("G0", frozenset({Rule.TRACEABILITY}))
G1 = GatePolicy("G1", _BLOCKING_AND_ESCALATING - {Rule.ALIGNMENT})
G2 = GatePolicy("G2", frozenset({Rule.TRACEABILITY, Rule.REFERENCE_PASSES, Rule.ALIGNMENT}))
G3 = GatePolicy("G3", _BLOCKING_AND_ESCALATING)
G3_PREREG = GatePolicy("G3-prereg", _PREREG_RULES, basis=EvidenceBasis.VALID_W)
G3_NOISO = GatePolicy("G3-noiso", _BLOCKING_AND_ESCALATING, basis=EvidenceBasis.VALID_W)
G3_STRICT = GatePolicy("G3-strict", _BLOCKING_AND_ESCALATING, EscalationHandling.AS_BLOCK, admissible=False)
G3_LENIENT = GatePolicy("G3-lenient", _BLOCKING_AND_ESCALATING, EscalationHandling.AS_PASS)
POLICIES: Mapping[str, GatePolicy] = {
    p.name: p for p in (G0, G1, G2, G3, G3_PREREG, G3_NOISO, G3_STRICT, G3_LENIENT)
}


@dataclass(frozen=True)
class MutantResults:
    """Executed results of one mutant: tests with a reproduced kill, tests that errored; the others survived."""

    killed: frozenset[str]
    errored: frozenset[str] = frozenset()

    def killed_by_any(self, tests: Collection[str]) -> bool:
        return not self.killed.isdisjoint(tests)

    def survives_every(self, tests: Collection[str]) -> bool:
        return self.killed.isdisjoint(tests) and self.errored.isdisjoint(tests)


@dataclass(frozen=True)
class CriterionEvidence:
    """What the gate knows about one criterion: the executed results of its mutants, split by class."""

    valid_x: tuple[MutantResults, ...]
    valid_w_only: tuple[MutantResults, ...] = ()
    undetermined: tuple[MutantResults, ...] = ()

    def blocking_pool(self, basis: EvidenceBasis) -> tuple[MutantResults, ...]:
        return self.valid_x if basis is EvidenceBasis.VALID_X else self.valid_x + self.valid_w_only

    def escalating_pool(self, basis: EvidenceBasis) -> tuple[MutantResults, ...]:
        if basis is EvidenceBasis.VALID_X:
            return self.valid_w_only + self.undetermined
        return self.undetermined


@dataclass(frozen=True)
class GateInputs:
    traceability_pass: bool
    reference_passes: Mapping[str, bool]
    evidence: CriterionEvidence
    alignment: Mapping[str, AlignmentVerdict]


@dataclass(frozen=True)
class GateDecision:
    outcome: GateOutcome
    fired: tuple[Rule, ...]


def _fires(rule: Rule, tests: Collection[str], inputs: GateInputs, basis: EvidenceBasis) -> bool:
    evidence = inputs.evidence
    if rule is Rule.TRACEABILITY:
        return not inputs.traceability_pass
    if rule is Rule.REFERENCE_PASSES:
        return any(not inputs.reference_passes.get(test, False) for test in tests)
    if rule is Rule.SURVIVING_VALID:
        return any(mutant.survives_every(tests) for mutant in evidence.blocking_pool(basis))
    if rule is Rule.NO_VALID_MUTANT or rule is Rule.NO_KILLED_VALID:
        return not any(mutant.killed_by_any(tests) for mutant in evidence.blocking_pool(basis))
    if rule is Rule.SURVIVING_UNDETERMINED:
        return any(mutant.survives_every(tests) for mutant in evidence.escalating_pool(basis))
    return any(
        inputs.alignment.get(test, AlignmentVerdict.UNDETERMINED) is not AlignmentVerdict.ALIGNED
        for test in tests
    )


def evaluate_gate(policy: GatePolicy, tests: Collection[str], inputs: GateInputs) -> GateDecision:
    fired = tuple(rule for rule in Rule if rule in policy.rules and _fires(rule, tests, inputs, policy.basis))
    raw = max((rule.outcome for rule in fired), default=GateOutcome.PASS)
    return GateDecision(policy.finalize(raw), fired)
