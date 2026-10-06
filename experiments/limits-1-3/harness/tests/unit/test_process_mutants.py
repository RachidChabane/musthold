from __future__ import annotations

from pathlib import Path

import pytest

from pilot.application.ports import RunRequest
from pilot.application.process_mutants import MutantProcessor, ReferenceUnstable
from pilot.domain.edits import Edit, EditError, EditFailure, EditSet
from pilot.domain.mutants import (
    Candidate,
    DifferentialKind,
    DifferentialResult,
    MutantStatus,
)
from pilot.domain.outcomes import KillResult, RunResult, TestOutcome

P, F = TestOutcome.PASSED, TestOutcome.FAILED
TESTS = ("t1", "t2", "t3")
ORIGINAL = "def f(x):\n    return x + 1\n"


def edit_set(tag: str) -> EditSet:
    return EditSet((Edit("m.py", "f", tag),))


class FakePreview:
    """Maps the edit's new_source tag to a mutated module text, or raises."""

    def __init__(self, table):
        self.table = table

    def preview(self, project_root, edit_set):
        tag = edit_set.edits[0].new_source
        value = self.table[tag]
        if isinstance(value, EditError):
            raise value
        return {"m.py": (ORIGINAL, value)}


class FakeSmoke:
    def __init__(self, broken=()):
        self.broken = set(broken)

    def passes(self, edit_set):
        return edit_set.edits[0].new_source not in self.broken


class FakeDifferential:
    def __init__(self):
        self.seen = []

    def compare(self, edit_set):
        self.seen.append(edit_set.edits[0].new_source)
        return DifferentialResult(DifferentialKind.NO_DIFF_FOUND)


class FakeRunner:
    """Reference passes; a mutant's outcomes come from `plan[tag]`, reruns from `reruns[tag]`."""

    def __init__(self, plan, reruns=None, reference_failures=0, returncodes=None):
        self.plan, self.reruns = plan, reruns or {}
        self.reference_failures = reference_failures
        self.returncodes = returncodes or {}
        self.requests = []

    def run_many(self, requests):
        self.requests.append(dict(requests))
        return {key: self._one(request) for key, request in requests.items()}

    def _one(self, request: RunRequest) -> RunResult:
        if request.edit_set is None:
            bad = self.reference_failures
            self.reference_failures = max(0, bad - 1)
            return RunResult({t: (F if bad else P) for t in TESTS}, 0)
        tag = request.edit_set.edits[0].new_source
        if request.nodeids:
            return RunResult({n: self.reruns.get(tag, {}).get(n, F) for n in request.nodeids}, 1)
        return RunResult(dict(zip(TESTS, self.plan[tag], strict=True)), self.returncodes.get(tag, 1))


def processor(preview, runner, smoke=None, differential=None):
    return MutantProcessor(runner, FakePreview(preview), Path("."), smoke or FakeSmoke(), differential or FakeDifferential())


def candidates(*tags, arm="T"):
    return [Candidate(arm, f"m{i}", edit_set(tag)) for i, tag in enumerate(tags)]


def by_id(records):
    return {r.candidate.mutant_id: r for r in records}


def test_reference_must_pass_every_confirmation_run():
    runner = FakeRunner({}, reference_failures=1)
    with pytest.raises(ReferenceUnstable):
        processor({}, runner).confirm_reference()
    assert len(runner.requests[0]) == 3


def test_kill_vector_marks_killed_survived_and_flaky():
    runner = FakeRunner({"a": (F, P, F)}, reruns={"a": {"t1": F, "t3": P}})
    records = processor({"a": "def f(x):\n    return x + 2\n"}, runner).process(candidates("a"))
    vector = records[0].kill_vector
    assert vector == {"t1": KillResult.KILLED, "t2": KillResult.SURVIVED, "t3": KillResult.FLAKY}
    assert records[0].killed_by == ["t1"]


def test_only_killing_tests_are_rerun_and_survivors_are_not():
    runner = FakeRunner({"a": (F, P, P)})
    processor({"a": "def f(x):\n    return x + 2\n"}, runner).process(candidates("a"))
    rerun = runner.requests[-1]["T/m0"]
    assert rerun.nodeids == ("t1",)


def test_timeouts_count_as_error():
    runner = FakeRunner({"a": (TestOutcome.ERROR, P, P)})
    records = processor({"a": "def f(x):\n    return x + 2\n"}, runner).process(candidates("a"))
    assert records[0].kill_vector["t1"] is KillResult.ERROR


def test_absent_qualname_and_unparseable_edits_are_stillborn_with_their_reason():
    table = {
        "absent": EditError(EditFailure.ABSENT_QUALNAME, "x"),
        "broken": EditError(EditFailure.UNPARSEABLE, "y"),
    }
    records = by_id(processor(table, FakeRunner({})).process(candidates("absent", "broken")))
    assert [records[k].status for k in ("m0", "m1")] == [MutantStatus.STILLBORN] * 2
    assert [records[k].reason for k in ("m0", "m1")] == ["absent_qualname", "unparseable"]


def test_more_than_ten_changed_lines_is_stillborn():
    big = "def f(x):\n" + "".join(f"    y{i} = {i}\n" for i in range(11)) + "    return x + 1\n"
    record = processor({"big": big}, FakeRunner({})).process(candidates("big"))[0]
    assert (record.status, record.reason) == (MutantStatus.STILLBORN, "too_many_changed_lines")


def test_exactly_ten_changed_lines_is_allowed():
    ten = "def f(x):\n" + "".join(f"    y{i} = {i}\n" for i in range(10)) + "    return x + 1\n"
    runner = FakeRunner({"ten": (F, P, P)})
    record = processor({"ten": ten}, runner).process(candidates("ten"))[0]
    assert record.status is MutantStatus.EVALUATED


def test_comment_or_docstring_only_edit_is_trivially_equivalent_and_never_run():
    trivial = '# note\ndef f(x):\n    """Doc."""\n    return x + 1  # same\n'
    runner = FakeRunner({})
    record = processor({"t": trivial}, runner).process(candidates("t"))[0]
    assert record.status is MutantStatus.TRIVIALLY_EQUIVALENT
    assert all(set(r) <= {"reference-1", "reference-2", "reference-3"} for r in runner.requests)


def test_identical_normalized_mutants_in_one_arm_are_merged_with_provenance():
    same_a = "def f(x):\n    return x + 2\n"
    same_b = "def f(x):\n    # reworded\n    return x+2\n"
    runner = FakeRunner({"a": (F, P, P), "b": (F, P, P)})
    records = by_id(processor({"a": same_a, "b": same_b}, runner).process(candidates("a", "b")))
    assert records["m1"].status is MutantStatus.DUPLICATE and records["m1"].duplicate_of == "m0"
    assert records["m0"].merged_ids == ["m1"]
    assert records["m0"].status is MutantStatus.EVALUATED


def test_duplicates_across_arms_are_kept():
    body = "def f(x):\n    return x + 2\n"
    pending = [Candidate("T", "m", edit_set("a")), Candidate("O", "m", edit_set("a"))]
    runner = FakeRunner({"a": (F, P, P)})
    records = processor({"a": body}, runner).process(pending)
    assert [r.status for r in records] == [MutantStatus.EVALUATED] * 2


def test_import_smoke_failure_is_stillborn_and_skips_the_suite():
    runner = FakeRunner({})
    record = processor({"a": "def f(x):\n    return x + 2\n"}, runner, smoke=FakeSmoke({"a"})).process(candidates("a"))[0]
    assert (record.status, record.reason) == (MutantStatus.STILLBORN, "import_smoke_failed")


def test_collection_error_is_stillborn():
    runner = FakeRunner({"a": (P, P, P)}, returncodes={"a": 2})
    record = processor({"a": "def f(x):\n    return x + 2\n"}, runner).process(candidates("a"))[0]
    assert (record.status, record.reason) == (MutantStatus.STILLBORN, "collection_error")


def test_differential_runs_only_for_evaluated_mutants():
    differential = FakeDifferential()
    runner = FakeRunner({"live": (F, P, P)})
    table = {
        "live": "def f(x):\n    return x + 2\n",
        "dead": EditError(EditFailure.ABSENT_QUALNAME, "x"),
    }
    records = processor(table, runner, differential=differential).process(candidates("live", "dead"))
    assert differential.seen == ["live"]
    assert records[0].differential == DifferentialResult(DifferentialKind.NO_DIFF_FOUND)
    assert records[1].differential is None
