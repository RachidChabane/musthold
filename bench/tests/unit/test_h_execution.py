from __future__ import annotations

from pathlib import Path

from specbench.adapters.go_file_bisection import BISECTION_RUNS, GoFileIsolation
from specbench.application.h_execution import PythonIsolation, TreeHCandidateExecutor
from specbench.application.ports import HExecutionRequest, TreeSpec
from specbench.application.stabilised_runs import StabilisedRuns
from specbench.domain.h_tests import HFile, HTest, TestKind
from specbench.domain.oracle import OracleTest, Outcome
from support.builders import BASE, MERGE, make_task
from support.fakes import ListJournal, RecordingTrees, ScriptedRunner

HUMAN = {"tests/test_u.py": b"def test_u():\n    pass\n"}
KEPT = HFile("tests/spec_h/test_spec_h_c3_kept.py", "def test_c3_a():\n    pass\n")


def h_test(path: str, name: str, number: int = 1) -> HTest:
    return HTest(f"{path}::{name}", number, TestKind.EXAMPLE, path, 0)


def test_python_files_that_do_not_collect_are_left_out_of_a_second_merge_run_and_of_base(tmp_path: Path) -> None:
    good = HFile("tests/spec_h/test_spec_h_c1_a.py", "def test_c1_a():\n    pass\n")
    broken = HFile("tests/spec_h/test_spec_h_c2_b.py", "import nothing\n")
    tests = (h_test(good.path, "test_c1_a"), h_test(broken.path, "test_c2_b", 2))

    def behaviour(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
        if test.test_id.startswith(broken.path):
            return Outcome.MISSING
        return Outcome.PASSED if spec.commit == MERGE else Outcome.FAILED

    trees = RecordingTrees(tmp_path)
    journal = ListJournal()
    executor = TreeHCandidateExecutor(
        StabilisedRuns(trees, ScriptedRunner(trees, behaviour), journal=journal), PythonIsolation()
    )
    execution = executor.execute(HExecutionRequest(make_task(), HUMAN, (KEPT,), (good, broken), tests))

    assert execution.unbuildable == frozenset({broken.path})
    first, rerun, base = trees.specs.values()
    assert (
        first.commit == MERGE
        and first.overlay is not None
        and set(first.overlay) == {*HUMAN, KEPT.path, good.path, broken.path}
    )
    assert rerun.commit == MERGE and rerun.overlay is not None and broken.path not in rerun.overlay
    assert base.commit == BASE and base.overlay is not None and set(base.overlay) == {*HUMAN, good.path}
    assert execution.runs[tests[0].test_id].merge == (Outcome.PASSED,) * 3
    assert execution.runs[tests[0].test_id].base == (Outcome.FAILED,) * 3
    assert (
        execution.runs[tests[1].test_id].merge == (Outcome.MISSING,) * 3 and execution.runs[tests[1].test_id].base == ()
    )
    assert [step for step, _, _ in journal.runs] == ["h-merge"] * 6 + ["h-base"] * 3


def test_a_go_package_that_does_not_build_is_bisected_file_by_file_and_based_one_file_at_a_time(tmp_path: Path) -> None:
    files = tuple(HFile(f"fee/h_c{n}_x_test.go", f"package fee_test\n// {n}\n") for n in (1, 2, 3))
    tests = tuple(h_test(f.path, f"TestH{n}_X", n) for n, f in zip((1, 2, 3), files, strict=True))
    for_go = tuple(HTest(f"fee::TestH{t.number}_X", t.number, t.kind, t.path, 0) for t in tests)

    def behaviour(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
        if spec.overlay is not None and files[1].path in spec.overlay and spec.commit == MERGE:
            return Outcome.MISSING
        return Outcome.PASSED if spec.commit == MERGE else Outcome.FAILED

    trees = RecordingTrees(tmp_path)
    runner = ScriptedRunner(trees, behaviour)
    isolation = GoFileIsolation(StabilisedRuns(trees, runner, BISECTION_RUNS))
    executor = TreeHCandidateExecutor(StabilisedRuns(trees, runner), isolation)
    execution = executor.execute(HExecutionRequest(make_task(), HUMAN, (), files, for_go))

    assert execution.unbuildable == frozenset({files[1].path})
    bases = [s for s in trees.specs.values() if s.commit == BASE]
    assert [sorted(set(s.overlay or {}) - set(HUMAN)) for s in bases] == [[files[0].path], [files[2].path]]
    assert execution.runs["fee::TestH1_X"].merge == (Outcome.PASSED,) * 3
    assert execution.runs["fee::TestH2_X"].base == ()
