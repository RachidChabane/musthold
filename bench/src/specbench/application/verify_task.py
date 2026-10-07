"""`verify-task`: the two-way checks of ADR 0004 section 2 on a task whose oracle and violations exist. No model call."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass

from specbench.application.ports import OperatorMutantEngine, OracleRunner, TreeFactory, TreeSpec
from specbench.domain.oracle import Oracle, OracleTest, Outcome, Part
from specbench.domain.sealing import SealedBundle
from specbench.domain.task import Task
from specbench.domain.verification import (
    VERIFICATION_RUNS,
    CheckResult,
    OperatorSurvivors,
    VerificationReport,
    check_base_fails,
    check_merge_passes,
    check_planted_pass,
    check_violations_diagonal,
    stabilise,
    survivors_check,
)

JUDGED_PARTS = (Part.U, Part.H)
SHIPPED_PARTS = (Part.U, Part.H, Part.O)


@dataclass(frozen=True)
class TaskVerifier:
    """Runs every verification step `runs` times on trees it materializes, and applies the rules of the domain."""

    trees: TreeFactory
    runner: OracleRunner
    operators: OperatorMutantEngine
    runs: int = VERIFICATION_RUNS

    def verify(self, task: Task, oracle: Oracle, bundle: SealedBundle) -> VerificationReport:
        overlay = {
            destination: bundle.files[source]
            for destination, source in oracle.overlay_paths(bundle, *SHIPPED_PARTS).items()
        }

        def spec(commit: str, *patches: bytes) -> TreeSpec:
            return TreeSpec(commit, patches, overlay)

        everything = oracle.tests
        merge = self._stable(spec(task.merge_sha), everything)
        base = self._stable(spec(task.base_sha), oracle.tests_of(*JUDGED_PARTS))
        violations = {
            v.violation_id: self._stable(
                spec(task.merge_sha, bundle.files[v.patch_path]), oracle.tests_of(*JUDGED_PARTS)
            )
            for v in oracle.violations
        }
        planted = {
            p.planted_id: self._stable(
                spec(task.merge_sha, bundle.files[p.patch_path]), oracle.tests_of(Part.U, Part.H, Part.R)
            )
            for p in oracle.planted
        }
        survivors = self._operator_survivors(task, everything, overlay)
        checks: tuple[CheckResult, ...] = (
            check_merge_passes(oracle, merge),
            check_base_fails(oracle, task.criteria, base),
            check_violations_diagonal(oracle, task.criteria, violations),
            check_planted_pass(oracle, planted),
            survivors_check(survivors),
        )
        return VerificationReport(task.task_id, self.runs, checks, survivors, self.operators.limitation)

    def _stable(self, spec: TreeSpec, tests: Sequence[OracleTest]) -> dict[str, Outcome]:
        with self.trees.materialize(spec) as tree:
            runs = [self.runner.run(tree, tests) for _ in range(self.runs)]
        return stabilise(runs, [t.test_id for t in tests])

    def _operator_survivors(
        self, task: Task, tests: Sequence[OracleTest], overlay: Mapping[str, bytes]
    ) -> OperatorSurvivors | None:
        generated = self.operators.mutants_of(task.base_sha, task.merge_sha)
        if generated is None:
            return None
        survivors: list[str] = []
        for mutant_id, mutant in sorted(generated.mutants.items()):
            outcomes = self._stable(TreeSpec(mutant.commit, mutant.patches, overlay), tests)
            if all(outcome is Outcome.PASSED for outcome in outcomes.values()):
                survivors.append(mutant_id)
        return OperatorSurvivors(len(generated.mutants), tuple(survivors))
