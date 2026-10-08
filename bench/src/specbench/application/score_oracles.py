"""`score_oracles`: the defect truth of an arm's final tree, by running the oracle three times on the oracle tree."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass

from specbench.application.ports import FileTreeFactory, OracleRunner, TreeFactory, TreeReader, TreeSpec
from specbench.domain.languages import Language, is_test_path
from specbench.domain.oracle import OVERLAID_PARTS, Oracle
from specbench.domain.records import Truth
from specbench.domain.scoring import SCORING_RUNS, compose_oracle_tree, settle, truth_of
from specbench.domain.sealing import SealedBundle, tree_digest
from specbench.domain.task import Task


@dataclass(frozen=True)
class ScoringBasis:
    """What every oracle tree of one task shares: its oracle, the upstream tests at merge and the sealed files."""

    language: Language
    oracle: Oracle
    upstream_tests: Mapping[str, bytes]
    sealed_overlay: Mapping[str, bytes]


def scoring_basis(
    trees: TreeFactory, reader: TreeReader, task: Task, oracle: Oracle, bundle: SealedBundle
) -> ScoringBasis:
    """The test files of the merge commit and the bundle's U, H and O files."""
    with trees.materialize(TreeSpec(task.merge_sha)) as merge:
        upstream = {path: content for path, content in reader.read(merge).items() if is_test_path(task.language, path)}
    overlay = {
        destination: bundle.files[source]
        for destination, source in oracle.overlay_paths(bundle, *OVERLAID_PARTS).items()
    }
    return ScoringBasis(task.language, oracle, upstream, overlay)


class OracleScorer:
    """Scores the final trees of one task; a tree whose oracle tree was scored before is not run again."""

    def __init__(
        self, basis: ScoringBasis, trees: FileTreeFactory, runner: OracleRunner, runs: int = SCORING_RUNS
    ) -> None:
        self._basis = basis
        self._trees = trees
        self._runner = runner
        self._runs = runs
        self._scored: dict[str, Truth] = {}

    @property
    def scored_tree_count(self) -> int:
        """How many distinct oracle trees were run."""
        return len(self._scored)

    def score(self, agent_files: Mapping[str, bytes]) -> Truth:
        basis = self._basis
        files = compose_oracle_tree(basis.language, agent_files, basis.upstream_tests, basis.sealed_overlay)
        digest = tree_digest(files)
        if digest not in self._scored:
            self._scored[digest] = self._run(files)
        return self._scored[digest]

    def _run(self, files: Mapping[str, bytes]) -> Truth:
        tests = self._basis.oracle.tests
        with self._trees.materialize_files(files) as tree:
            runs = [self._runner.run(tree, tests) for _ in range(self._runs)]
        return truth_of(self._basis.oracle, settle(runs, [t.test_id for t in tests]))
