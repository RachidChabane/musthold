from __future__ import annotations

import shutil

import pytest

from pilot.adapters.ast_edits import AstEditApplier
from pilot.adapters.plain_pytest import PlainPytestRunner
from pilot.adapters.prereg_files import load_edit_sets, load_refactor
from pilot.adapters.process import SubprocessRunner
from pilot.adapters.project_copy import ProjectWorkspace, copy_project
from pilot.adapters.pytest_runner import PytestJunitRunner, WorkspaceVariantRunner
from pilot.application.ports import RunRequest
from pilot.domain.outcomes import TestOutcome
from pilot.domain.variants import VariantKind

pytestmark = pytest.mark.integration
FIRST = "tests/test_mini.py::test_raises_when_exit_before_entry"


@pytest.fixture
def workspace(mini_root, tmp_path):
    return ProjectWorkspace(mini_root / "fixture" / "parkfee", tmp_path / "work", AstEditApplier())


def test_junit_runner_returns_outcomes_keyed_by_node_id(workspace):
    runner = WorkspaceVariantRunner(workspace, PytestJunitRunner(SubprocessRunner()).run)
    result = runner.run_many({"ref": RunRequest()})["ref"]
    assert len(result.outcomes) == 8 and result.all_passed
    assert FIRST in result.outcomes


def test_junit_runner_applies_edit_sets_and_scopes_to_node_ids(workspace, mini_root):
    violations = load_edit_sets(mini_root / "prereg" / "violations", VariantKind.VIOLATION)
    runner = WorkspaceVariantRunner(workspace, PytestJunitRunner(SubprocessRunner()).run)
    result = runner.run_many({"v": RunRequest(violations["PARK-1_a"], (FIRST,))})["v"]
    assert result.outcomes == {FIRST: TestOutcome.FAILED}
    assert result.returncode == 1


def test_many_variants_run_concurrently_without_mixing_results(workspace, mini_root):
    violations = load_edit_sets(mini_root / "prereg" / "violations", VariantKind.VIOLATION)
    refactor = load_refactor(mini_root / "prereg" / "refactor")
    requests = {name: RunRequest(edits) for name, edits in {**violations, **refactor}.items()}
    results = WorkspaceVariantRunner(workspace, PytestJunitRunner(SubprocessRunner()).run).run_many(requests)
    assert results["PARK-1_a"].outcomes[FIRST] is TestOutcome.FAILED
    assert results["PARK-3_a"].outcomes[FIRST] is TestOutcome.PASSED
    assert results["R"].outcomes["tests/test_mini.py::test_quote_keeps_two_decimal_places"] is TestOutcome.FAILED


def test_the_fixture_directory_is_never_written_to(workspace, mini_root):
    before = sorted(p.relative_to(mini_root) for p in mini_root.rglob("*"))
    WorkspaceVariantRunner(workspace, PytestJunitRunner(SubprocessRunner()).run).run_many({"ref": RunRequest()})
    assert sorted(p.relative_to(mini_root) for p in mini_root.rglob("*")) == before


def test_copies_are_removed_after_use(workspace, tmp_path):
    WorkspaceVariantRunner(workspace, PytestJunitRunner(SubprocessRunner()).run).run_many({"ref": RunRequest()})
    assert list((tmp_path / "work").iterdir()) == []


def test_copy_project_skips_caches(mini_root, tmp_path):
    source = tmp_path / "src_tree"
    shutil.copytree(mini_root / "fixture" / "parkfee", source)
    (source / ".hypothesis").mkdir()
    (source / "tests" / "__pycache__").mkdir(exist_ok=True)
    copy_project(source, tmp_path / "copy")
    assert not (tmp_path / "copy" / ".hypothesis").exists()
    assert not (tmp_path / "copy" / "tests" / "__pycache__").exists()
    assert (tmp_path / "copy" / "tests" / "test_mini.py").is_file()


def test_plain_runner_copies_extra_files_and_reads_the_summary(workspace, mini_root):
    oracle_files = sorted((mini_root / "prereg" / "oracle").glob("test_oracle_park_*.py"))
    runner = WorkspaceVariantRunner(workspace, PlainPytestRunner(SubprocessRunner(), oracle_files))
    outcomes = runner.run_many({"ref": RunRequest()})["ref"].outcomes
    assert len(outcomes) == 8 + 4 + 1 + 1 + 1
    assert all(o is TestOutcome.PASSED for o in outcomes.values())
    assert any("test_oracle_park_1.py::test_oracle_raises_for_earlier_exit[1]" in n for n in outcomes)
