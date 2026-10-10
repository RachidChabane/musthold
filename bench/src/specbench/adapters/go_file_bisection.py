"""`CandidateIsolation` for Go: a package whose H files do not build together is bisected by file, at merge only.

A Go package that does not build reports none of its tests, so one broken candidate file would take every candidate
of its package with it. The files of such a package are added one at a time, in path order, on top of the human test
files and the H files already kept; a file whose addition leaves every test of the group unreported does not build
and is rejected. At base a missing new symbol is the expected failure, so nothing is bisected there; each candidate
file has a base tree of its own, so a candidate's base outcome never depends on which other files survive.
"""

from __future__ import annotations

import posixpath
from collections.abc import Sequence
from typing import Final

from specbench.application.h_execution import oracle_tests, overlay_of
from specbench.application.ports import HExecutionRequest, TreeSpec
from specbench.application.stabilised_runs import RunSeries, StabilisedRuns
from specbench.domain.h_tests import HFile, HTest
from specbench.domain.oracle import Outcome

BISECTION_STEP: Final = "h-bisect"
BISECTION_RUNS: Final = 1
"""Whether a package builds does not vary between runs, so each bisection step runs once."""


def _package(path: str) -> str:
    return posixpath.dirname(path) or "."


class GoFileIsolation:
    def __init__(self, single_runs: StabilisedRuns) -> None:
        if single_runs.runs != BISECTION_RUNS:
            raise ValueError(f"bisection runs each step {BISECTION_RUNS} time")
        self._single = single_runs

    def unbuildable(self, request: HExecutionRequest, merge: RunSeries) -> frozenset[str]:
        tests_of = {f.path: [t for t in request.tests if t.path == f.path] for f in request.candidates}
        packages: dict[str, list[HFile]] = {}
        for file in request.candidates:
            packages.setdefault(_package(file.path), []).append(file)
        rejected: set[str] = set()
        for package, files in sorted(packages.items()):
            own = [t for f in files for t in tests_of[f.path]]
            if all(o is Outcome.MISSING for t in own for o in merge.of(t.test_id)):
                rejected |= self._bisect(request, package, sorted(files, key=lambda f: f.path), tests_of)
        return frozenset(rejected)

    def _bisect(
        self, request: HExecutionRequest, package: str, files: Sequence[HFile], tests_of: dict[str, list[HTest]]
    ) -> set[str]:
        accepted: list[HFile] = []
        rejected: set[str] = set()
        for file in files:
            group = [*accepted, file]
            tests = [t for f in group for t in tests_of[f.path]]
            spec = TreeSpec(request.task.merge_sha, (), overlay_of(request.human_overlay, request.kept, group))
            series = self._single.series(f"{BISECTION_STEP} {package}", spec, oracle_tests(tests))
            if all(o is Outcome.MISSING for t in tests for o in series.of(t.test_id)):
                rejected.add(file.path)
            else:
                accepted.append(file)
        return rejected

    def base_groups(self, files: Sequence[HFile]) -> tuple[tuple[HFile, ...], ...]:
        return tuple((f,) for f in files)
