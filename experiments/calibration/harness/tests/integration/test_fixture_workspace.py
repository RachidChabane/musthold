from __future__ import annotations

import json
import os
import sys
from pathlib import Path

import pytest
import yaml

from calibration.adapters.fixture_workspace import CONFIG_FILE, PytestFixtureWorkspace
from real_process_runner import RealProcessRunner
from repository_paths import PILOT_FIXTURE

pytestmark = pytest.mark.integration

SHALLNOT_ENV = "CALIBRATION_TEST_SHALLNOT"
CRITERIA_COUNT = 10
BOUND_TESTS = 38
GATE_TIMEOUT_SECONDS = 600
NICE_PREFIX = ("nice", "-n", "10")


def make_fixture(root: Path) -> Path:
    fixture = root / "parkfee"
    (fixture / "src" / "parkfee" / "__pycache__").mkdir(parents=True)
    (fixture / "src" / "parkfee" / "__pycache__" / "x.pyc").write_bytes(b"cache")
    (fixture / "src" / "parkfee" / "code.py").write_text("VALUE = 1\n")
    (fixture / "tests").mkdir()
    (fixture / "tests" / "test_code.py").write_text("def test_value():\n    assert True\n")
    (fixture / "spec").mkdir()
    (fixture / "spec" / "parking.md").write_text("# spec\n")
    (fixture / ".pytest_cache").mkdir()
    (fixture / ".pytest_cache" / "v").write_text("cache")
    (fixture / "junit.xml").write_text("<testsuites/>")
    (fixture / "junit-old.xml").write_text("<testsuites/>")
    return fixture


def relative_files(root: Path) -> set[str]:
    return {str(path.relative_to(root)) for path in root.rglob("*") if path.is_file()}


def test_materialize_copies_the_fixture_without_caches_and_junit_files(tmp_path: Path) -> None:
    fixture = make_fixture(tmp_path)
    copy = PytestFixtureWorkspace(sys.executable).materialize(fixture, tmp_path / "copy")
    assert copy == tmp_path / "copy"
    assert relative_files(copy) == {
        "src/parkfee/code.py",
        "tests/test_code.py",
        "spec/parking.md",
        CONFIG_FILE,
    }
    assert relative_files(fixture) >= {"junit.xml", "junit-old.xml", "src/parkfee/__pycache__/x.pyc"}


def test_materialize_writes_the_configuration_the_gate_needs(tmp_path: Path) -> None:
    copy = PytestFixtureWorkspace("/opt/with space/python").materialize(make_fixture(tmp_path), tmp_path / "copy")
    assert yaml.safe_load((copy / CONFIG_FILE).read_text()) == {
        "version": 1,
        "specs": ["spec"],
        "focus": ["spec"],
        "tests": ["tests"],
        "results": ["junit.xml"],
        "test_commands": ["/opt/with space/python -m pytest -p no:cacheprovider --timeout=10 --junitxml=junit.xml"],
    }


def test_materialize_refuses_an_existing_destination(tmp_path: Path) -> None:
    fixture = make_fixture(tmp_path)
    (tmp_path / "copy").mkdir()
    with pytest.raises(FileExistsError):
        PytestFixtureWorkspace(sys.executable).materialize(fixture, tmp_path / "copy")


@pytest.mark.skipif(not Path(os.environ.get(SHALLNOT_ENV, "")).is_file(), reason=f"{SHALLNOT_ENV} names no executable")
def test_the_gate_lists_the_ten_criteria_and_the_bound_tests_of_the_pilot_fixture(tmp_path: Path) -> None:
    shallnot = os.environ[SHALLNOT_ENV]
    project = PytestFixtureWorkspace(sys.executable).materialize(PILOT_FIXTURE, tmp_path / "project")
    report = tmp_path / "report.json"
    result = RealProcessRunner().run(
        [*NICE_PREFIX, shallnot, "gate", "--advisory", "--json-out", str(report)],
        cwd=project,
        timeout_seconds=GATE_TIMEOUT_SECONDS,
    )
    assert result.returncode == 0, result.stderr.decode()
    document = json.loads(report.read_text())
    assert document["summary"]["requirements"]["in_focus"] == CRITERIA_COUNT
    assert len(document["requirements"]) == CRITERIA_COUNT
    assert document["summary"]["tests"]["bound"] == BOUND_TESTS
