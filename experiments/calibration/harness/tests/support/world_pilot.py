"""The pilot half of the synthetic world: the snapshot's `prereg/` files and a run-02 directory."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Final

import yaml

from calibration.domain.canonical import JsonValue, document_json, line_json, sha256_hex
from calibration.domain.constants import ENCODING
from calibration.domain.pilot_records import criterion_number, criterion_ref
from support.world_model import (
    ARM_P,
    CRITERIA,
    CRITERION_REFS,
    DIFFERS,
    DIFFERS_SHARE,
    ERROR,
    ERROR_SHARE,
    FLAKY,
    FLAKY_SHARE,
    ITEM_TESTS,
    KILLED,
    MEASURED_MUTANTS,
    MUTANTS,
    NO_DIFF_FOUND,
    NO_DIFF_SHARE,
    NONE_OPTION,
    ORACLED_MUTANTS,
    SKIPPED,
    STRAY_KILL_SHARE,
    SURVIVED,
    TARGET_KILL_SHARE,
    TESTS,
    MutantSpec,
    TestSpec,
    short_hash,
    unit_float,
)

PILOT_RUN_ID: Final = "run-02"
JUDGE_HOST: Final = "J-H"
JUDGE_OTHER: Final = "J-X"
ARM_MASKED: Final = "NAME-MASKED"
ARM_SHOWN: Final = "NAME-SHOWN"
VALIDITY_JUDGES: Final = ("V-H1", "V-H2")
REPETITIONS: Final = 5
HOST_ACCURACY: Final = 0.8
VERDICT_ACCURACY: Final = 0.85
UNDETERMINED_SHARE: Final = 0.07
STATUS_OK: Final = "ok"
STATUS_MISSING: Final = "missing"
RATERS: Final = ("R-OAI", "R-AUTHOR")
DISAGREEING_TESTS: Final = (2, 8)
SILENT_TEST: Final = 7
PARTIAL_REPETITION_TEST: Final = 4
UNVOTED_MUTANT: Final = 3
HALF_VOTED_MUTANT: Final = 5
PILOT_STAGES: Final = (
    ("core", "2026-09-12T21:03:11+02:00", "2026-09-12T23:40:02+02:00", 9411.0),
    ("secondary", "2026-09-13T21:01:00+02:00", "2026-09-13T22:10:30+02:00", 4170.0),
)
LABEL_FIELDS: Final = ("nodeid", "cites", "label", "shape", "checks", "alignment_truth")
SHAPE_NOT_APPLICABLE: Final = "n/a"
ARM_P_KIND: Final = "r"
EDIT_FILE: Final = "src/parkfee/tariff.py"
JSONL_SUFFIX: Final = ".jsonl"


def host_k1_answer(test: TestSpec) -> str:
    """The criterion a careful reader names for the test: NONE for a vacuous or off-spec test, else its first check."""
    if test.label in ("VACUOUS", "OFFSPEC"):
        return NONE_OPTION
    assert not isinstance(test.checks, str)
    return criterion_ref(test.checks[0])


def _label_record(test: TestSpec) -> dict[str, object]:
    return {
        "nodeid": test.nodeid,
        "cites": test.cites_ref,
        "label": test.label,
        "shape": SHAPE_NOT_APPLICABLE,
        "checks": test.check_refs,
        "alignment_truth": test.alignment_truth,
    }


def write_snapshot_prereg(prereg_dir: Path) -> None:
    """`prereg/labels.yaml` and `prereg/arm_p/labels.yaml` of the pilot snapshot."""
    (prereg_dir / "arm_p").mkdir(parents=True)
    labels = [_label_record(test) for test in TESTS]
    (prereg_dir / "labels.yaml").write_text(
        yaml.safe_dump(labels, sort_keys=False, default_flow_style=None), encoding=ENCODING
    )
    planted = {
        mutant.mutant_id: {
            "declared_target": criterion_ref(mutant.target or CRITERIA[0]),
            "kind": ARM_P_KIND,
            "source": mutant.source,
        }
        for mutant in MUTANTS
        if mutant.arm == ARM_P
    }
    (prereg_dir / "arm_p" / "labels.yaml").write_text(yaml.safe_dump(planted, sort_keys=True), encoding=ENCODING)


def _write(run_dir: Path, name: str, content: str) -> None:
    path = run_dir / name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding=ENCODING)


def _jsonl(rows: Sequence[Mapping[str, JsonValue]]) -> str:
    return "".join(line_json(row) for row in rows)


def _edit(mutant: MutantSpec) -> dict[str, JsonValue]:
    return {
        "file": EDIT_FILE,
        "qualname": f"charge_{mutant.mutant_id[:4]}",
        "new_source": f"def charge_{mutant.mutant_id[:4]}(x):\n    return x + 1\n",
    }


def _mutant_document(mutant: MutantSpec) -> dict[str, JsonValue]:
    generator: JsonValue = (
        {"model": "claude-sonnet-5-5", "cli": "claude", "call_id": mutant.call_id, "nonce": "n"}
        if mutant.call_id is not None
        else None
    )
    return {
        "mutant_id": mutant.mutant_id,
        "arm": mutant.arm,
        "target": criterion_ref(mutant.target) if mutant.target is not None else None,
        "slot": None,
        "generator": generator,
        "edits": [_edit(mutant)],
        "normalized_edits": [_edit(mutant)],
        "mechanism_summary": None,
        "merged_from": [mutant.mutant_id],
    }


def _processing(mutant: MutantSpec) -> dict[str, JsonValue]:
    return {
        "mutant_id": mutant.mutant_id,
        "stillborn": mutant.stillborn,
        "stillborn_reason": "does not import" if mutant.stillborn else None,
        "status": mutant.status,
        "duplicate_of": mutant.duplicate_of,
        "changed_lines": 2,
    }


def _kill_result(mutant: MutantSpec, test: TestSpec) -> str:
    draw = unit_float(mutant.mutant_id, test.nodeid, "kill")
    failed = mutant.failed or ()
    if test.cites in failed and draw < TARGET_KILL_SHARE:
        return KILLED
    if test.cites not in failed and draw < STRAY_KILL_SHARE:
        return KILLED
    if draw > 1.0 - FLAKY_SHARE:
        return FLAKY
    if draw < ERROR_SHARE and test.cites not in failed:
        return ERROR
    return SURVIVED


def _differential(mutant: MutantSpec) -> dict[str, JsonValue]:
    draw = unit_float(mutant.mutant_id, "differential")
    status = DIFFERS if draw < DIFFERS_SHARE else NO_DIFF_FOUND if draw < NO_DIFF_SHARE else SKIPPED
    differs = status == DIFFERS
    return {
        "mutant_id": mutant.mutant_id,
        "status": status,
        "function": "charge" if differs else None,
        "args": [1] if differs else None,
        "original_out": "2" if differs else None,
        "mutant_out": "3" if differs else None,
        "reason": None if differs else "no differing input",
    }


def _mutant_class(mutant: MutantSpec) -> dict[str, JsonValue]:
    failed = mutant.failed or ()
    anchor = mutant.target if mutant.target in failed else (failed[0] if failed else None)
    return {
        "mutant_id": mutant.mutant_id,
        "arm": mutant.arm,
        "target": criterion_ref(mutant.target) if mutant.target is not None else None,
        "per_criterion": {
            ref: "VALID_W" if criterion_number(ref) in failed else "NOT_VIOLATING" for ref in CRITERION_REFS
        },
        "equivalent": False,
        "unattributed": not failed,
        "primary": {"included": anchor is not None, "criterion": criterion_ref(anchor) if anchor else None},
        "survived_all": not failed,
    }


def _host_vote(test: TestSpec, repetition: int) -> dict[str, JsonValue]:
    correct = unit_float(test.nodeid, repetition, "ref") < HOST_ACCURACY
    answer = host_k1_answer(test)
    if not correct:
        pool = [ref for ref in (*CRITERION_REFS, NONE_OPTION) if ref != answer]
        answer = pool[int(unit_float(test.nodeid, repetition, "wrong") * len(pool))]
    adequate = test.label in ("GOOD", "GOOD_BROAD", "MIS_OTHER")
    levels = ("FULL", "PARTIAL") if adequate else ("WEAK", "NONE")
    coverage = levels[int(unit_float(test.nodeid, repetition, "level") * len(levels))]
    confidence = round(0.6 + 0.35 * unit_float(test.nodeid, repetition, "confidence"), 2)
    return {
        "judge": JUDGE_HOST,
        "arm": ARM_MASKED,
        "nodeid": test.nodeid,
        "rep": repetition,
        "call_id": f"jh-{short_hash(test.nodeid)}-{repetition}",
        "status": STATUS_OK,
        "verifies_ref": answer,
        "coverage": coverage,
        "confidence": confidence,
        "rationale": None,
    }


def _missing_vote(test: TestSpec, repetition: int) -> dict[str, JsonValue]:
    return {
        "judge": JUDGE_HOST,
        "arm": ARM_MASKED,
        "nodeid": test.nodeid,
        "rep": repetition,
        "call_id": f"call-missing-{repetition}",
        "status": STATUS_MISSING,
        "verifies_ref": None,
        "coverage": None,
        "confidence": None,
        "rationale": None,
    }


def _alignment_rows() -> list[dict[str, JsonValue]]:
    rows: list[dict[str, JsonValue]] = []
    for index, test in enumerate(ITEM_TESTS):
        for repetition in range(1, REPETITIONS + 1):
            silent = index == SILENT_TEST
            partial = index == PARTIAL_REPETITION_TEST and repetition == REPETITIONS
            rows.append(_missing_vote(test, repetition) if silent or partial else _host_vote(test, repetition))
        rows.append({**_host_vote(test, 1), "arm": ARM_SHOWN, "verifies_ref": test.cites_ref})
        rows.append({**_host_vote(test, 1), "judge": JUDGE_OTHER, "verifies_ref": NONE_OPTION})
    return rows


def _requirement(mutant: MutantSpec, number: int, judge: str) -> dict[str, JsonValue]:
    violated = number in (mutant.failed or ())
    right = unit_float(mutant.mutant_id, number, judge, "verdict") < VERDICT_ACCURACY
    if unit_float(mutant.mutant_id, number, judge, "abstain") < UNDETERMINED_SHARE:
        verdict = "UNDETERMINED"
    else:
        verdict = "VIOLATED" if violated == right else "NOT_VIOLATED"
    confidence = round(0.5 + 0.45 * unit_float(mutant.mutant_id, number, judge, "confidence"), 2)
    return {"verdict": verdict, "confidence": confidence, "witness": None}


def _validity_rows() -> list[dict[str, JsonValue]]:
    rows: list[dict[str, JsonValue]] = []
    for index, mutant in enumerate(ORACLED_MUTANTS):
        if index == UNVOTED_MUTANT:
            continue
        for judge in VALIDITY_JUDGES:
            missing = index == HALF_VOTED_MUTANT and judge == VALIDITY_JUDGES[1]
            payload: JsonValue = None
            if not missing:
                payload = {
                    "requirements": {ref: _requirement(mutant, criterion_number(ref), judge) for ref in CRITERION_REFS}
                }
            rows.append(
                {
                    "mutant_id": mutant.mutant_id,
                    "judge": judge,
                    "call_id": f"v-{mutant.mutant_id}-{judge}",
                    "status": STATUS_MISSING if missing else STATUS_OK,
                    "payload": payload,
                }
            )
    return rows


def _relabel_rows() -> list[dict[str, JsonValue]]:
    rows: list[dict[str, JsonValue]] = []
    for index, test in enumerate(TESTS):
        for rater in RATERS:
            label = "WEAK" if index in DISAGREEING_TESTS and rater == RATERS[0] else test.label
            rows.append({"rater": rater, "nodeid": test.nodeid, "label": label, "checks": test.check_refs})
    return rows


def _timing() -> dict[str, JsonValue]:
    return {
        stage: {"started": started, "finished": finished, "seconds": seconds}
        for stage, started, finished, seconds in PILOT_STAGES
    }


def pilot_files() -> dict[str, str]:
    """Every file of the synthetic run-02 directory, by relative path."""
    files: dict[str, str] = {
        "bindings.json": document_json({test.nodeid: test.cites_ref for test in TESTS}),
        "processing.jsonl": _jsonl([_processing(mutant) for mutant in MUTANTS]),
        "differential.jsonl": _jsonl([_differential(mutant) for mutant in MEASURED_MUTANTS]),
        "kills.jsonl": _jsonl(
            [
                {"mutant_id": mutant.mutant_id, "results": {test.nodeid: _kill_result(mutant, test) for test in TESTS}}
                for mutant in MUTANTS
                if mutant.evaluated
            ]
        ),
        "alignment.jsonl": _jsonl(_alignment_rows()),
        "validity.jsonl": _jsonl(_validity_rows()),
        "mutant_classes.json": document_json([_mutant_class(mutant) for mutant in MEASURED_MUTANTS]),
        "oracle_classes.json": document_json(
            {
                "oracle_digest": sha256_hex("oracle"),
                "mutants": {
                    mutant.mutant_id: {
                        "arm": mutant.arm,
                        "failed_criteria": list(mutant.failed or ()),
                        "source": mutant.source,
                    }
                    for mutant in ORACLED_MUTANTS
                },
            }
        ),
        "relabel.jsonl": _jsonl(_relabel_rows()),
        "timing.json": document_json(_timing()),
    }
    for mutant in MUTANTS:
        files[f"mutants/{mutant.mutant_id}.json"] = document_json(_mutant_document(mutant))
    return files


def write_pilot_run(run_dir: Path) -> None:
    for name, content in pilot_files().items():
        _write(run_dir, name, content)
