"""Two-way verification of a task (ADR 0004 section 2): rules over test outcomes, no execution here.

Flake policy for verification: each step runs `VERIFICATION_RUNS` times and a test
whose outcomes differ between runs is FLAKY; a FLAKY test on a step's decisive set
fails that step, so the task is dropped and logged. The defect truth keeps its own
2-of-3 rule; verification demands stability because it certifies the oracle itself.
"""

from __future__ import annotations

from collections import Counter
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from enum import Enum
from typing import Final

from specbench.domain.canonical import JsonValue
from specbench.domain.oracle import MIN_PLANTED, MIN_VIOLATIONS_PER_CRITERION, Oracle, OracleTest, Outcome, Part
from specbench.domain.task import Criterion

VERIFICATION_RUNS: Final = 3

SuiteRun = Mapping[str, Outcome]


class CheckStatus(Enum):
    PASSED = "passed"
    FAILED = "failed"
    NOT_EVALUATED = "not-evaluated"


class CheckName(Enum):
    MERGE_PASSES = "merge-passes-all"
    BASE_FAILS = "base-fails-new-criteria"
    VIOLATIONS_DIAGONAL = "violations-diagonal"
    PLANTED_PASS = "planted-non-violations-pass"
    OPERATOR_SURVIVORS = "operator-mutant-survivors"


DECISIVE_CHECKS: Final = (
    CheckName.MERGE_PASSES,
    CheckName.BASE_FAILS,
    CheckName.VIOLATIONS_DIAGONAL,
    CheckName.PLANTED_PASS,
)


@dataclass(frozen=True)
class CheckResult:
    name: CheckName
    status: CheckStatus
    problems: tuple[str, ...] = ()

    def to_record(self) -> dict[str, JsonValue]:
        return {"name": self.name.value, "problems": list(self.problems), "status": self.status.value}


def stabilise(runs: Sequence[SuiteRun], expected: Sequence[str]) -> dict[str, Outcome]:
    """One outcome per expected test: the common outcome of all runs, FLAKY when they differ, MISSING when unreported."""
    stable: dict[str, Outcome] = {}
    for test_id in expected:
        seen = {run.get(test_id, Outcome.MISSING) for run in runs}
        stable[test_id] = next(iter(seen)) if len(seen) == 1 else Outcome.FLAKY
    return stable


def _flaky(outcomes: Mapping[str, Outcome], tests: Sequence[OracleTest]) -> list[str]:
    return [f"{t.test_id} is flaky" for t in tests if outcomes[t.test_id] is Outcome.FLAKY]


def _result(name: CheckName, problems: list[str]) -> CheckResult:
    return CheckResult(name, CheckStatus.FAILED if problems else CheckStatus.PASSED, tuple(problems))


def check_merge_passes(oracle: Oracle, outcomes: Mapping[str, Outcome]) -> CheckResult:
    """The merge commit passes U, R, H and O."""
    problems = [
        f"{t.test_id} ({t.part.value}) is {outcomes[t.test_id].value} at merge"
        for t in oracle.tests
        if outcomes[t.test_id] is not Outcome.PASSED
    ]
    return _result(CheckName.MERGE_PASSES, problems)


def check_base_fails(oracle: Oracle, criteria: Sequence[Criterion], outcomes: Mapping[str, Outcome]) -> CheckResult:
    """The base fails every U and H test of each new or revised criterion, and each has at least one."""
    problems: list[str] = []
    for criterion in (c for c in criteria if c.is_new_or_revised):
        tests = oracle.criterion_tests(criterion.criterion_id)
        if not tests:
            problems.append(f"{criterion.criterion_id} has no U or H test")
        problems += _flaky(outcomes, tests)
        problems += [f"{t.test_id} does not fail at base" for t in tests if outcomes[t.test_id] is Outcome.PASSED]
    return _result(CheckName.BASE_FAILS, problems)


def failing_tests(outcomes: Mapping[str, Outcome], tests: Sequence[OracleTest]) -> frozenset[str]:
    return frozenset(t.test_id for t in tests if outcomes[t.test_id].is_failure)


def check_violations_diagonal(
    oracle: Oracle, criteria: Sequence[Criterion], outcomes_by_violation: Mapping[str, Mapping[str, Outcome]]
) -> CheckResult:
    """Each violation fails tests of its own criterion only, and every criterion has at least two violations."""
    problems: list[str] = []
    counts = Counter(v.criterion for v in oracle.violations)
    for criterion in criteria:
        if counts[criterion.criterion_id] < MIN_VIOLATIONS_PER_CRITERION:
            problems.append(
                f"{criterion.criterion_id} has {counts[criterion.criterion_id]} violations, {MIN_VIOLATIONS_PER_CRITERION} required"
            )
    judged = oracle.tests_of(Part.U, Part.H)
    for violation in oracle.violations:
        outcomes = outcomes_by_violation[violation.violation_id]
        own = {t.test_id for t in oracle.criterion_tests(violation.criterion)}
        problems += [f"{violation.violation_id}: {p}" for p in _flaky(outcomes, judged)]
        failed = failing_tests(outcomes, judged)
        if not failed & own:
            problems.append(f"{violation.violation_id} fails no test of {violation.criterion}")
        for stray in sorted(failed - own):
            problems.append(f"{violation.violation_id} also fails {stray}, outside {violation.criterion}")
    return _result(CheckName.VIOLATIONS_DIAGONAL, problems)


def check_planted_pass(oracle: Oracle, outcomes_by_planted: Mapping[str, Mapping[str, Outcome]]) -> CheckResult:
    """Each planted non-violating refactor passes all of U, H and R."""
    problems: list[str] = []
    if len(oracle.planted) < MIN_PLANTED:
        problems.append(f"{len(oracle.planted)} planted refactors, {MIN_PLANTED} required")
    judged = oracle.tests_of(Part.U, Part.H, Part.R)
    for planted in oracle.planted:
        outcomes = outcomes_by_planted[planted.planted_id]
        problems += [
            f"{planted.planted_id}: {t.test_id} is {outcomes[t.test_id].value}"
            for t in judged
            if outcomes[t.test_id] is not Outcome.PASSED
        ]
    return _result(CheckName.PLANTED_PASS, problems)


@dataclass(frozen=True)
class OperatorSurvivors:
    """Operator mutants of the merge diff that no oracle test kills: the oracle-gap estimate."""

    generated: int
    survivors: tuple[str, ...]


@dataclass(frozen=True)
class VerificationReport:
    task_id: str
    runs: int
    checks: tuple[CheckResult, ...]
    survivors: OperatorSurvivors | None
    limitation: str | None

    @property
    def verified(self) -> bool:
        decisive = {c.name: c.status for c in self.checks}
        return all(decisive.get(name) is CheckStatus.PASSED for name in DECISIVE_CHECKS)

    def to_record(self) -> dict[str, JsonValue]:
        survivors: JsonValue = (
            None
            if self.survivors is None
            else {"generated": self.survivors.generated, "survivors": list(self.survivors.survivors)}
        )
        return {
            "checks": [c.to_record() for c in self.checks],
            "limitation": self.limitation,
            "operator_survivors": survivors,
            "runs": self.runs,
            "task_id": self.task_id,
            "verified": self.verified,
        }


def survivors_check(survivors: OperatorSurvivors | None) -> CheckResult:
    """The survivor listing never fails a task; it is not evaluated when no operator engine is reachable."""
    if survivors is None:
        return CheckResult(CheckName.OPERATOR_SURVIVORS, CheckStatus.NOT_EVALUATED)
    problems = tuple(f"survives: {m}" for m in survivors.survivors)
    return CheckResult(CheckName.OPERATOR_SURVIVORS, CheckStatus.PASSED, problems)
