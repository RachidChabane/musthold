"""Execution of H candidates for their selection (ADR 0012), on the verifier's own trees, runner and stabiliser.

The merge tree carries the human test files, the H files already kept and the writing's candidates, at the paths the
verifier overlays them. The language's isolation strategy finds the candidate files that do not collect or build there;
those are left out, and the merge is run again without them. Each base tree then judges the candidates as verification
would. No outcome ever reaches a model.
"""

from __future__ import annotations

from collections.abc import Callable, Mapping, Sequence
from typing import Final, Protocol

from specbench.application.ports import HExecutionRequest, TreeSpec
from specbench.application.stabilised_runs import RunSeries, StabilisedRuns
from specbench.domain.h_selection import CandidateRuns, HExecution, uncollected_files
from specbench.domain.h_tests import HFile, HTest
from specbench.domain.oracle import OracleTest, Outcome, Part
from specbench.domain.task import Task

H_MERGE_STEP: Final = "h-merge"
H_BASE_STEP: Final = "h-base"


def oracle_tests(tests: Sequence[HTest]) -> list[OracleTest]:
    return [OracleTest(t.test_id, Part.H, None) for t in tests]


def overlay_of(human: Mapping[str, bytes], *groups: Sequence[HFile]) -> dict[str, bytes]:
    """The files a tree carries on top of its commit: the human test files, then every H file of the groups."""
    return {**human, **{f.path: f.content.encode() for files in groups for f in files}}


class CandidateIsolation(Protocol):
    """Strategy per language: which candidate files fail to collect or build at merge, and which files share a base tree."""

    def unbuildable(self, request: HExecutionRequest, merge: RunSeries) -> frozenset[str]: ...

    def base_groups(self, files: Sequence[HFile]) -> tuple[tuple[HFile, ...], ...]: ...


class PythonIsolation:
    """pytest collects each file on its own: a file none of whose tests any run reported failed to collect.

    Files never interact, so one base tree holds them all.
    """

    def unbuildable(self, request: HExecutionRequest, merge: RunSeries) -> frozenset[str]:
        return uncollected_files(request.tests, {t.test_id: merge.of(t.test_id) for t in request.tests})

    def base_groups(self, files: Sequence[HFile]) -> tuple[tuple[HFile, ...], ...]:
        return (tuple(files),) if files else ()


class TreeHCandidateExecutor:
    """`HCandidateExecutor` for one task: merge with every candidate, isolation of broken files, then base."""

    def __init__(self, runs: StabilisedRuns, isolation: CandidateIsolation) -> None:
        self._runs = runs
        self._isolation = isolation

    def execute(self, request: HExecutionRequest) -> HExecution:
        merge = self._merge(request, request.candidates, request.tests)
        merge_runs = {t.test_id: merge.of(t.test_id) for t in request.tests}
        unbuildable = self._isolation.unbuildable(request, merge)
        buildable = tuple(f for f in request.candidates if f.path not in unbuildable)
        tests = tuple(t for t in request.tests if t.path not in unbuildable)
        if unbuildable and tests:
            rerun = self._merge(request, buildable, tests)
            merge_runs |= {t.test_id: rerun.of(t.test_id) for t in tests}
        base_runs: dict[str, tuple[Outcome, ...]] = {}
        for group in self._isolation.base_groups(buildable):
            paths = {f.path for f in group}
            own = [t for t in tests if t.path in paths]
            spec = TreeSpec(request.task.base_sha, (), overlay_of(request.human_overlay, group))
            series = self._runs.series(H_BASE_STEP, spec, oracle_tests(own))
            base_runs |= {t.test_id: series.of(t.test_id) for t in own}
        runs = {t.test_id: CandidateRuns(merge_runs[t.test_id], base_runs.get(t.test_id, ())) for t in request.tests}
        return HExecution(runs, unbuildable)

    def _merge(self, request: HExecutionRequest, files: Sequence[HFile], tests: Sequence[HTest]) -> RunSeries:
        spec = TreeSpec(request.task.merge_sha, (), overlay_of(request.human_overlay, request.kept, files))
        return self._runs.series(H_MERGE_STEP, spec, oracle_tests(tests))


class PerTaskHExecutor:
    """`HCandidateExecutor` that builds each task's executor once, from its repository and environment."""

    def __init__(self, executor_for: Callable[[Task], TreeHCandidateExecutor]) -> None:
        self._executor_for = executor_for
        self._executors: dict[str, TreeHCandidateExecutor] = {}

    def execute(self, request: HExecutionRequest) -> HExecution:
        task = request.task
        if task.task_id not in self._executors:
            self._executors[task.task_id] = self._executor_for(task)
        return self._executors[task.task_id].execute(request)
