from __future__ import annotations

from collections.abc import Generator, Mapping, Sequence
from contextlib import contextmanager
from pathlib import Path

import pytest

from specbench.application.score_oracles import OracleScorer, ScoringBasis
from specbench.domain.languages import Language
from specbench.domain.oracle import Oracle, OracleTest, Outcome, Part
from specbench.domain.records import Truth
from specbench.domain.scoring import (
    FAILURE_QUORUM,
    SCORING_RUNS,
    OracleTreeCollision,
    compose_oracle_tree,
    settle,
    settled_outcome,
    truth_of,
)
from specbench.domain.sealing import tree_digest

PASSED, FAILED, ERROR, MISSING, FLAKY = Outcome.PASSED, Outcome.FAILED, Outcome.ERROR, Outcome.MISSING, Outcome.FLAKY
U_TEST, R_TEST, H_TEST, O_TEST = "tests/test_u.py::test_u", "tests/test_r.py::test_r", "t_h::test_h", "t_o::test_o"
PART_TEST = {Part.U: U_TEST, Part.R: R_TEST, Part.H: H_TEST, Part.O: O_TEST}
ORACLE = Oracle(
    (
        OracleTest(U_TEST, Part.U, "T-1"),
        OracleTest(R_TEST, Part.R, None),
        OracleTest(H_TEST, Part.H, "T-1"),
        OracleTest(O_TEST, Part.O, None),
    ),
    (),
    (),
)


def outcomes_with(changes: Mapping[Part, Outcome]) -> dict[str, Outcome]:
    outcomes = dict.fromkeys(PART_TEST.values(), PASSED)
    for part, outcome in changes.items():
        outcomes[PART_TEST[part]] = outcome
    return outcomes


def test_the_agents_test_files_never_reach_the_oracle_tree() -> None:
    tree = compose_oracle_tree(
        Language.PYTHON,
        {"pkg/fee.py": b"agent", "tests/test_agent.py": b"mine", "pkg/test_inline.py": b"mine", "conftest.py": b"mine"},
        {},
        {},
    )
    assert tree == {"pkg/fee.py": b"agent"}


def test_upstream_tests_overwrite_the_agents_copies_and_come_back_when_deleted() -> None:
    tree = compose_oracle_tree(
        Language.PYTHON,
        {"pkg/fee.py": b"agent", "tests/test_r.py": b"weakened"},
        {"tests/test_r.py": b"upstream r", "tests/test_gone.py": b"upstream gone"},
        {},
    )
    assert tree == {"pkg/fee.py": b"agent", "tests/test_r.py": b"upstream r", "tests/test_gone.py": b"upstream gone"}


def test_sealed_files_are_overlaid_last_and_the_agents_source_is_otherwise_kept() -> None:
    tree = compose_oracle_tree(
        Language.PYTHON,
        {"pkg/new.py": b"agent new"},
        {"tests/test_u.py": b"u"},
        {"tests/test_h.py": b"sealed h", "tests/test_o.py": b"sealed o"},
    )
    assert tree == {
        "pkg/new.py": b"agent new",
        "tests/test_u.py": b"u",
        "tests/test_h.py": b"sealed h",
        "tests/test_o.py": b"sealed o",
    }


@pytest.mark.parametrize(
    ("agent_files", "colliding"),
    [
        ({"pkg/helper.py": b"agent helper"}, ["pkg/helper.py"]),
        ({"tests/spec_h/test_h.py": b"agent h", "pkg/fee.py": b"x"}, ["tests/spec_h/test_h.py"]),
        ({"pkg/helper.py": b"a", "tests/spec_h/test_h.py": b"b"}, ["pkg/helper.py", "tests/spec_h/test_h.py"]),
    ],
)
def test_a_sealed_path_that_would_overwrite_a_file_of_the_agents_tree_is_an_error(
    agent_files: Mapping[str, bytes], colliding: list[str]
) -> None:
    overlay = {"pkg/helper.py": b"sealed o", "tests/spec_h/test_h.py": b"sealed h"}
    with pytest.raises(OracleTreeCollision) as raised:
        compose_oracle_tree(Language.PYTHON, agent_files, {}, overlay)
    assert str(raised.value).endswith(", ".join(colliding))


def test_the_sealed_copy_of_an_upstream_test_replaces_the_agents_version_by_design() -> None:
    tree = compose_oracle_tree(
        Language.PYTHON,
        {"tests/test_u.py": b"weakened by the agent", "pkg/fee.py": b"agent"},
        {"tests/test_u.py": b"upstream"},
        {"tests/test_u.py": b"upstream"},
    )
    assert tree == {"pkg/fee.py": b"agent", "tests/test_u.py": b"upstream"}


def test_go_test_files_and_testdata_are_the_tests_of_a_go_tree() -> None:
    tree = compose_oracle_tree(
        Language.GO,
        {"fee.go": b"agent", "fee_test.go": b"mine", "testdata/golden.txt": b"mine"},
        {"fee_test.go": b"upstream"},
        {},
    )
    assert tree == {"fee.go": b"agent", "fee_test.go": b"upstream"}


def test_tree_digest_follows_the_content_not_the_insertion_order() -> None:
    first = {"a.py": b"1", "b.py": b"2"}
    assert tree_digest(first) == tree_digest({"b.py": b"2", "a.py": b"1"})
    assert tree_digest(first) != tree_digest({"a.py": b"1", "b.py": b"3"})
    assert tree_digest(first) != tree_digest({"a.py": b"1"})


@pytest.mark.parametrize(
    ("runs", "settled"),
    [
        ([PASSED, PASSED, PASSED], PASSED),
        ([FAILED, FAILED, FAILED], FAILED),
        ([FAILED, FAILED, PASSED], FAILED),
        ([PASSED, FAILED, FAILED], FAILED),
        ([FAILED, PASSED, PASSED], FLAKY),
        ([PASSED, PASSED, MISSING], FLAKY),
        ([ERROR, PASSED, ERROR], ERROR),
        ([MISSING, MISSING, PASSED], MISSING),
        ([FAILED, ERROR, PASSED], FAILED),
        ([ERROR, MISSING, PASSED], ERROR),
    ],
)
def test_a_test_failing_in_two_of_three_runs_fails_and_in_one_is_flaky(runs: list[Outcome], settled: Outcome) -> None:
    assert FAILURE_QUORUM == 2 and SCORING_RUNS == 3
    assert settled_outcome(runs) is settled


def test_a_test_a_run_does_not_report_is_missing_in_that_run() -> None:
    runs = [{U_TEST: PASSED}, {U_TEST: PASSED}, {}]
    assert settle(runs, [U_TEST, R_TEST]) == {U_TEST: FLAKY, R_TEST: MISSING}


def test_a_clean_change_has_no_defect_of_any_kind() -> None:
    assert truth_of(ORACLE, outcomes_with({})) == Truth(False, False, False)


@pytest.mark.parametrize(
    ("changes", "truth"),
    [
        ({Part.U: FAILED}, Truth(True, True, False)),
        ({Part.R: ERROR}, Truth(True, True, False)),
        ({Part.H: FAILED}, Truth(False, True, False)),
        ({Part.O: FAILED}, Truth(False, False, True)),
        ({Part.U: FLAKY, Part.R: FLAKY, Part.H: FLAKY, Part.O: FLAKY}, Truth(False, False, False)),
        ({Part.U: MISSING}, Truth(True, True, False)),
    ],
)
def test_each_part_feeds_its_own_truth(changes: dict[Part, Outcome], truth: Truth) -> None:
    assert truth_of(ORACLE, outcomes_with(changes)) == truth


class MemoryFileTrees:
    def __init__(self, root: Path) -> None:
        self._root = root
        self.materialized: list[dict[str, bytes]] = []

    @contextmanager
    def materialize_files(self, files: Mapping[str, bytes]) -> Generator[Path]:
        self.materialized.append(dict(files))
        directory = self._root / f"tree-{len(self.materialized)}"
        directory.mkdir()
        yield directory


class CountingRunner:
    def __init__(self) -> None:
        self.calls = 0

    def run(self, tree: Path, tests: Sequence[OracleTest]) -> Mapping[str, Outcome]:
        self.calls += 1
        return {t.test_id: PASSED for t in tests}


def scorer(tmp_path: Path, runner: CountingRunner) -> tuple[OracleScorer, MemoryFileTrees]:
    trees = MemoryFileTrees(tmp_path)
    basis = ScoringBasis(Language.PYTHON, ORACLE, {"tests/test_u.py": b"u"}, {"tests/test_h.py": b"h"})
    return OracleScorer(basis, trees, runner), trees


def test_an_identical_oracle_tree_is_scored_once(tmp_path: Path) -> None:
    runner = CountingRunner()
    subject, trees = scorer(tmp_path, runner)
    first = subject.score({"pkg/fee.py": b"a", "tests/test_mine.py": b"one"})
    second = subject.score({"pkg/fee.py": b"a", "tests/test_mine.py": b"two", "tests/test_other.py": b"x"})
    assert first == second
    assert runner.calls == SCORING_RUNS
    assert len(trees.materialized) == 1
    assert subject.scored_tree_count == 1


def test_a_different_oracle_tree_is_scored_again(tmp_path: Path) -> None:
    runner = CountingRunner()
    subject, _ = scorer(tmp_path, runner)
    subject.score({"pkg/fee.py": b"a"})
    subject.score({"pkg/fee.py": b"b"})
    assert runner.calls == 2 * SCORING_RUNS
    assert subject.scored_tree_count == 2


def test_a_final_tree_that_holds_a_sealed_path_is_not_scored_and_runs_nothing(tmp_path: Path) -> None:
    runner = CountingRunner()
    subject, trees = scorer(tmp_path, runner)
    with pytest.raises(OracleTreeCollision, match=r"tests/test_h\.py"):
        subject.score({"pkg/fee.py": b"a", "tests/test_h.py": b"the agent wrote a file at the sealed path"})
    assert runner.calls == 0
    assert trees.materialized == []
    assert subject.scored_tree_count == 0
