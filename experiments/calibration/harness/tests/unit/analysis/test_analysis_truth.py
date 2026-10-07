"""The pilot's truth and judgment files as `analyze` parses them."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Final

import pytest

from calibration.analysis.truth import (
    MatrixInputs,
    MutantMeta,
    PilotTruth,
    TestLabel,
    consensus_nodeids,
    parse_alignment_votes,
    parse_arm_p_sources,
    parse_kills,
    parse_labels,
    parse_matrix_inputs,
    parse_mutant_meta,
    parse_oracle,
    parse_relabels,
    parse_validity_votes,
    read_pilot_truth,
)
from calibration.analysis.votes import NONE_ANSWER, JudgeVote, ValidityVote
from calibration.domain.jsonread import RecordError
from support.synthetic_world import build_world
from support.world_model import ARM_T, MEASURED_MUTANTS, MUTANTS, ORACLED_MUTANTS, TESTS

REF_ONE: Final = "PARK-1~1"
REF_TWO: Final = "PARK-2~1"
REF_THREE: Final = "PARK-3~1"
NODE_A: Final = "tests/test_a.py::test_a"
NODE_B: Final = "tests/test_b.py::test_b"
LABELS_YAML: Final = f"""
- nodeid: {NODE_A}
  cites: {REF_ONE}
  label: GOOD_BROAD
  checks: ["{REF_ONE}", "{REF_TWO}"]
  alignment_truth: ALIGNED
- nodeid: tests/test_c.py::test_c
  cites: {REF_ONE}
  label: VACUOUS
  checks: NONE
  alignment_truth: NOT_ALIGNED
- nodeid: tests/test_d.py::test_d
  cites: {REF_TWO}
  label: OFFSPEC
  checks: UNCOVERED
  alignment_truth: NOT_ALIGNED
- nodeid: tests/test_e.py::test_e
  cites: {REF_TWO}
  label: MIS_OTHER
  checks: ["{REF_THREE}"]
  alignment_truth: NOT_ALIGNED
- nodeid: tests/test_f.py::test_f
  cites: {REF_THREE}
  label: WEAK
  checks: ["{REF_THREE}"]
  alignment_truth: NOT_ALIGNED
- nodeid: tests/test_g.py::test_g
  cites: {REF_THREE}
  label: GOOD
  checks: ["{REF_THREE}"]
  alignment_truth: ALIGNED
"""


def _by_class() -> dict[str, TestLabel]:
    return {label.label_class: label for label in parse_labels(LABELS_YAML)}


def test_labels_keep_the_class_the_checks_and_the_truth() -> None:
    labels = _by_class()
    assert set(labels) == {"GOOD_BROAD", "VACUOUS", "OFFSPEC", "MIS_OTHER", "WEAK", "GOOD"}
    assert labels["GOOD_BROAD"].checks == (REF_ONE, REF_TWO)
    assert labels["VACUOUS"].checks == ()
    assert labels["OFFSPEC"].checks == ()


def test_an_unrecognised_label_is_refused() -> None:
    content = "- {nodeid: x, cites: PARK-1~1, label: '?', checks: [], alignment_truth: ALIGNED}"
    with pytest.raises(RecordError):
        parse_labels(content)


@pytest.mark.parametrize(
    ("label_class", "strict", "lenient"),
    [
        ("GOOD", {REF_THREE}, {REF_THREE}),
        ("GOOD_BROAD", {REF_ONE}, {REF_ONE, REF_TWO}),
        ("WEAK", {REF_THREE}, {REF_THREE}),
        ("MIS_OTHER", {REF_THREE}, {REF_THREE}),
        ("VACUOUS", {NONE_ANSWER, REF_ONE}, {NONE_ANSWER, REF_ONE}),
        ("OFFSPEC", {NONE_ANSWER, REF_TWO}, {NONE_ANSWER, REF_TWO}),
    ],
)
def test_k1_accepted_sets(label_class: str, strict: set[str], lenient: set[str]) -> None:
    label = _by_class()[label_class]
    assert label.k1_accepted(lenient=False) == frozenset(strict)
    assert label.k1_accepted(lenient=True) == frozenset(lenient)


@pytest.mark.parametrize(
    ("label_class", "k2", "k3"),
    [
        ("GOOD", True, True),
        ("GOOD_BROAD", True, True),
        ("WEAK", False, False),
        ("VACUOUS", False, False),
        ("OFFSPEC", False, False),
        ("MIS_OTHER", None, False),
    ],
)
def test_k2_and_k3_truth(label_class: str, k2: bool | None, k3: bool) -> None:
    label = _by_class()[label_class]
    assert label.k2_positive is k2
    assert label.k3_positive is k3


def _relabel_row(rater: str, nodeid: str, label: str) -> str:
    return json.dumps({"rater": rater, "nodeid": nodeid, "label": label, "checks": []})


def test_relabels_keep_the_class_per_rater() -> None:
    content = "\n".join([_relabel_row("R-OAI", NODE_A, "GOOD(PARK-1~1)"), _relabel_row("R-AUTHOR", NODE_A, "WEAK")])
    assert parse_relabels(content) == {NODE_A: {"R-OAI": "GOOD", "R-AUTHOR": "WEAK"}}


def test_consensus_needs_both_raters_to_agree_with_the_author() -> None:
    labels = parse_labels(LABELS_YAML)
    first, second = labels[0], labels[1]
    relabels = {
        first.nodeid: {"R-OAI": first.label_class, "R-AUTHOR": first.label_class},
        second.nodeid: {"R-OAI": second.label_class, "R-AUTHOR": "WEAK"},
        labels[2].nodeid: {"R-OAI": labels[2].label_class},
    }
    assert consensus_nodeids(labels, relabels) == frozenset({first.nodeid})


def test_arm_p_sources_read_the_criterion_number() -> None:
    content = "abc:\n  source: PARK-8_b\n  kind: r\ndef:\n  source: PARK-2_a\n"
    assert parse_arm_p_sources(content) == {"abc": 8, "def": 2}


def test_an_arm_p_source_without_a_number_is_refused() -> None:
    with pytest.raises(RecordError):
        parse_arm_p_sources("abc:\n  source: planted\n")


def test_oracle_reads_the_failed_criteria() -> None:
    content = json.dumps({"mutants": {"m1": {"failed_criteria": [1, 3]}, "m2": {"failed_criteria": []}}})
    assert parse_oracle(content) == {"m1": frozenset({1, 3}), "m2": frozenset()}


def test_kills_keep_every_result_per_mutant() -> None:
    row = {"mutant_id": "m1", "results": {NODE_A: "killed", NODE_B: "flaky"}}
    assert parse_kills(json.dumps(row)) == {"m1": {NODE_A: "killed", NODE_B: "flaky"}}


def _alignment_row(**changes: object) -> str:
    row: dict[str, object] = {
        "judge": "J-H",
        "arm": "NAME-MASKED",
        "nodeid": NODE_A,
        "rep": 1,
        "status": "ok",
        "verifies_ref": REF_ONE,
        "coverage": "FULL",
        "confidence": 0.8,
    }
    return json.dumps(row | changes)


def test_alignment_votes_keep_only_answered_masked_host_repetitions() -> None:
    rows = [
        _alignment_row(),
        _alignment_row(rep=2, confidence=None),
        _alignment_row(rep=3, status="missing"),
        _alignment_row(rep=4, judge="J-X"),
        _alignment_row(rep=5, arm="NAME-SHOWN"),
    ]
    assert parse_alignment_votes("\n".join(rows)) == {
        NODE_A: {1: JudgeVote(REF_ONE, "FULL", 0.8), 2: JudgeVote(REF_ONE, "FULL", 0.0)}
    }


def _validity_row(judge: str, status: str, payload: object) -> str:
    return json.dumps({"mutant_id": "m1", "judge": judge, "status": status, "payload": payload})


def test_validity_votes_keep_answered_host_judges_only() -> None:
    requirements = {
        REF_ONE: {"verdict": "VIOLATED", "confidence": 0.9, "witness": None},
        REF_TWO: {"verdict": "UNDETERMINED"},
    }
    rows = [
        _validity_row("V-H1", "ok", {"requirements": requirements}),
        _validity_row("V-H2", "missing", None),
        _validity_row("A-OPUS", "ok", {"requirements": requirements}),
    ]
    assert parse_validity_votes("\n".join(rows)) == {
        "m1": {"V-H1": {REF_ONE: ValidityVote("VIOLATED", 0.9), REF_TWO: ValidityVote("UNDETERMINED", 0.0)}}
    }


def test_mutant_meta_reads_arm_target_and_generator_call() -> None:
    document = {"mutant_id": "m1", "arm": "T", "target": REF_ONE, "generator": {"call_id": "gen-1"}}
    assert parse_mutant_meta(json.dumps(document)) == MutantMeta("m1", "T", REF_ONE, "gen-1")
    planted = {"mutant_id": "m2", "arm": "P", "target": None, "generator": None}
    assert parse_mutant_meta(json.dumps(planted)) == MutantMeta("m2", "P", None, None)


def _class_record(mutant_id: str, arm: str, criterion: str | None, *, unattributed: bool) -> dict[str, object]:
    return {
        "mutant_id": mutant_id,
        "arm": arm,
        "primary": {"included": criterion is not None, "criterion": criterion},
        "unattributed": unattributed,
    }


CLASS_RECORDS: Final = [
    _class_record("m_valid", "T", REF_TWO, unattributed=False),
    _class_record("m_unattributed", "T", None, unattributed=True),
    _class_record("m_planted", "P", REF_ONE, unattributed=False),
    _class_record("m_unevaluated", "T", REF_ONE, unattributed=False),
    _class_record("m_unplaced", "T", None, unattributed=False),
]
KILLS: Final = {
    "m_valid": {NODE_A: "killed", NODE_B: "survived"},
    "m_unattributed": {NODE_A: "killed", NODE_B: "killed"},
}
EVALUATED: Final = frozenset({"m_valid", "m_unattributed", "m_planted", "m_unplaced"})
EXPECTED_MATRIX: Final = MatrixInputs(((2, frozenset({NODE_A})),), (frozenset({NODE_A, NODE_B}),))


def test_matrix_inputs_from_a_list_of_records() -> None:
    assert parse_matrix_inputs(json.dumps(CLASS_RECORDS), KILLS, EVALUATED) == EXPECTED_MATRIX


def test_matrix_inputs_from_a_keyed_mapping() -> None:
    keyed = {
        str(record["mutant_id"]): {k: v for k, v in record.items() if k != "mutant_id"} for record in CLASS_RECORDS
    }
    assert parse_matrix_inputs(json.dumps(keyed), KILLS, EVALUATED) == EXPECTED_MATRIX


def _truth(
    kills: dict[str, dict[str, str]], bindings: dict[str, str], differential: dict[str, dict[str, object]]
) -> PilotTruth:
    return PilotTruth((), frozenset(), {}, bindings, (), differential, {}, {}, kills, {}, {}, MatrixInputs((), ()))


def test_f3_status_defaults_to_no_diff_found() -> None:
    truth = _truth({}, {}, {"m1": {"status": "DIFFERS"}})
    assert truth.f3_status("m1") == "DIFFERS"
    assert truth.f3_status("m2") == "NO_DIFF_FOUND"


def test_bound_tests_are_the_tests_bound_to_the_criterion() -> None:
    truth = _truth({}, {NODE_B: REF_ONE, NODE_A: REF_ONE, "other": REF_TWO}, {})
    assert truth.bound_tests(REF_ONE) == (NODE_A, NODE_B)


def test_a_reproduced_kill_needs_a_killed_result_of_a_bound_test() -> None:
    bindings = {NODE_A: REF_ONE, NODE_B: REF_TWO}
    truth = _truth(
        {
            "killed": {NODE_A: "killed"},
            "flaky": {NODE_A: "flaky"},
            "error": {NODE_A: "error"},
            "elsewhere": {NODE_B: "killed"},
        },
        bindings,
        {},
    )
    assert truth.reproduced_kill("killed", REF_ONE)
    assert not truth.reproduced_kill("flaky", REF_ONE)
    assert not truth.reproduced_kill("error", REF_ONE)
    assert not truth.reproduced_kill("elsewhere", REF_ONE)
    assert not truth.reproduced_kill("unknown", REF_ONE)


@pytest.fixture(scope="module")
def world_truth(tmp_path_factory: pytest.TempPathFactory) -> PilotTruth:
    world = build_world(tmp_path_factory.mktemp("truth-world"))
    return read_pilot_truth(world.prereg_dir, world.pilot_run_dir)


def test_a_world_is_read_with_the_counts_of_its_catalogue(world_truth: PilotTruth) -> None:
    assert len(world_truth.labels) == len(TESTS)
    assert len(world_truth.bindings) == len(TESTS)
    assert len(world_truth.processing) == len(MUTANTS)
    assert len(world_truth.mutants) == len(MUTANTS)
    assert set(world_truth.oracle) == {mutant.mutant_id for mutant in ORACLED_MUTANTS}
    assert len(world_truth.arm_p_sources) == sum(1 for mutant in MEASURED_MUTANTS if mutant.arm == "P")


def test_a_world_oracle_holds_the_failed_criteria_of_the_catalogue(world_truth: PilotTruth) -> None:
    for mutant in ORACLED_MUTANTS:
        assert world_truth.oracle[mutant.mutant_id] == frozenset(mutant.failed or ())


def test_a_world_consensus_drops_the_test_the_raters_relabel_differently(world_truth: PilotTruth) -> None:
    relabelled = TESTS[2]
    assert relabelled.label == "VACUOUS"
    assert world_truth.consensus == {test.nodeid for test in TESTS} - {relabelled.nodeid}


def test_a_world_matrix_pools_hold_the_arm_t_mutants_of_the_catalogue(world_truth: PilotTruth) -> None:
    evaluated_t = [m for m in MEASURED_MUTANTS if m.arm == ARM_T]
    expected_valid = sum(1 for m in evaluated_t if m.failed)
    assert len(world_truth.matrix.valid_pool) == expected_valid
    assert len(world_truth.matrix.unattributed) == len(evaluated_t) - expected_valid


def test_a_world_votes_leave_out_the_unvoted_tests_and_mutants(world_truth: PilotTruth) -> None:
    assert len(world_truth.alignment_votes) == len({label.nodeid for label in world_truth.labels}) - 2
    assert len(world_truth.validity_votes) == len(ORACLED_MUTANTS) - 1


def test_a_world_mutant_documents_name_the_generator_call_of_arm_t(world_truth: PilotTruth) -> None:
    for mutant in MEASURED_MUTANTS:
        meta = world_truth.mutants[mutant.mutant_id]
        assert meta.arm == mutant.arm
        assert meta.generator_call_id == mutant.call_id


def test_a_world_is_unread_when_a_file_is_absent(tmp_path: Path) -> None:
    world = build_world(tmp_path / "w")
    (world.pilot_run_dir / "oracle_classes.json").unlink()
    with pytest.raises(FileNotFoundError):
        read_pilot_truth(world.prereg_dir, world.pilot_run_dir)
