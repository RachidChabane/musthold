"""H: per-criterion property and example tests written from the spec and its Interface alone.

The writer returns files; this module checks them against the layout and determinism rules and derives the oracle
manifest entries from the tests' own names, so a test's criterion is never a claim the model makes separately.
"""

from __future__ import annotations

import posixpath
import re
from collections.abc import Sequence
from dataclasses import dataclass
from typing import Final

from specbench.domain.changed_tests import test_ids
from specbench.domain.jsonread import mapping, sequence, text
from specbench.domain.languages import Language

PYTHON_H_DIRECTORY: Final = "tests/spec_h"
PYTHON_H_FILE: Final = re.compile(r"^test_spec_h_[a-z0-9_]+\.py$")
GO_H_FILE: Final = re.compile(r"^h_[a-z0-9_]+_test\.go$")
PYTHON_TEST_NAME: Final = re.compile(r"(?:^|::)test_c(?P<number>[1-9][0-9]*)_[A-Za-z0-9_]+$")
GO_TEST_NAME: Final = re.compile(r"::TestH(?P<number>[1-9][0-9]*)_[A-Za-z0-9_]+$")
GO_IMPORT: Final = re.compile(r'^\s*(?:import\s+)?(?:[A-Za-z_.]+\s+)?"([^"]+)"\s*$', re.MULTILINE)
MAX_FILES: Final = 12


@dataclass(frozen=True)
class HFile:
    path: str
    content: str


@dataclass(frozen=True)
class HTest:
    test_id: str
    number: int


def h_files_from_record(record: object) -> tuple[HFile, ...]:
    body = mapping(record, "h tests")
    return tuple(
        HFile(text(mapping(f, "file").get("path"), "path"), text(mapping(f, "file").get("content"), "content"))
        for f in sequence(body.get("files"), "files")
    )


def go_directory_of(import_path: str, module: str) -> str | None:
    """The module-relative directory of a package import path, or None when it lies outside the module."""
    if import_path == module:
        return "."
    return import_path.removeprefix(module + "/") if import_path.startswith(module + "/") else None


def _layout_problem(language: Language, file: HFile, directories: frozenset[str]) -> str | None:
    name = posixpath.basename(file.path)
    directory = posixpath.dirname(file.path) or "."
    if language is Language.PYTHON:
        if directory != PYTHON_H_DIRECTORY or PYTHON_H_FILE.match(name) is None:
            return f"{file.path}: Python H files live at {PYTHON_H_DIRECTORY}/test_spec_h_<name>.py"
        return None
    if directory not in directories or GO_H_FILE.match(name) is None:
        return f"{file.path}: Go H files are named h_<name>_test.go inside a package directory of the Interface"
    return None


def _determinism_problems(language: Language, file: HFile, module: str) -> list[str]:
    problems: list[str] = []
    if language is Language.PYTHON:
        if "hypothesis" in file.content and "derandomize=True" not in file.content:
            problems.append(f"{file.path}: Hypothesis tests must set derandomize=True")
        return problems
    for imported in GO_IMPORT.findall(file.content):
        first = imported.split("/")[0]
        if "." in first and not imported.startswith(module):
            problems.append(f"{file.path}: Go H tests import the standard library and the module only, not {imported}")
    if "math/rand" in file.content and "NewSource(" not in file.content:
        problems.append(f"{file.path}: randomised Go H tests must use rand.New(rand.NewSource(<fixed seed>))")
    return problems


def h_problems(
    language: Language, files: Sequence[HFile], criteria_count: int, directories: frozenset[str], module: str
) -> tuple[list[str], list[HTest]]:
    """Problems with the written files, and the tests they define with the criterion each name states."""
    problems: list[str] = []
    tests: list[HTest] = []
    if not files or len(files) > MAX_FILES:
        problems.append(f"{len(files)} H files, 1 to {MAX_FILES} required")
    pattern = PYTHON_TEST_NAME if language is Language.PYTHON else GO_TEST_NAME
    for file in files:
        layout = _layout_problem(language, file, directories)
        if layout is not None:
            problems.append(layout)
            continue
        problems += _determinism_problems(language, file, module)
        for test_id in test_ids(language, file.path, file.content):
            match = pattern.search(test_id)
            if match is None:
                problems.append(f"{test_id}: name must state its criterion (test_c<n>_... or TestH<n>_...)")
            else:
                tests.append(HTest(test_id, int(match.group("number"))))
    covered = {t.number for t in tests}
    problems += [f"criterion {n} has no H test" for n in range(1, criteria_count + 1) if n not in covered]
    problems += [
        f"{t.test_id}: names criterion {t.number}, which does not exist" for t in tests if t.number > criteria_count
    ]
    if len({t.test_id for t in tests}) != len(tests):
        problems.append("H test ids repeat")
    return problems, tests
