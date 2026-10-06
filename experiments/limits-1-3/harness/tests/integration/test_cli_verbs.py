from __future__ import annotations

import json
import shutil

import pytest
import yaml

from pilot.adapters.prereg_files import read_label_matrix, write_label_matrix
from pilot.cli.main import main

pytestmark = pytest.mark.integration
MINI_COUNTS = ["--expected-tests", "8", "--expected-violations", "6", "--expected-planted", "2", "--expected-criteria", "3"]
VACUOUS_NODE = "tests/test_mini.py::test_exit_before_entry_is_rejected_consistently"


@pytest.fixture
def root(mini_root, tmp_path):
    copy = tmp_path / "mini"
    shutil.copytree(mini_root, copy)
    return copy


def verify(root, *extra):
    return main(["verify-labels", "--root", str(root), *MINI_COUNTS, *extra])


def test_verify_labels_writes_the_matrix_and_the_log(root, capsys):
    assert verify(root) == 0
    matrix = read_label_matrix(root / "prereg" / "label_matrix.csv")
    assert matrix.variant_ids == ("PARK-1_a", "PARK-1_b", "PARK-2_a", "PARK-2_b", "PARK-3_a", "PARK-3_b", "R")
    assert len(matrix.rows) == 8
    assert set(matrix.rows[VACUOUS_NODE]) == {"pass"}
    log = (root / "prereg" / "verify_log.txt").read_text()
    assert log.splitlines()[-1] == "RESULT: PASS"
    assert "verify-labels: OK" in capsys.readouterr().out


def test_verify_labels_is_reproducible(root):
    verify(root)
    first = (root / "prereg" / "label_matrix.csv").read_text(), (root / "prereg" / "verify_log.txt").read_text()
    verify(root)
    assert first == ((root / "prereg" / "label_matrix.csv").read_text(), (root / "prereg" / "verify_log.txt").read_text())


def test_verify_labels_fails_and_names_the_condition_when_a_label_is_wrong(root, capsys):
    path = root / "prereg" / "labels.yaml"
    entries = yaml.safe_load(path.read_text())
    for entry in entries:
        if entry["nodeid"] == VACUOUS_NODE:
            entry["label"] = "GOOD"
    path.write_text(yaml.safe_dump(entries))
    assert verify(root) == 1
    err = capsys.readouterr().err
    assert f"[4] GOOD {VACUOUS_NODE}: does not fail under every violation of 1" in err
    assert "RESULT: FAIL" in (root / "prereg" / "verify_log.txt").read_text()


def test_verify_labels_fails_when_the_counts_do_not_match(root, capsys):
    assert main(["verify-labels", "--root", str(root), "--expected-tests", "38", "--expected-violations", "6",
                 "--expected-planted", "2", "--expected-criteria", "3"]) == 1
    assert "labelled tests: 8, expected 38" in capsys.readouterr().err


def test_verify_labels_fails_when_an_oracle_stops_being_diagonal(root, capsys):
    (root / "prereg" / "oracle" / "test_oracle_park_2.py").write_text(
        "def test_oracle_always_passes():\n    assert True\n"
    )
    assert verify(root) == 1
    assert "[2] oracle 2 passes on PARK-2_a" in capsys.readouterr().err


def test_runner_crosscheck_reproduces_the_committed_matrix(root, capsys):
    verify(root)
    assert main(["runner-crosscheck", "--root", str(root)]) == 0
    assert "runner-crosscheck: OK" in capsys.readouterr().out


def test_runner_crosscheck_reports_a_flipped_cell(root, capsys):
    verify(root)
    path = root / "prereg" / "label_matrix.csv"
    matrix = read_label_matrix(path)
    flipped = {k: tuple("fail" if (k == VACUOUS_NODE and i == 0) else c for i, c in enumerate(v)) for k, v in matrix.rows.items()}
    write_label_matrix(path, type(matrix)(matrix.variant_ids, flipped))
    assert main(["runner-crosscheck", "--root", str(root)]) == 1
    assert f"{VACUOUS_NODE} x PARK-1_a: fail != pass" in capsys.readouterr().err


def test_leak_check_passes_on_the_mini_fixture(root, capsys):
    assert main(["leak-check", "--root", str(root), "--test-files", "test_mini.py", "--expected-tests", "8"]) == 0


def test_leak_check_fails_on_a_comment(root, capsys):
    path = root / "fixture" / "parkfee" / "tests" / "test_mini.py"
    path.write_text(path.read_text().replace("def at(minutes):", "# helper\ndef at(minutes):"))
    assert main(["leak-check", "--root", str(root), "--test-files", "test_mini.py", "--expected-tests", "8"]) == 1
    assert "test_mini.py:11: comment" in capsys.readouterr().err


def test_traceability_check_passes_on_the_mini_fixture(root, tmp_path):
    out = tmp_path / "out"
    assert main(["traceability-check", "--root", str(root), "--expected-tests", "8", "--expected-criteria", "3", "--out-dir", str(out)]) == 0
    assert (out / "traceability.json").is_file() and (out / "junit.xml").is_file()


def test_traceability_check_fails_on_an_unbound_test(root, capsys):
    path = root / "fixture" / "parkfee" / "tests" / "test_mini.py"
    path.write_text(path.read_text().replace('@pytest.mark.verifies("PARK-2~1")\n', ""))
    code = main(["traceability-check", "--root", str(root), "--expected-tests", "8", "--expected-criteria", "3"])
    assert code == 1
    assert "tests bound 7/8" in capsys.readouterr().err


def test_canary_refuses_to_run_without_the_system_prompt(root, capsys):
    assert main(["canary", "--root", str(root), "--run-id", "r1"]) == 1
    assert "system.txt" in capsys.readouterr().err


def test_assemble_arm_p_writes_items_labels_and_the_planted_map_deterministically(root):
    assert main(["assemble-arm-p", "--root", str(root)]) == 0
    items = root / "prereg" / "arm_p" / "items"
    first = {p.name: p.read_text() for p in items.iterdir()}
    labels = yaml.safe_load((root / "prereg" / "arm_p" / "labels.yaml").read_text())
    assert len(labels) == 8 and set(first) == {f"{k}.json" for k in labels} | {"planted_map.json"}
    assert {entry["kind"] for entry in labels.values()} == {"r", "n1"}
    assert set(next(iter(labels.values()))) == {"source", "declared_target", "kind"}
    assert main(["assemble-arm-p", "--root", str(root)]) == 0
    assert first == {p.name: p.read_text() for p in items.iterdir()}
    for mutant_id, entry in labels.items():
        body = json.loads((items / f"{mutant_id}.json").read_text())
        assert list(body) == ["edits"] and json.loads((items / "planted_map.json").read_text())[mutant_id] == entry["source"]


def test_run_core_refuses_to_run_without_the_prereg_prompts(root, capsys):
    assert main(["run-core", "--root", str(root), "--run-id", "r1"]) == 1
    assert "MissingPreregFile" in capsys.readouterr().err
