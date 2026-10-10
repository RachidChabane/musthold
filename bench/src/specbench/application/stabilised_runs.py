"""The stabilised multi-run of verification: one tree, `runs` runs of the same tests, folded by the verifier's rule.

Verification and the selection of H share this one component, so a test that construction keeps was judged exactly as
verification judges it: same trees, same runner, same number of runs, same flake rule.
"""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from pathlib import Path

from specbench.application.ports import OracleRunner, RunJournal, TreeFactory, TreeSpec
from specbench.domain.oracle import OracleTest, Outcome
from specbench.domain.verification import VERIFICATION_RUNS, stabilise


class SilentJournal:
    """A `RunJournal` that keeps nothing."""

    def record(self, step: str, run: int, outcomes: Mapping[str, Outcome]) -> None:
        return None


@dataclass(frozen=True)
class RunSeries:
    """Every run of one step, and the stable outcome of each test across them."""

    runs: tuple[Mapping[str, Outcome], ...]
    stable: dict[str, Outcome]

    def of(self, test_id: str) -> tuple[Outcome, ...]:
        """The outcome of one test in each run; a run that did not report it saw it MISSING."""
        return tuple(run.get(test_id, Outcome.MISSING) for run in self.runs)


@dataclass(frozen=True)
class StabilisedRuns:
    """Materializes a tree once, runs the tests on it `runs` times, journals each run and folds them with `stabilise`."""

    trees: TreeFactory
    runner: OracleRunner
    runs: int = VERIFICATION_RUNS
    journal: RunJournal = field(default_factory=SilentJournal)

    def series(self, step: str, spec: TreeSpec, tests: Sequence[OracleTest]) -> RunSeries:
        with self.trees.materialize(spec) as tree:
            runs = tuple(self._run(step, run, tree, tests) for run in range(1, self.runs + 1))
        return RunSeries(runs, stabilise(runs, [t.test_id for t in tests]))

    def _run(self, step: str, run: int, tree: Path, tests: Sequence[OracleTest]) -> Mapping[str, Outcome]:
        outcomes = self.runner.run(tree, tests)
        self.journal.record(step, run, outcomes)
        return outcomes
