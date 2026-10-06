"""The gate rules of PREREG 9.2 (ADR 0001): one implementation, every policy.

Rules, in order: 1 traceability, 2 reference passes, 3 a surviving VALID mutant,
4 no killed VALID mutant, 5 a surviving UNDETERMINED mutant, 6 alignment.
Rules 1 to 4 block, rules 5 and 6 escalate, and BLOCK > ESCALATE > PASS.
"""

from __future__ import annotations

from collections.abc import Collection, Mapping
from dataclasses import dataclass
from enum import Enum, IntEnum

from pilot.domain.alignment_aggregate import AlignmentVerdict


class GateOutcome(IntEnum):
    PASS = 0
    ESCALATE = 1
    BLOCK = 2


class Rule(Enum):
    TRACEABILITY = 1
    REFERENCE_PASSES = 2
    SURVIVING_VALID = 3
    NO_KILLED_VALID = 4
    SURVIVING_UNDETERMINED = 5
    ALIGNMENT = 6

    @property
    def outcome(self) -> GateOutcome:
        if self.value <= Rule.NO_KILLED_VALID.value:
            return GateOutcome.BLOCK
        return GateOutcome.ESCALATE


class EscalationHandling(Enum):
    KEEP = "keep"
    AS_BLOCK = "as_block"
    AS_PASS = "as_pass"


@dataclass(frozen=True)
class GatePolicy:
    name: str
    rules: frozenset[Rule]
    escalation: EscalationHandling = EscalationHandling.KEEP

    def finalize(self, outcome: GateOutcome) -> GateOutcome:
        if outcome is not GateOutcome.ESCALATE or self.escalation is EscalationHandling.KEEP:
            return outcome
        return GateOutcome.BLOCK if self.escalation is EscalationHandling.AS_BLOCK else GateOutcome.PASS


_ALL_RULES = frozenset(Rule)
G0 = GatePolicy("G0", frozenset({Rule.TRACEABILITY}))
G1 = GatePolicy("G1", frozenset(r for r in Rule if r.value <= Rule.SURVIVING_UNDETERMINED.value))
G2 = GatePolicy("G2", frozenset({Rule.TRACEABILITY, Rule.ALIGNMENT}))
G3 = GatePolicy("G3", _ALL_RULES)
G3_STRICT = GatePolicy("G3-strict", _ALL_RULES, EscalationHandling.AS_BLOCK)
G3_LENIENT = GatePolicy("G3-lenient", _ALL_RULES, EscalationHandling.AS_PASS)
POLICIES: Mapping[str, GatePolicy] = {
    p.name: p for p in (G0, G1, G2, G3, G3_STRICT, G3_LENIENT)
}


@dataclass(frozen=True)
class CriterionEvidence:
    """What the gate knows about one criterion: the kill sets of its mutants."""

    valid_kill_sets: tuple[frozenset[str], ...]
    undetermined_kill_sets: tuple[frozenset[str], ...]


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


def _survives(kill_set: frozenset[str], tests: Collection[str]) -> bool:
    return kill_set.isdisjoint(tests)


def _fires(rule: Rule, tests: Collection[str], inputs: GateInputs) -> bool:
    evidence = inputs.evidence
    if rule is Rule.TRACEABILITY:
        return not inputs.traceability_pass
    if rule is Rule.REFERENCE_PASSES:
        return any(not inputs.reference_passes.get(test, False) for test in tests)
    if rule is Rule.SURVIVING_VALID:
        return any(_survives(kills, tests) for kills in evidence.valid_kill_sets)
    if rule is Rule.NO_KILLED_VALID:
        return not any(not _survives(kills, tests) for kills in evidence.valid_kill_sets)
    if rule is Rule.SURVIVING_UNDETERMINED:
        return any(_survives(kills, tests) for kills in evidence.undetermined_kill_sets)
    return any(
        inputs.alignment.get(test, AlignmentVerdict.UNDETERMINED) is not AlignmentVerdict.ALIGNED
        for test in tests
    )


def evaluate_gate(policy: GatePolicy, tests: Collection[str], inputs: GateInputs) -> GateDecision:
    fired = tuple(rule for rule in Rule if rule in policy.rules and _fires(rule, tests, inputs))
    raw = max((rule.outcome for rule in fired), default=GateOutcome.PASS)
    return GateDecision(policy.finalize(raw), fired)
