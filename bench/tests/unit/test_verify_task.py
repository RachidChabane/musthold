from __future__ import annotations

from pathlib import Path

import pytest

from specbench.application.ports import OperatorMutants, TreeSpec
from specbench.application.verify_task import TaskVerifier
from specbench.domain.oracle import OracleTest, Outcome, Part, oracle_from_bundle
from specbench.domain.verification import CheckName, CheckStatus, VerificationReport
from support.builders import BASE, MERGE, TASK_ID, make_bundle, make_task
from support.fakes import Behaviour, FixedOperatorEngine, ListJournal, RecordingTrees, ScriptedRunner

TOTAL_RUNS = 3


def patch_name(spec: TreeSpec) -> str | None:
    return spec.patches[0].decode().split()[1] if spec.patches else None


def correct(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
    """A well-formed task: merge passes, base fails new criteria, violations fail only their own criterion."""
    name = patch_name(spec)
    if spec.commit == BASE and test.part in (Part.U, Part.H):
        return Outcome.FAILED
    if name is not None and name.startswith("violations/"):
        target = {"V1": 1, "V2": 1, "V3": 2, "V4": 2}[name.split("/")[1]]
        return Outcome.FAILED if test.criterion == f"{TASK_ID}-{target}" else Outcome.PASSED
    return Outcome.PASSED


def verify(
    tmp_path: Path,
    behaviour: Behaviour,
    operators: FixedOperatorEngine | None = None,
    journal: ListJournal | None = None,
) -> VerificationReport:
    trees = RecordingTrees(tmp_path)
    bundle = make_bundle()
    verifier = TaskVerifier(
        trees,
        ScriptedRunner(trees, behaviour),
        operators or FixedOperatorEngine(None, "no engine"),
        journal=journal or ListJournal(),
    )
    return verifier.verify(make_task(), oracle_from_bundle(bundle), bundle)


def status(report: VerificationReport, name: CheckName) -> CheckStatus:
    return next(c.status for c in report.checks if c.name is name)


def problems(report: VerificationReport, name: CheckName) -> tuple[str, ...]:
    return next(c.problems for c in report.checks if c.name is name)


def test_a_well_formed_task_is_verified(tmp_path: Path) -> None:
    report = verify(tmp_path, correct)
    assert report.verified
    assert [c.status for c in report.checks[:4]] == [CheckStatus.PASSED] * 4
    assert status(report, CheckName.OPERATOR_SURVIVORS) is CheckStatus.NOT_EVALUATED
    assert report.limitation == "no engine"


def test_each_step_runs_three_times_on_one_tree(tmp_path: Path) -> None:
    seen: dict[str, set[int]] = {}

    def spy(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
        seen.setdefault(f"{spec.commit}:{patch_name(spec)}", set()).add(run)
        return correct(spec, test, run)

    verify(tmp_path, spy)
    assert all(runs == set(range(TOTAL_RUNS)) for runs in seen.values())
    assert len(seen) == 1 + 1 + 4 + 2


def test_every_run_of_every_step_is_journaled_with_its_own_outcomes(tmp_path: Path) -> None:
    def behaviour(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
        if patch_name(spec) == "violations/V3" and test.part is Part.H and run == 2:
            return Outcome.MISSING
        return correct(spec, test, run)

    journal = ListJournal()
    verify(tmp_path, behaviour, journal=journal)
    steps = [
        "merge",
        "base",
        "violation V1",
        "violation V2",
        "violation V3",
        "violation V4",
        "planted P1",
        "planted P2",
    ]
    assert [(step, run) for step, run, _ in journal.runs] == [
        (step, run) for step in steps for run in range(1, TOTAL_RUNS + 1)
    ]
    third_violation = [outcomes for step, _, outcomes in journal.runs if step == "violation V3"]
    assert [Outcome.MISSING in outcomes.values() for outcomes in third_violation] == [False, False, True]


def test_merge_failure_drops_the_task(tmp_path: Path) -> None:
    def behaviour(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
        if spec.commit == MERGE and not spec.patches and test.part is Part.O:
            return Outcome.FAILED
        return correct(spec, test, run)

    report = verify(tmp_path, behaviour)
    assert not report.verified
    assert "(O) is failed at merge" in problems(report, CheckName.MERGE_PASSES)[0]


def test_a_u_test_that_passes_at_base_fails_the_base_check(tmp_path: Path) -> None:
    def behaviour(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
        if spec.commit == BASE and test.part is Part.U:
            return Outcome.PASSED
        return correct(spec, test, run)

    report = verify(tmp_path, behaviour)
    assert status(report, CheckName.BASE_FAILS) is CheckStatus.FAILED
    assert not report.verified


def test_a_violation_that_breaks_another_criterion_is_not_diagonal(tmp_path: Path) -> None:
    def behaviour(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
        if patch_name(spec) == "violations/V1" and test.criterion == f"{TASK_ID}-2":
            return Outcome.FAILED
        return correct(spec, test, run)

    report = verify(tmp_path, behaviour)
    assert any("V1 also fails" in p for p in problems(report, CheckName.VIOLATIONS_DIAGONAL))


def test_a_violation_that_fails_nothing_is_not_diagonal(tmp_path: Path) -> None:
    def behaviour(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
        return Outcome.PASSED if patch_name(spec) == "violations/V3" else correct(spec, test, run)

    report = verify(tmp_path, behaviour)
    assert any("V3 fails no test of" in p for p in problems(report, CheckName.VIOLATIONS_DIAGONAL))


def test_a_planted_refactor_that_fails_a_test_is_a_defect_of_the_task(tmp_path: Path) -> None:
    def behaviour(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
        if patch_name(spec) == "planted/P2" and test.part is Part.R:
            return Outcome.ERROR
        return correct(spec, test, run)

    report = verify(tmp_path, behaviour)
    assert status(report, CheckName.PLANTED_PASS) is CheckStatus.FAILED


def test_a_flaky_test_on_a_decisive_set_fails_the_step(tmp_path: Path) -> None:
    def behaviour(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
        if spec.commit == MERGE and not spec.patches and test.part is Part.H and run == 1:
            return Outcome.FAILED
        return correct(spec, test, run)

    report = verify(tmp_path, behaviour)
    assert "flaky" in problems(report, CheckName.MERGE_PASSES)[0]
    assert not report.verified


def test_operator_survivors_are_listed_and_never_fail_the_task(tmp_path: Path) -> None:
    mutants = OperatorMutants(
        {
            "M1": TreeSpec(MERGE, (b"--- operators/M1\n",)),
            "M2": TreeSpec(MERGE, (b"--- operators/M2\n",)),
        }
    )

    def behaviour(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
        if patch_name(spec) == "operators/M1" and test.part is Part.U:
            return Outcome.FAILED
        return correct(spec, test, run)

    report = verify(tmp_path, behaviour, FixedOperatorEngine(mutants))
    assert report.verified
    assert report.survivors is not None
    assert report.survivors.generated == 2 and report.survivors.survivors == ("M2",)
    assert report.limitation is None


@pytest.mark.parametrize("missing", [Outcome.MISSING, Outcome.ERROR])
def test_collection_errors_count_as_failures_at_base(tmp_path: Path, missing: Outcome) -> None:
    def behaviour(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
        return missing if spec.commit == BASE else correct(spec, test, run)

    assert status(verify(tmp_path, behaviour), CheckName.BASE_FAILS) is CheckStatus.PASSED
