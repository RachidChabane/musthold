"""Label verification by execution (PREREG 3.2, items 1-9).

One rule per test class, selected through a strategy table; pure over outcomes.
"""

from __future__ import annotations

from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass

from pilot.domain.constants import (
    EXPECTED_CRITERION_COUNT,
    EXPECTED_PLANTED_COUNT,
    EXPECTED_TEST_COUNT,
    EXPECTED_VIOLATION_COUNT,
    REFERENCE_VARIANT_ID,
)
from pilot.domain.labels import TestClass, TestLabel
from pilot.domain.outcome_matrix import VariantOutcomes
from pilot.domain.variants import VariantId, VariantKind

STRUCTURE_RULE = "structure"
ANOMALY_RULE = "anomaly"
MISSING_RULE = "missing"
MISSING_EXAMPLES_SHOWN = 3


@dataclass(frozen=True)
class ExpectedCounts:
    tests: int = EXPECTED_TEST_COUNT
    violations: int = EXPECTED_VIOLATION_COUNT
    planted: int = EXPECTED_PLANTED_COUNT
    criteria: int = EXPECTED_CRITERION_COUNT


@dataclass(frozen=True)
class RuleFailure:
    rule: str
    message: str

    def __str__(self) -> str:
        return f"[{self.rule}] {self.message}"


class _Evidence:
    """Read-only view of the runs, grouped the way the rules speak about them."""

    def __init__(self, runs: Mapping[str, VariantOutcomes]) -> None:
        self.reference = runs[REFERENCE_VARIANT_ID]
        parsed = {
            VariantId.parse(key): run
            for key, run in runs.items()
            if key != REFERENCE_VARIANT_ID
        }
        self.violations = _of_kind(parsed, VariantKind.VIOLATION)
        self.planted = _of_kind(parsed, VariantKind.PLANTED)
        self.refactor = _of_kind(parsed, VariantKind.REFACTOR)

    def violations_of(self, criterion: int) -> list[VariantOutcomes]:
        return [r for v, r in self.violations if v.criterion == criterion]

    def violations_except(self, criterion: int) -> list[VariantOutcomes]:
        return [r for v, r in self.violations if v.criterion != criterion]

    @property
    def all_violations(self) -> list[VariantOutcomes]:
        return [r for _, r in self.violations]

    @property
    def refactor_runs(self) -> list[VariantOutcomes]:
        return [r for _, r in self.refactor]


def _of_kind(
    parsed: Mapping[VariantId, VariantOutcomes], kind: VariantKind
) -> list[tuple[VariantId, VariantOutcomes]]:
    return sorted(
        ((v, r) for v, r in parsed.items() if v.kind is kind),
        key=lambda pair: pair[0].sort_key,
    )


def evaluate(
    labels: Sequence[TestLabel],
    runs: Mapping[str, VariantOutcomes],
    expected: ExpectedCounts,
) -> list[RuleFailure]:
    evidence = _Evidence(runs)
    failures = _structure_failures(labels, evidence, expected)
    failures += _anomaly_failures(runs) + _missing_failures(labels, runs, expected)
    failures += _reference_and_oracle_failures(labels, evidence, expected)
    failures += _oracle_diagonal_failures(evidence, expected)
    failures += _planted_oracle_failures(evidence)
    for label in labels:
        failures += _LABEL_RULES[label.test_class](label, evidence)
    return failures


def _structure_failures(
    labels: Sequence[TestLabel], evidence: _Evidence, expected: ExpectedCounts
) -> list[RuleFailure]:
    failures = []
    counts = (
        ("labelled tests", len(labels), expected.tests),
        ("violations", len(evidence.violations), expected.violations),
        ("planted items", len(evidence.planted), expected.planted),
        ("refactor variants", len(evidence.refactor), 1),
    )
    for name, actual, wanted in counts:
        if actual != wanted:
            failures.append(RuleFailure(STRUCTURE_RULE, f"{name}: {actual}, expected {wanted}"))
    labelled = {label.nodeid for label in labels}
    discovered = set(evidence.reference.project_nodeids())
    for nodeid in sorted(labelled - discovered):
        failures.append(RuleFailure(STRUCTURE_RULE, f"labelled test not collected: {nodeid}"))
    for nodeid in sorted(discovered - labelled):
        failures.append(RuleFailure(STRUCTURE_RULE, f"collected test without label: {nodeid}"))
    return failures


def _anomaly_failures(runs: Mapping[str, VariantOutcomes]) -> list[RuleFailure]:
    return [
        RuleFailure(ANOMALY_RULE, f"{variant}: {', '.join(found)}")
        for variant, run in sorted(runs.items())
        if (found := run.anomalies())
    ]


def _missing_failures(
    labels: Sequence[TestLabel], runs: Mapping[str, VariantOutcomes], expected: ExpectedCounts
) -> list[RuleFailure]:
    failures = []
    for variant, run in sorted(runs.items()):
        missing = [label.nodeid for label in labels if label.nodeid not in run.outcomes]
        missing += [
            f"oracle {criterion}"
            for criterion in range(1, expected.criteria + 1)
            if criterion not in run.oracle_criteria()
        ]
        if missing:
            shown = ", ".join(missing[:MISSING_EXAMPLES_SHOWN])
            failures.append(
                RuleFailure(MISSING_RULE, f"{variant}: no outcome for {len(missing)} item(s): {shown}")
            )
    return failures


def _reference_and_oracle_failures(
    labels: Sequence[TestLabel], evidence: _Evidence, expected: ExpectedCounts
) -> list[RuleFailure]:
    failures = []
    for label in labels:
        if not evidence.reference.of(label.nodeid).is_pass:
            failures.append(RuleFailure("1", f"reference fails {label.nodeid}"))
    for criterion in range(1, expected.criteria + 1):
        if not evidence.reference.oracle_passed(criterion):
            failures.append(RuleFailure("1", f"reference fails oracle {criterion}"))
        for run in evidence.refactor_runs:
            if not run.oracle_passed(criterion):
                failures.append(RuleFailure("1", f"R fails oracle {criterion}"))
    return failures


def _oracle_diagonal_failures(
    evidence: _Evidence, expected: ExpectedCounts
) -> list[RuleFailure]:
    failures = []
    for variant, run in evidence.violations:
        for criterion in range(1, expected.criteria + 1):
            should_fail = criterion == variant.criterion
            if run.oracle_passed(criterion) == should_fail:
                state = "passes" if should_fail else "fails"
                failures.append(
                    RuleFailure("2", f"oracle {criterion} {state} on {variant.value}")
                )
    return failures


def _planted_oracle_failures(evidence: _Evidence) -> list[RuleFailure]:
    failures = []
    for variant, run in evidence.planted:
        for criterion in run.oracle_criteria():
            if not run.oracle_passed(criterion):
                failures.append(RuleFailure("3", f"oracle {criterion} fails on {variant.value}"))
    return failures


def _fails(run: VariantOutcomes, nodeid: str) -> bool:
    return not run.of(nodeid).is_pass


def _always_fails(runs: Sequence[VariantOutcomes], nodeid: str) -> bool:
    return bool(runs) and all(_fails(r, nodeid) for r in runs)


def _ever_fails(runs: Sequence[VariantOutcomes], nodeid: str) -> bool:
    return any(_fails(r, nodeid) for r in runs)


def _fail(rule: str, label: TestLabel, message: str) -> list[RuleFailure]:
    return [RuleFailure(rule, f"{label.test_class.value} {label.nodeid}: {message}")]


def _good(label: TestLabel, ev: _Evidence) -> list[RuleFailure]:
    failures: list[RuleFailure] = []
    own, rest = ev.violations_of(label.cites), ev.violations_except(label.cites)
    if not _always_fails(own, label.nodeid):
        failures += _fail("4", label, f"does not fail under every violation of {label.cites}")
    if _ever_fails(rest, label.nodeid):
        failures += _fail("4", label, "fails under a violation of another criterion")
    return failures + _passes_refactor(label, ev, "4")


def _good_broad(label: TestLabel, ev: _Evidence) -> list[RuleFailure]:
    failures: list[RuleFailure] = []
    if not _always_fails(ev.violations_of(label.cites), label.nodeid):
        failures += _fail("5", label, f"does not fail under every violation of {label.cites}")
    if label.other is None or not _ever_fails(ev.violations_of(label.other), label.nodeid):
        failures += _fail("5", label, f"fails under no violation of {label.other}")
    return failures + _passes_refactor(label, ev, "5")


def _weak(label: TestLabel, ev: _Evidence) -> list[RuleFailure]:
    failures: list[RuleFailure] = []
    own = ev.violations_of(label.cites)
    if not (_ever_fails(own, label.nodeid) and not _always_fails(own, label.nodeid)):
        failures += _fail("6", label, f"must fail under some but not all violations of {label.cites}")
    if _ever_fails(ev.violations_except(label.cites), label.nodeid):
        failures += _fail("6", label, "fails under a violation of another criterion")
    return failures + _passes_refactor(label, ev, "6")


def _mis_other(label: TestLabel, ev: _Evidence) -> list[RuleFailure]:
    failures: list[RuleFailure] = []
    if _ever_fails(ev.violations_of(label.cites), label.nodeid):
        failures += _fail("7", label, f"fails under a violation of its cited criterion {label.cites}")
    if label.other is None or not _ever_fails(ev.violations_of(label.other), label.nodeid):
        failures += _fail("7", label, f"fails under no violation of {label.other}")
    return failures


def _vacuous(label: TestLabel, ev: _Evidence) -> list[RuleFailure]:
    failures: list[RuleFailure] = []
    if _ever_fails(ev.all_violations, label.nodeid):
        failures += _fail("8", label, "fails under a violation")
    return failures + _passes_refactor(label, ev, "8")


def _offspec(label: TestLabel, ev: _Evidence) -> list[RuleFailure]:
    failures: list[RuleFailure] = []
    if _ever_fails(ev.all_violations, label.nodeid):
        failures += _fail("9", label, "fails under a violation")
    if not _always_fails(ev.refactor_runs, label.nodeid):
        failures += _fail("9", label, "does not fail under R")
    return failures


def _passes_refactor(label: TestLabel, ev: _Evidence, rule: str) -> list[RuleFailure]:
    if _ever_fails(ev.refactor_runs, label.nodeid):
        return _fail(rule, label, "fails under R")
    return []


_LABEL_RULES: dict[TestClass, Callable[[TestLabel, _Evidence], list[RuleFailure]]] = {
    TestClass.GOOD: _good,
    TestClass.GOOD_BROAD: _good_broad,
    TestClass.WEAK: _weak,
    TestClass.MIS_OTHER: _mis_other,
    TestClass.VACUOUS: _vacuous,
    TestClass.OFFSPEC: _offspec,
}
