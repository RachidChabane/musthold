"""Contracts of H selection by execution (ADR 0012): the record's schema, the seal guard of bundle version 3, and the
one stabiliser verification and selection share."""

from __future__ import annotations

from collections.abc import Sequence
from pathlib import Path
from typing import cast

import pytest

from specbench.adapters.schemas import SchemaName, load_schema, schema_problems
from specbench.application.corpus import seal_task
from specbench.application.h_execution import PythonIsolation, TreeHCandidateExecutor
from specbench.application.ports import CorpusRepository, HExecutionRequest, SealStore, TreeSpec
from specbench.application.verify_task import TaskVerifier
from specbench.domain.canonical import canonical_json
from specbench.domain.h_selection import (
    H_SELECTION_PATH,
    MAX_KEPT_PER_CRITERION,
    MAX_REFILL_ROUNDS,
    CandidateRuns,
    HExecution,
    HRound,
    HSelectionRecord,
    Verdict,
    h_selection_from_record,
    select_h,
)
from specbench.domain.h_tests import HFile, HTest, TestKind
from specbench.domain.languages import Language
from specbench.domain.oracle import OracleTest, Outcome, oracle_from_bundle
from specbench.domain.sealing import CorpusLock, SealedBundle, SealingError
from support.builders import H_TEST_ID, MERGE, TASK_ID, make_bundle, make_task, selection_record_for, task_json
from support.fakes import FixedOperatorEngine, RecordingTrees, ScriptedRunner

P, F = Outcome.PASSED, Outcome.FAILED
CALL = "build-" + "a" * 20


def record() -> HSelectionRecord:
    tests = [
        HTest(f"t.py::test_c{n}_{k}", n, TestKind.PROPERTY if k == 0 else TestKind.EXAMPLE, "t.py", 0)
        for n in (1, 2)
        for k in range(4)
    ]
    runs = {t.test_id: CandidateRuns((P, P, P), (F, F, F)) for t in tests}
    runs[tests[1].test_id] = CandidateRuns((F, F, F), (F, F, F))
    execution = HExecution(runs)
    selection = select_h(tests, execution, (1, 2), frozenset({1, 2}), 7, TASK_ID)
    return HSelectionRecord(TASK_ID, 7, (HRound(0, (1, 2), (CALL,)), HRound(1, (2,), (CALL,))), selection, execution)


def test_the_selection_record_conforms_to_its_schema_and_reads_back() -> None:
    body = record().to_record()
    assert schema_problems(SchemaName.H_SELECTION, body) == []
    read = h_selection_from_record(body)
    assert read.kept == record().selection.kept
    assert read.exposed == frozenset({f"{TASK_ID}-1", f"{TASK_ID}-2"})


def test_the_schema_names_the_verdicts_outcomes_and_limits_of_the_domain() -> None:
    schema = load_schema(SchemaName.H_SELECTION)
    candidate = schema["properties"]["candidates"]["items"]["properties"]
    assert candidate["verdict"]["enum"] == [v.value for v in Verdict]
    assert schema["$defs"]["outcome"]["enum"] == [o.value for o in Outcome]
    assert candidate["kind"]["enum"] == [k.value for k in TestKind]
    assert schema["properties"]["criteria"]["items"]["properties"]["kept"]["maximum"] == MAX_KEPT_PER_CRITERION
    assert schema["properties"]["rounds"]["maxItems"] == MAX_REFILL_ROUNDS + 1


class MemoryCorpus:
    def __init__(self) -> None:
        self.lock = CorpusLock(())
        self.written: list[str] = []

    def read_recipients(self) -> list[str]:
        return ["age1recipient"]

    def write_task(self, language: Language, task_id: str, task_json: str, ciphertext: bytes) -> None:
        self.written.append(task_id)

    def read_lock(self) -> CorpusLock:
        return self.lock

    def write_lock(self, lock: CorpusLock) -> None:
        self.lock = lock


class PlainSeals:
    def seal(self, bundle: SealedBundle, recipients: Sequence[str]) -> bytes:
        return b"sealed"


def seal(bundle: SealedBundle) -> MemoryCorpus:
    corpus = MemoryCorpus()
    seal_task(cast(CorpusRepository, corpus), cast(SealStore, PlainSeals()), task_json(), bundle)
    return corpus


def test_seal_takes_a_version_3_bundle_whose_h_is_exactly_the_selected_h() -> None:
    assert seal(make_bundle()).written == [TASK_ID]


@pytest.mark.parametrize(
    ("bundle", "message"),
    [
        (make_bundle(version=2), "below 3"),
        (make_bundle(without=(H_SELECTION_PATH,)), "not selected by execution"),
        (
            make_bundle({H_SELECTION_PATH: canonical_json(selection_record_for([])).encode()}),
            "not the tests selected by execution",
        ),
    ],
)
def test_seal_refuses_a_bundle_whose_h_was_not_selected_by_execution(bundle: SealedBundle, message: str) -> None:
    with pytest.raises(SealingError, match=message):
        seal(bundle)


def flaky_h_at_merge(spec: TreeSpec, test: OracleTest, run: int) -> Outcome:
    commit = spec.commit
    if test.test_id == H_TEST_ID and commit == MERGE:
        return F if run == 1 else P
    return P if commit == MERGE else F


def test_verification_and_selection_share_one_stabiliser_and_judge_a_test_alike(tmp_path: Path) -> None:
    trees = RecordingTrees(tmp_path)
    verifier = TaskVerifier(trees, ScriptedRunner(trees, flaky_h_at_merge), FixedOperatorEngine(None, "none"))
    bundle = make_bundle()
    report = verifier.verify(make_task(), oracle_from_bundle(bundle), bundle)
    merge_check = next(c for c in report.checks if c.name.value == "merge-passes-all")
    assert f"{H_TEST_ID} (H) is flaky at merge" in merge_check.problems

    h_file = HFile("tests/test_h.py", bundle.files["oracle/H/tests/test_h.py"].decode())
    candidate = HTest(H_TEST_ID, 2, TestKind.EXAMPLE, h_file.path, 0)
    executor = TreeHCandidateExecutor(verifier.stabilised, PythonIsolation())
    execution = executor.execute(HExecutionRequest(make_task(), {}, (), (h_file,), (candidate,)))
    selection = select_h([candidate], execution, (2,), frozenset({2}), 7, TASK_ID)
    assert execution.runs[H_TEST_ID].at_merge is Outcome.FLAKY
    assert selection.verdicts[H_TEST_ID] is Verdict.REJECTED_FLAKY
    verifier_merge = next(spec for spec in trees.specs.values() if spec.commit == MERGE and not spec.patches)
    selection_merge = list(trees.specs.values())[-2]
    assert selection_merge.commit == MERGE and selection_merge.overlay is not None
    assert verifier_merge.overlay is not None
    assert selection_merge.overlay[h_file.path] == verifier_merge.overlay[h_file.path]
