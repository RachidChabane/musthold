"""JUnit XML parsing: outcomes only, never bindings (PREREG 5)."""

from __future__ import annotations

import xml.etree.ElementTree as ElementTree
from pathlib import Path

from pilot.domain.constants import PYTEST_TIMEOUT_MESSAGE_PREFIX
from pilot.domain.outcomes import TestOutcome

_NODEID_SEPARATOR = "::"
_MODULE_SUFFIX = ".py"


def parse_outcomes(junit_path: Path, project_root: Path) -> dict[str, TestOutcome]:
    if not junit_path.is_file():
        return {}
    try:
        root = ElementTree.parse(junit_path).getroot()
    except ElementTree.ParseError:
        return {}
    return {
        _nodeid(project_root, case.get("classname", ""), case.get("name", "")): _outcome(case)
        for case in root.iter("testcase")
    }


def _outcome(case: ElementTree.Element) -> TestOutcome:
    if case.find("error") is not None:
        return TestOutcome.ERROR
    failure = case.find("failure")
    if failure is not None:
        timed_out = failure.get("message", "").startswith(PYTEST_TIMEOUT_MESSAGE_PREFIX)
        return TestOutcome.ERROR if timed_out else TestOutcome.FAILED
    if case.find("skipped") is not None:
        return TestOutcome.SKIPPED
    return TestOutcome.PASSED


def _nodeid(project_root: Path, classname: str, name: str) -> str:
    parts = [p for p in classname.split(".") if p]
    for split in range(len(parts), 0, -1):
        module = "/".join(parts[:split]) + _MODULE_SUFFIX
        if (project_root / module).is_file():
            return _NODEID_SEPARATOR.join([module, *parts[split:], name])
    return name
