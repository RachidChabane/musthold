"""Oracle classification on the mini fixture: real plain-pytest processes, no model."""

from __future__ import annotations

import json
import shutil

import pytest

from pilot.adapters.oracle_runner import OracleClassifier, failed_criteria, oracle_criteria
from pilot.domain.outcomes import TestOutcome

pytestmark = pytest.mark.integration
PASSED, FAILED = TestOutcome.PASSED, TestOutcome.FAILED


def write_run(run_dir, mini_root):
    (run_dir / "mutants").mkdir(parents=True)
    rows = []
    sources = {
        "aaaa000000000001": ("PARK-1_a", "OK", None),
        "aaaa000000000002": ("PARK-2_a", "OK", None),
        "aaaa000000000003": ("PARK-1_a", "DUPLICATE", "aaaa000000000001"),
        "aaaa000000000004": ("PARK-1_a", "TRIVIALLY_EQUIVALENT", None),
    }
    for mutant_id, (source, status, duplicate_of) in sources.items():
        edits = json.loads((mini_root / "prereg" / "violations" / f"{source}.json").read_text())["edits"]
        (run_dir / "mutants" / f"{mutant_id}.json").write_text(json.dumps({"mutant_id": mutant_id, "arm": "T", "edits": edits}))
        rows.append({"mutant_id": mutant_id, "stillborn": False, "status": status, "duplicate_of": duplicate_of})
    rows.append({"mutant_id": "aaaa000000000009", "stillborn": True, "status": "OK", "duplicate_of": None})
    (run_dir / "processing.jsonl").write_text("".join(json.dumps(r) + "\n" for r in rows))


def test_oracle_classify_runs_each_mutant_once_inherits_duplicates_and_resumes(tmp_path, mini_root):
    run_dir = tmp_path / "run"
    write_run(run_dir, mini_root)
    project = mini_root / "fixture" / "parkfee"
    oracle_files = sorted((mini_root / "prereg" / "oracle").glob("test_oracle_park_*.py"))
    classifier = OracleClassifier(project, oracle_files, run_dir)
    executed, reused, unresolved = classifier.classify(workers=2)
    assert (executed, reused, unresolved) == (2, 0, [])
    cache = json.loads((run_dir / "oracle_classes.json").read_text())["mutants"]
    assert cache["aaaa000000000001"]["failed_criteria"] == [1]
    assert cache["aaaa000000000002"]["failed_criteria"] == [2]
    assert cache["aaaa000000000003"]["source"] == "inherited:aaaa000000000001"
    assert cache["aaaa000000000003"]["failed_criteria"] == [1]
    assert cache["aaaa000000000004"]["failed_criteria"] == []
    assert "aaaa000000000009" not in cache
    assert OracleClassifier(project, oracle_files, run_dir).classify(workers=2) == (0, 4, [])


def test_a_changed_oracle_invalidates_the_cache(tmp_path, mini_root):
    run_dir = tmp_path / "run"
    write_run(run_dir, mini_root)
    project = mini_root / "fixture" / "parkfee"
    oracle_dir = tmp_path / "oracle"
    shutil.copytree(mini_root / "prereg" / "oracle", oracle_dir)
    files = sorted(oracle_dir.glob("test_oracle_park_*.py"))
    OracleClassifier(project, files, run_dir).classify(workers=2)
    files[2].write_text(files[2].read_text() + "\n# touched\n")
    executed, reused, _ = OracleClassifier(project, files, run_dir).classify(workers=2)
    assert executed == 2 and reused == 0


def test_a_file_with_no_reported_outcome_is_an_infrastructure_failure():
    with pytest.raises(Exception, match="no outcome"):
        failed_criteria({"tests/test_oracle_park_1.py::test_a": PASSED}, [1, 2])


def test_failed_errored_and_timed_out_files_count_as_failing_criteria():
    outcomes = {
        "tests/test_oracle_park_1.py::test_a[1]": PASSED,
        "tests/test_oracle_park_1.py::test_a[2]": FAILED,
        "tests/test_oracle_park_2.py": TestOutcome.ERROR,
        "tests/test_oracle_park_3.py::test_c": PASSED,
    }
    assert failed_criteria(outcomes, [1, 2, 3]) == {1: True, 2: True, 3: False}


def test_oracle_criteria_come_from_the_file_names(mini_root):
    files = sorted((mini_root / "prereg" / "oracle").glob("test_oracle_park_*.py"))
    assert oracle_criteria(files) == [1, 2, 3]
