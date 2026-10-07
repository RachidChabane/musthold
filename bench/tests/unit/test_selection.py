from __future__ import annotations

from datetime import date
from pathlib import Path

from specbench.adapters.selection_store import read_attrition, write_selection
from specbench.application.select_tasks import select_feasible
from specbench.domain.candidates import Candidate, ranking_by_language
from specbench.domain.changed_tests import added_lines, touched_by_diff, without_parameters
from specbench.domain.ground_tests import GroundTests, partition_ground_tests
from specbench.domain.languages import Language
from specbench.domain.mining import Rejection, Stage
from specbench.domain.oracle import Outcome
from specbench.domain.selection import ProbedTask
from specbench.domain.task import Toolchain, parse_task

SEED = 20261007

PYTHON_TESTS = """import pytest


def test_old():
    assert True


@pytest.mark.parametrize("n", [1, 2])
def test_new(n):
    assert n


class TestThing:
    def test_inside(self):
        assert True

    def test_untouched(self):
        assert True
"""

PYTHON_DIFF = """diff --git a/tests/test_a.py b/tests/test_a.py
--- a/tests/test_a.py
+++ b/tests/test_a.py
@@ -5,0 +6,3 @@
+@pytest.mark.parametrize("n", [1, 2])
+def test_new(n):
+    assert n
@@ -12 +14 @@
-        assert False
+        assert True
"""

GO_TESTS = """package fee

import "testing"

func TestOld(t *testing.T) {
	t.Log("old")
}

func TestNew(t *testing.T) {
	if Fee(5) != 0 {
		t.Fatal("no")
	}
}
"""

GO_DIFF = """diff --git a/fee/fee_test.go b/fee/fee_test.go
--- a/fee/fee_test.go
+++ b/fee/fee_test.go
@@ -9,0 +10,3 @@
+	if Fee(5) != 0 {
+		t.Fatal("no")
+	}
"""


def candidate(repository: str, language: Language = Language.PYTHON, number: int = 1) -> Candidate:
    return Candidate(
        repository=repository,
        language=language,
        stars=500,
        spdx_id="MIT",
        pull_number=number,
        title="feat: add a thing",
        merged_on=date(2026, 8, 1),
        base_sha="a" * 40,
        merge_sha="b" * 40,
        non_test_files=("pkg/a.py",),
        non_test_lines=40,
        test_files=("tests/test_a.py",),
        licence_path="LICENSE",
        notice_path=None,
    )


def probed(c: Candidate) -> ProbedTask:
    return ProbedTask(c, "c" * 40, Toolchain("python", "3.13.14"), {"uv.lock": "d" * 64}, 2, 5, 9, "e" * 64)


def test_added_lines_follow_the_new_file_numbering() -> None:
    assert added_lines(PYTHON_DIFF) == {"tests/test_a.py": frozenset({6, 7, 8, 14})}


def test_python_tests_are_touched_by_their_decorators_body_and_class() -> None:
    touched = touched_by_diff(Language.PYTHON, {"tests/test_a.py": PYTHON_TESTS}, added_lines(PYTHON_DIFF))
    assert touched == {"tests/test_a.py::test_new", "tests/test_a.py::TestThing::test_inside"}


def test_go_tests_are_touched_by_a_line_inside_their_body() -> None:
    touched = touched_by_diff(Language.GO, {"fee/fee_test.go": GO_TESTS}, added_lines(GO_DIFF))
    assert touched == {"fee::TestNew"}


def test_parametrized_ids_match_on_the_name_before_the_bracket() -> None:
    assert without_parameters("tests/test_a.py::test_new[1-x]") == "tests/test_a.py::test_new"


def test_fail_to_pass_needs_a_touched_test_that_fails_at_base_and_passes_at_merge() -> None:
    touched = frozenset({"t.py::test_new", "t.py::test_edit"})
    merge = {
        "t.py::test_new[1]": Outcome.PASSED,
        "t.py::test_new[2]": Outcome.PASSED,
        "t.py::test_edit": Outcome.PASSED,
        "t.py::test_old": Outcome.PASSED,
        "t.py::test_broken": Outcome.FAILED,
        "t.py::test_gone": Outcome.PASSED,
    }
    base_with_tests = {
        "t.py::test_edit": Outcome.PASSED,
        "t.py::test_old": Outcome.PASSED,
        "t.py::test_broken": Outcome.PASSED,
    }
    ground = partition_ground_tests(touched, merge, base_with_tests)
    assert ground.u == ("t.py::test_new[1]", "t.py::test_new[2]")
    assert ground.r == ("t.py::test_edit", "t.py::test_old")
    assert (ground.merge_passed, ground.merge_total, ground.merge_pass_percent) == (5, 6, 83)
    assert GroundTests((), (), 0, 0).merge_pass_percent == 0


class ScriptedProbe:
    def __init__(self, rejected: dict[str, Stage]) -> None:
        self._rejected = rejected
        self.probed: list[str] = []

    def probe(self, candidate: Candidate) -> ProbedTask | Rejection:
        self.probed.append(candidate.label)
        stage = self._rejected.get(candidate.repository)
        return Rejection(stage, "scripted") if stage else probed(candidate)


def pool() -> list[Candidate]:
    return [candidate(f"acme/py{i}") for i in range(9)] + [candidate(f"acme/go{i}", Language.GO) for i in range(6)]


def test_a_rejected_candidate_is_replaced_by_the_next_in_seeded_order() -> None:
    ranking = ranking_by_language(SEED, pool())
    first_python = ranking[Language.PYTHON][0].repository
    probe = ScriptedProbe({first_python: Stage.OFFLINE_TESTS})
    selection = select_feasible(ranking, probe, 3, 2)
    python = [t for t in (*selection.tasks, *selection.reserves) if t.candidate.language is Language.PYTHON]
    assert first_python not in {t.candidate.repository for t in python}
    assert [t.candidate.label for t in selection.tasks if t.candidate.language is Language.PYTHON] == [
        c.label for c in ranking[Language.PYTHON][1:4]
    ]
    assert [t.candidate.label for t in selection.reserves if t.candidate.language is Language.PYTHON] == [
        c.label for c in ranking[Language.PYTHON][4:6]
    ]
    stages = {entry.subject: entry.stage for entry in selection.attrition}
    assert stages[f"{first_python}#1"] is Stage.OFFLINE_TESTS
    assert sum(stage is Stage.NOT_SELECTED for stage in stages.values()) == 3 + 1


def test_candidates_beyond_the_depth_are_never_probed() -> None:
    ranking = ranking_by_language(SEED, pool())
    probe = ScriptedProbe({})
    select_feasible(ranking, probe, 3, 2)
    assert len(probe.probed) == 10
    assert len({label.split("#")[0] for label in probe.probed}) == 10


def test_a_selection_writes_unconstructed_task_records_and_the_upstream_notice(tmp_path: Path) -> None:
    ranking = ranking_by_language(SEED, pool())
    selection = select_feasible(ranking, ScriptedProbe({}), 3, 2)
    write_selection(tmp_path, SEED, selection, [], lambda task, name: f"upstream {name}\n")
    first = selection.tasks[0]
    directory = tmp_path / "tasks" / first.task_id
    task = parse_task((directory / "task.json").read_text())
    assert not task.is_constructed and task.base_sha == "c" * 40 and task.merge_sha == "b" * 40
    assert (directory / "LICENSE").read_text() == "upstream LICENSE\n"
    assert len(list((tmp_path / "tasks").iterdir())) == 10
    assert read_attrition(tmp_path / "attrition.jsonl")
