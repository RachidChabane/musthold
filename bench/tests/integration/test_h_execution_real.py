"""H candidates executed for real: git trees, pytest and `go test`, then selected by verification's own rules."""

from __future__ import annotations

import shutil
import sys
from pathlib import Path

import pytest

from specbench.adapters.git_trees import GitTreeFactory
from specbench.adapters.go_file_bisection import BISECTION_RUNS, GoFileIsolation
from specbench.adapters.oracle_runners import GoTestOracleRunner, PytestOracleRunner
from specbench.adapters.process import MemoryFloorRunner, SubprocessRunner, system_free_percent
from specbench.application.h_execution import PythonIsolation, TreeHCandidateExecutor
from specbench.application.ports import HExecutionRequest
from specbench.application.stabilised_runs import StabilisedRuns
from specbench.domain.h_selection import Verdict, select_h
from specbench.domain.h_tests import HFile, HWriting, h_layout_of, h_problems
from specbench.domain.languages import Language
from specbench.domain.oracle import Outcome
from support.builders import make_task
from support.tiny_upstream import denied_runner
from support.upstream import commit_all, git
from support.upstream import make_upstream as make_python_upstream

pytestmark = pytest.mark.integration
SEED = 20261007
GO_MODULE = "example.com/tiny"
GO_ENVIRONMENT = {"GOFLAGS": "-mod=mod", "GOPROXY": "off", "GOTOOLCHAIN": "local"}
PY_HEADER = "from hypothesis import given, settings, strategies as st\nfrom pkg.fee import fee\n\n\n"
PY_PROPERTY = (
    "@settings(derandomize=True, max_examples=20, deadline=None)\n@given(st.integers(min_value=1, max_value=9))\n"
    "def test_c{n}_short_stays_are_free(m):\n    assert fee(m) == 0\n\n\n"
)


def python_file(number: int, examples: str) -> HFile:
    body = (PY_HEADER + PY_PROPERTY + examples).replace("{n}", str(number))
    return HFile(f"tests/spec_h/test_spec_h_c{number}_fee.py", body)


def test_python_candidates_are_judged_at_merge_and_base_as_verification_judges_them(tmp_path: Path) -> None:
    repo, base, merge = make_python_upstream(tmp_path)
    task = make_task(base_sha=base, merge_sha=merge)
    good = python_file(
        1,
        "def test_c1_nine_minutes_are_free():\n    assert fee(9) == 0\n\n\n"
        "def test_c1_ten_minutes_are_paid():\n    assert fee(10) == 0\n\n\n"
        "def test_c1_an_hour_costs_five():\n    assert fee(60) == 5\n",
    )
    broken = HFile(
        "tests/spec_h/test_spec_h_c2_fee.py",
        "from pkg.fee import fee_that_does_not_exist\n\n\ndef test_c2_a():\n    pass\n",
    )
    layout = h_layout_of(Language.PYTHON, "", ["pkg.fee"])
    _, tests = h_problems(layout, HWriting((1, 2)), [good, broken])
    runs = StabilisedRuns(GitTreeFactory(repo), PytestOracleRunner(Path(sys.executable), denied_runner()))
    executor = TreeHCandidateExecutor(runs, PythonIsolation())
    execution = executor.execute(HExecutionRequest(task, {}, (), (good, broken), tuple(tests)))
    assert execution.unbuildable == frozenset({broken.path})
    selection = select_h(tests, execution, (1, 2), frozenset({1, 2}), SEED, task.task_id)
    verdicts = {test_id.split("::")[1]: verdict for test_id, verdict in selection.verdicts.items()}
    assert verdicts == {
        "test_c1_short_stays_are_free": Verdict.KEPT,
        "test_c1_nine_minutes_are_free": Verdict.KEPT,
        "test_c1_ten_minutes_are_paid": Verdict.REJECTED_MERGE,
        "test_c1_an_hour_costs_five": Verdict.REJECTED_BASE_PASS,
        "test_c2_a": Verdict.REJECTED_FILE,
    }
    ten = next(t for t in tests if t.test_id.endswith("ten_minutes_are_paid"))
    assert execution.runs[ten.test_id].merge == (Outcome.FAILED,) * 3


GO_BASE = "package fee\n\nfunc Fee(minutes int) int { return 5 }\n"
GO_MERGE = (
    "package fee\n\nfunc Fee(minutes int) int {\n\tif Free(minutes) {\n\t\treturn 0\n\t}\n\treturn 5\n}\n\n"
    "func Free(minutes int) bool { return minutes < 10 }\n"
)


def make_go_upstream(root: Path) -> tuple[Path, str, str]:
    repo = root / "go-upstream"
    (repo / "fee").mkdir(parents=True)
    (repo / "go.mod").write_text(f"module {GO_MODULE}\n\ngo 1.21\n")
    (repo / "fee" / "fee.go").write_text(GO_BASE)
    git(repo, "init", "-q", "-b", "main")
    base = commit_all(repo, "base")
    (repo / "fee" / "fee.go").write_text(GO_MERGE)
    merge = commit_all(repo, "merge")
    return repo, base, merge


def go_file(name: str, body: str) -> HFile:
    header = f'package fee_test\n\nimport (\n\t"testing"\n\n\t"{GO_MODULE}/fee"\n)\n\n'
    return HFile(f"fee/{name}", header + body)


@pytest.mark.skipif(shutil.which("go") is None, reason="go is not installed")
def test_a_go_package_that_does_not_build_is_bisected_by_file_and_a_new_symbol_fails_only_at_base(
    tmp_path: Path,
) -> None:
    repo, base, merge = make_go_upstream(tmp_path)
    task = make_task(language=Language.GO, base_sha=base, merge_sha=merge)
    old_symbol = go_file(
        "h_c1_fee_test.go",
        "func TestH1_ShortStayIsFree(t *testing.T) {\n\tif fee.Fee(5) != 0 {\n\t\tt.Fatal(fee.Fee(5))\n\t}\n}\n\n"
        "func TestH1_LongStayCostsFive(t *testing.T) {\n\tif fee.Fee(60) != 5 {\n\t\tt.Fatal(fee.Fee(60))\n\t}\n}\n",
    )
    new_symbol = go_file(
        "h_c2_free_test.go",
        "func TestH2_NineMinutesAreFree(t *testing.T) {\n\tif !fee.Free(9) {\n\t\tt.Fatal(9)\n\t}\n}\n",
    )
    broken = go_file("h_c3_broken_test.go", "func TestH3_Never(t *testing.T) {\n\t_ = fee.Nope()\n}\n")
    layout = h_layout_of(Language.GO, GO_MODULE, [f"{GO_MODULE}/fee"])
    files = (old_symbol, new_symbol, broken)
    _, tests = h_problems(layout, HWriting((1, 2, 3)), files)
    runner = GoTestOracleRunner(
        MemoryFloorRunner(SubprocessRunner(), system_free_percent),
        {**GO_ENVIRONMENT, "GOMODCACHE": str(tmp_path / "modcache")},
    )
    trees = GitTreeFactory(repo)
    executor = TreeHCandidateExecutor(
        StabilisedRuns(trees, runner), GoFileIsolation(StabilisedRuns(trees, runner, BISECTION_RUNS))
    )
    execution = executor.execute(HExecutionRequest(task, {}, (), files, tuple(tests)))
    assert execution.unbuildable == frozenset({broken.path})
    runs = {t.test_id: execution.runs[t.test_id] for t in tests}
    assert runs["fee::TestH2_NineMinutesAreFree"].base == (Outcome.MISSING,) * 3
    selection = select_h(tests, execution, (1, 2, 3), frozenset({1, 2, 3}), SEED, task.task_id)
    assert selection.verdicts == {
        "fee::TestH1_LongStayCostsFive": Verdict.REJECTED_BASE_PASS,
        "fee::TestH1_ShortStayIsFree": Verdict.KEPT,
        "fee::TestH2_NineMinutesAreFree": Verdict.KEPT,
        "fee::TestH3_Never": Verdict.REJECTED_FILE,
    }
