from __future__ import annotations

from pathlib import Path

from pilot.adapters.junit import parse_outcomes
from pilot.adapters.plain_pytest import parse_summary
from pilot.adapters.shallnot import parse_report, shallnot_argv
from pilot.domain.outcomes import TestOutcome
from pilot.domain.traceability import TraceabilityReport, null_gate_failures
JUNIT = """<?xml version="1.0" encoding="utf-8"?>
<testsuites><testsuite name="pytest" tests="6">
<testcase classname="tests.test_a" name="test_pass" time="0.1"/>
<testcase classname="tests.test_a" name="test_fail"><failure message="assert 1 == 2">x</failure></testcase>
<testcase classname="tests.test_a" name="test_timeout"><failure message="Failed: Timeout &gt;10.0s">x</failure></testcase>
<testcase classname="tests.test_a" name="test_error"><error message="boom">x</error></testcase>
<testcase classname="tests.test_a" name="test_skip"><skipped message="s"/></testcase>
<testcase classname="tests.test_b.TestGroup" name="test_method"/>
</testsuite></testsuites>
"""


def test_junit_outcomes_are_keyed_by_pytest_node_id(tmp_path):
    (tmp_path / "tests").mkdir()
    (tmp_path / "tests" / "test_a.py").write_text("")
    (tmp_path / "tests" / "test_b.py").write_text("")
    junit = tmp_path / "junit.xml"
    junit.write_text(JUNIT)
    assert parse_outcomes(junit, tmp_path) == {
        "tests/test_a.py::test_pass": TestOutcome.PASSED,
        "tests/test_a.py::test_fail": TestOutcome.FAILED,
        "tests/test_a.py::test_timeout": TestOutcome.ERROR,
        "tests/test_a.py::test_error": TestOutcome.ERROR,
        "tests/test_a.py::test_skip": TestOutcome.SKIPPED,
        "tests/test_b.py::TestGroup::test_method": TestOutcome.PASSED,
    }


def test_missing_or_broken_junit_yields_no_outcomes(tmp_path):
    assert parse_outcomes(tmp_path / "none.xml", tmp_path) == {}
    broken = tmp_path / "broken.xml"
    broken.write_text("<testsuites>")
    assert parse_outcomes(broken, tmp_path) == {}


SUMMARY = """.F
=================================== FAILURES ===================================
FAILED tests/not_a_summary_line.py::inside_failure_section
=========================== short test summary info ============================
PASSED tests/test_a.py::test_one
FAILED tests/test_a.py::test_two - assert 1 == 2
ERROR tests/test_b.py - ImportError: nope
SKIPPED [1] tests/test_a.py:3: reason
2 passed, 1 failed
"""


def test_plain_summary_parsing_reads_only_the_summary_section():
    assert parse_summary(SUMMARY) == {
        "tests/test_a.py::test_one": TestOutcome.PASSED,
        "tests/test_a.py::test_two": TestOutcome.FAILED,
        "tests/test_b.py": TestOutcome.ERROR,
    }


REPORT = {
    "verdict": "pass",
    "summary": {
        "requirements": {"in_focus": 2, "covered": 2},
        "tests": {"total": 3, "bound": 3, "untagged": 0},
    },
    "requirements": [
        {"ref": "PARK-1~1", "tests": [{"name": "t1"}, {"name": "t2"}]},
        {"ref": "PARK-2~1", "tests": [{"name": "t3"}]},
    ],
    "findings": [],
}


def test_shallnot_report_parsing_extracts_bindings():
    report = parse_report(REPORT)
    assert report.bindings == {"t1": ("PARK-1~1",), "t2": ("PARK-1~1",), "t3": ("PARK-2~1",)}
    assert (report.verdict, report.tests_bound, report.requirements_covered) == ("pass", 3, 2)


def test_shallnot_argv_is_the_contracted_command():
    argv = shallnot_argv(Path("/r/junit.xml"), Path("/r/out.json"))
    assert " ".join(argv) == (
        "shallnot check --no-config --strict --specs spec --focus spec --tests tests "
        "--results /r/junit.xml --format json --json-out /r/out.json"
    )


def make_report(**overrides):
    fields = dict(
        verdict="pass", requirements_in_focus=2, requirements_covered=2, tests_total=3,
        tests_bound=3, finding_categories=(), bindings={},
    )
    return TraceabilityReport(**{**fields, **overrides})


def test_null_gate_precondition_holds_for_a_clean_report():
    assert null_gate_failures(make_report(), 3, 2) == []


def test_null_gate_precondition_names_every_violation():
    report = make_report(
        verdict="fail", tests_bound=2, requirements_covered=1,
        finding_categories=("orphan_tag", "untagged_test", "revision_mismatch"),
    )
    failures = null_gate_failures(report, 3, 2)
    assert "tests bound 2/3, expected 3/3" in failures
    assert "criteria covered 1/2, expected 2/2" in failures
    assert "finding orphan_tag" in failures and "finding revision_mismatch" in failures
    assert "verdict fail" in failures
    assert not any("untagged_test" in f for f in failures)
