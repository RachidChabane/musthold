from __future__ import annotations

import sys
from collections.abc import Mapping, Sequence
from pathlib import Path

import pytest

from specbench.adapters.file_trees import DiskFileTreeFactory
from specbench.adapters.git_trees import GitTreeFactory
from specbench.adapters.oracle_runners import PytestOracleRunner
from specbench.adapters.process import MemoryFloorRunner, SubprocessRunner, system_free_percent
from specbench.adapters.workspace_repo import DiskTreeReader
from specbench.application.ports import TreeSpec
from specbench.application.score_oracles import OracleScorer, scoring_basis
from specbench.domain.oracle import OracleTest, Outcome, oracle_from_bundle
from specbench.domain.records import Truth
from specbench.domain.scoring import SCORING_RUNS
from support.upstream import GOOD_V1, bundle, make_upstream, task

pytestmark = pytest.mark.integration

FEE_PATH = "pkg/fee.py"
AGENT_TEST_PATH = "tests/test_agent.py"
ALWAYS_PASSING_TEST = b"def test_agent():\n    assert True\n"
FEE_RETURNING_FLOAT = "def fee(minutes):\n    if minutes < 10:\n        return 0.0\n    return 5\n"
FEE_FREE_EVERYWHERE = "def fee(minutes):\n    return 0\n"
FEE_NEVER_FREE = "def fee(minutes):\n    return 5\n"


class CountingRunner:
    def __init__(self, inner: PytestOracleRunner) -> None:
        self._inner = inner
        self.calls = 0

    def run(self, tree: Path, tests: Sequence[OracleTest]) -> Mapping[str, Outcome]:
        self.calls += 1
        return self._inner.run(tree, tests)


@pytest.fixture
def merge_tree(tmp_path: Path) -> tuple[OracleScorer, CountingRunner, dict[str, bytes]]:
    repo, base, merge = make_upstream(tmp_path)
    sealed = bundle(GOOD_V1)
    trees = GitTreeFactory(repo)
    reader = DiskTreeReader()
    basis = scoring_basis(trees, reader, task(base, merge), oracle_from_bundle(sealed), sealed)
    runner = CountingRunner(
        PytestOracleRunner(Path(sys.executable), MemoryFloorRunner(SubprocessRunner(), system_free_percent))
    )
    with trees.materialize(TreeSpec(merge)) as tree:
        agent_files = reader.read(tree)
    return OracleScorer(basis, DiskFileTreeFactory(tmp_path), runner), runner, agent_files


def with_fee(files: Mapping[str, bytes], source: str) -> dict[str, bytes]:
    return {**files, FEE_PATH: source.encode()}


def test_the_upstream_change_is_clean(merge_tree: tuple[OracleScorer, CountingRunner, dict[str, bytes]]) -> None:
    scorer, _, merge_files = merge_tree
    assert scorer.score(merge_files) == Truth(
        defective=False, defective_full=False, o_failed=False, defective_full_unexposed=False
    )


def test_a_change_that_breaks_a_u_test_is_defective(
    merge_tree: tuple[OracleScorer, CountingRunner, dict[str, bytes]],
) -> None:
    scorer, _, merge_files = merge_tree
    assert scorer.score(with_fee(merge_files, FEE_NEVER_FREE)) == Truth(True, True, False, True)


def test_a_change_that_breaks_a_pre_existing_test_is_defective(
    merge_tree: tuple[OracleScorer, CountingRunner, dict[str, bytes]],
) -> None:
    scorer, _, merge_files = merge_tree
    assert scorer.score(with_fee(merge_files, FEE_FREE_EVERYWHERE)).defective


def test_a_failure_of_an_out_of_spec_test_alone_is_not_a_defect(
    merge_tree: tuple[OracleScorer, CountingRunner, dict[str, bytes]],
) -> None:
    scorer, _, merge_files = merge_tree
    assert scorer.score(with_fee(merge_files, FEE_RETURNING_FLOAT)) == Truth(False, False, True, False)


def test_the_agents_own_tests_and_edits_to_upstream_tests_change_nothing(
    merge_tree: tuple[OracleScorer, CountingRunner, dict[str, bytes]],
) -> None:
    scorer, runner, merge_files = merge_tree
    weakened = with_fee(merge_files, FEE_NEVER_FREE)
    weakened["tests/test_r.py"] = b"def test_r1():\n    assert True\n"
    weakened[AGENT_TEST_PATH] = ALWAYS_PASSING_TEST
    assert scorer.score(weakened) == Truth(True, True, False, True)
    assert scorer.score(with_fee(merge_files, FEE_NEVER_FREE)) == Truth(True, True, False, True)
    assert runner.calls == SCORING_RUNS
    assert scorer.scored_tree_count == 1
