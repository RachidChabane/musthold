"""The two ecosystems of the demos (pytest and Go), their test paths and the scan's own detection (PREREG 6.4)."""

from __future__ import annotations

import fnmatch
from collections.abc import Callable, Collection
from dataclasses import dataclass
from enum import StrEnum
from typing import Final


class Ecosystem(StrEnum):
    PYTEST = "pytest"
    GO = "go"


ECOSYSTEM_ORDER: Final = (Ecosystem.PYTEST, Ecosystem.GO)
"""Lists are built, and candidates attempted, pytest first."""

SEARCH_LANGUAGE: Final = {Ecosystem.PYTEST: "Python", Ecosystem.GO: "Go"}
TEST_GLOBS: Final = {Ecosystem.PYTEST: ("test_*.py", "*_test.py"), Ecosystem.GO: ("*_test.go",)}
SOURCE_SUFFIX: Final = {Ecosystem.PYTEST: ".py", Ecosystem.GO: ".go"}
VENDOR_SEGMENT: Final = "vendor"
CONFTEST: Final = "conftest.py"
CHANGED_STATUSES: Final = frozenset({"added", "modified", "renamed"})

PYTEST_INI: Final = "pytest.ini"
TOX_INI: Final = "tox.ini"
SETUP_CFG: Final = "setup.cfg"
PYPROJECT: Final = "pyproject.toml"
GO_MOD: Final = "go.mod"

PYTEST_SECTIONS: Final = ((TOX_INI, "[pytest]"), (SETUP_CFG, "[tool:pytest]"), (PYPROJECT, "[tool.pytest"))
"""The configuration files whose section string makes a root pytest, in the order they are read."""


def basename(path: str) -> str:
    return path.rsplit("/", 1)[-1]


def has_vendor_segment(path: str) -> bool:
    return VENDOR_SEGMENT in path.split("/")


def is_test_path(ecosystem: Ecosystem, path: str) -> bool:
    """Filter 5: a path the scan's test globs match by basename, outside every vendor directory."""
    if has_vendor_segment(path):
        return False
    name = basename(path)
    return any(fnmatch.fnmatchcase(name, glob) for glob in TEST_GLOBS[ecosystem])


def is_source_path(ecosystem: Ecosystem, path: str) -> bool:
    """Filter 6: a source file of the ecosystem that is neither a test file nor conftest.py, outside vendor."""
    if has_vendor_segment(path) or is_test_path(ecosystem, path):
        return False
    return path.endswith(SOURCE_SUFFIX[ecosystem]) and basename(path) != CONFTEST


@dataclass(frozen=True)
class ChangedFile:
    path: str
    status: str

    def counts_as_changed(self) -> bool:
        return self.status in CHANGED_STATUSES


type ConfigReader = Callable[[str], str | None]
"""Reads one root configuration file; None when it cannot be read."""


@dataclass(frozen=True)
class Detection:
    ecosystem: Ecosystem | None
    unreadable: str | None = None
    """The configuration file whose read failed, which stops the detection."""


def detect(root_names: Collection[str], read_config: ConfigReader) -> Detection:
    """The first ecosystem the scan's registry recognizes at a root: pytest is tested before Go.

    Only the root listing and the three configuration files are read, and conftest.py only for its presence.
    """
    if PYTEST_INI in root_names or CONFTEST in root_names:
        return Detection(Ecosystem.PYTEST)
    for name, section in PYTEST_SECTIONS:
        if name not in root_names:
            continue
        content = read_config(name)
        if content is None:
            return Detection(None, unreadable=name)
        if section in content:
            return Detection(Ecosystem.PYTEST)
    if GO_MOD in root_names:
        return Detection(Ecosystem.GO)
    return Detection(None)
