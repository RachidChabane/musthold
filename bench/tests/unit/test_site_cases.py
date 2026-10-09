"""Case records: the schema, the parser's refusals and the reading of a gate report."""

from __future__ import annotations

import copy
import json
from pathlib import Path
from typing import Any

import pytest

from specbench.adapters.schemas import SchemaName, schema_problems
from specbench.adapters.site_inputs import EvidenceOutsideRoot, EvidenceReader
from specbench.domain.arms import Arm
from specbench.domain.cases import case_from_record
from specbench.domain.jsonread import RecordError
from specbench.site.gate_report import read_gate_report
from support.site import REPOSITORY_ROOT

CASES = REPOSITORY_ROOT / "bench/tests/fixtures/site_run/cases"


def fixture_case(name: str = "parcel-shipping-boundary") -> dict[str, Any]:
    return json.loads((CASES / f"{name}.json").read_text(encoding="utf-8"))


@pytest.mark.parametrize("path", sorted(CASES.glob("*.json")), ids=lambda p: p.stem)
def test_fixture_cases_conform_to_the_schema_and_parse(path: Path) -> None:
    record = json.loads(path.read_text(encoding="utf-8"))
    assert schema_problems(SchemaName.CASE, record) == []
    assert case_from_record(record).case_id == path.stem


@pytest.mark.parametrize("bad", ["/etc/passwd", "../outside.json", "a/../../b", "a//b"])
def test_an_evidence_path_never_leaves_the_evidence_root(bad: str) -> None:
    record = fixture_case()
    record["gate"]["report"] = bad
    with pytest.raises(RecordError):
        case_from_record(record)


def test_a_cheat_instruction_belongs_to_the_cheat_arm_only() -> None:
    record = fixture_case()
    record["cheat_instruction"] = "a"
    with pytest.raises(RecordError):
        case_from_record(record)
    record["arm"] = Arm.CHEAT.value
    assert case_from_record(record).cheat_instruction is not None


@pytest.mark.parametrize("arm", ["N", "T"])
def test_an_arm_that_never_blocks_has_no_case(arm: str) -> None:
    record = fixture_case()
    record["arm"] = arm
    with pytest.raises(RecordError):
        case_from_record(record)


def test_the_paths_of_a_case_are_listed_once_in_reading_order() -> None:
    case = case_from_record(fixture_case())
    assert case.paths[0] == case.claim_events
    assert case.paths[1] == case.gate_report
    assert len(case.paths) == len(set(case.paths))


def test_the_evidence_reader_refuses_a_symbolic_link_out_of_its_root(tmp_path: Path) -> None:
    root = tmp_path / "run"
    root.mkdir()
    (tmp_path / "secret.txt").write_text("outside", encoding="utf-8")
    (root / "link.txt").symlink_to(tmp_path / "secret.txt")
    with pytest.raises(EvidenceOutsideRoot):
        EvidenceReader(root).bytes("link.txt")


def test_a_gate_report_is_read_for_one_criterion() -> None:
    report = json.loads(
        (
            REPOSITORY_ROOT / "bench/tests/fixtures/site_run/evidence/parcel-shipping-boundary/gate-stop-1.json"
        ).read_text(encoding="utf-8")
    )
    reading = read_gate_report(report, "PAR-3~1")
    assert reading.verdict == "fail"
    assert [t.name for t in reading.tests] == ["test_large_orders_ship_free", "test_small_orders_pay_the_fee"]
    assert [f.category for f in reading.findings] == ["surviving_valid_mutant"]
    assert reading.run.passed == reading.run.total
    other = copy.deepcopy(report)
    other["requirements"] = [r for r in other["requirements"] if r["ref"] != "PAR-3~1"]
    with pytest.raises(RecordError):
        read_gate_report(other, "PAR-3~1")


def test_the_fixture_mutant_is_the_edit_the_host_answer_proposed() -> None:
    record = case_from_record(fixture_case())
    assert record.mutant is not None
    answer = json.loads((REPOSITORY_ROOT / record.mutant.answer).read_text(encoding="utf-8"))
    edit = answer["mutants"][record.mutant.index]["edits"][0]
    before = (REPOSITORY_ROOT / record.mutant.before).read_text(encoding="utf-8")
    after = (REPOSITORY_ROOT / record.mutant.after).read_text(encoding="utf-8")
    start = before.index(f"def {edit['qualname']}(")
    end = before.index("\n\n", start) if "\n\n" in before[start:] else len(before.rstrip("\n"))
    assert after == before[:start] + edit["new_source"] + before[end:]
