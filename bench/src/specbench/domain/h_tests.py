"""H: per-criterion property and example tests written from the spec and its Interface alone.

The writer returns files; this module checks them against the layout, isolation and determinism rules and derives each
candidate test from the files themselves: its criterion from its name, its kind (property or example) from its code.
Neither is a claim the model makes separately.

One writer call is a *writing*: the pool (round 0) asks for every criterion, a refill (round k >= 1) for the criteria
the selection left short. Each file holds the tests of one criterion and its own helpers, so that one broken helper
costs one criterion's candidates, never the whole task's.
"""

from __future__ import annotations

import ast
import posixpath
import re
import sys
from collections.abc import Iterable, Sequence
from dataclasses import dataclass
from enum import StrEnum
from typing import Final

from specbench.domain.changed_tests import go_test_spans, python_test_spans, test_ids
from specbench.domain.jsonread import mapping, sequence, text
from specbench.domain.languages import Language

PYTHON_H_DIRECTORY: Final = "tests/spec_h"
PYTHON_H_FILE: Final = re.compile(r"^test_spec_h_(?:r(?P<round>[1-9][0-9]*)_)?c(?P<number>[1-9][0-9]*)_[a-z0-9_]+\.py$")
GO_H_FILE: Final = re.compile(r"^h_(?:r(?P<round>[1-9][0-9]*)_)?c(?P<number>[1-9][0-9]*)_[a-z0-9_]+_test\.go$")
PYTHON_TEST_NAME: Final = re.compile(r"(?:^|::)test_c(?P<number>[1-9][0-9]*)_[A-Za-z0-9_]+$")
GO_TEST_NAME: Final = re.compile(r"::TestH(?P<number>[1-9][0-9]*)_[A-Za-z0-9_]+$")
GO_IMPORT: Final = re.compile(r'^\s*(?:import\s+)?(?:[A-Za-z_.]+\s+)?"([^"]+)"\s*$', re.MULTILINE)
MAX_FILES: Final = 12
POOL_ROUND: Final = 0
POOL_SIZE: Final = 4
"""Candidates the writer returns per criterion it is asked for."""
MIN_PROPERTY_TESTS: Final = 1
MIN_EXAMPLE_TESTS: Final = 2

PYTHON_TEST_PACKAGES: Final = frozenset({"pytest", "hypothesis"})
PYTHON_PROPERTY_DECORATOR: Final = "given"
GO_PROPERTY_MARKERS: Final = ("quick.Check", "rand.New(")
"""A Go test is a property test when its own body checks generated inputs: `testing/quick` or a seeded generator."""
GO_TOP_LEVEL_FUNCTION: Final = re.compile(r"^func\s+([A-Za-z_][A-Za-z0-9_]*)\s*[(\[]", re.MULTILINE)
GO_TOP_LEVEL_DECLARATION: Final = re.compile(
    r"^(?:var|const|type)\s+([A-Za-z_][A-Za-z0-9_]*(?:\s*,\s*[A-Za-z_][A-Za-z0-9_]*)*)", re.MULTILINE
)
GO_GROUP_START: Final = re.compile(r"^(?:var|const|type)\s*\(\s*$")
GO_GROUP_END: Final = ")"
GO_GROUP_MEMBER: Final = re.compile(r"^(?:\t| {4})([A-Za-z_][A-Za-z0-9_]*(?:\s*,\s*[A-Za-z_][A-Za-z0-9_]*)*)")
GO_REPEATABLE_IDENTIFIERS: Final = frozenset({"_", "init"})
IDENTIFIER_SEPARATOR: Final = re.compile(r"\s*,\s*")


class TestKind(StrEnum):
    """What a candidate checks: a rule over generated inputs, or one hand-picked case."""

    __test__ = False

    PROPERTY = "property"
    EXAMPLE = "example"


@dataclass(frozen=True)
class HFile:
    path: str
    content: str


@dataclass(frozen=True)
class HTest:
    """One candidate: its id as the runners report it, its criterion, its kind and the file that holds it."""

    test_id: str
    number: int
    kind: TestKind = TestKind.EXAMPLE
    path: str = ""
    round: int = POOL_ROUND


@dataclass(frozen=True)
class HWriting:
    """What one writer call is asked for: the criteria to write candidates for, and the round (0: the pool)."""

    criteria: tuple[int, ...]
    round: int = POOL_ROUND


@dataclass(frozen=True)
class HLayout:
    """The facts of the task the checks need: its language, Go module and packages, Python packages of the Interface."""

    language: Language
    module: str
    go_directories: frozenset[str]
    python_packages: frozenset[str]


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


def h_layout_of(language: Language, module: str, import_paths: Iterable[str]) -> HLayout:
    """The layout facts of a task from the import paths its Interface names."""
    paths = [p.strip() for p in import_paths if p.strip()]
    directories = frozenset(d for p in paths if (d := go_directory_of(p, module)) is not None)
    packages = frozenset(p.split(".")[0] for p in paths)
    return HLayout(language, module, directories, packages)


def file_name_rule(language: Language, writing_round: int) -> str:
    """The file naming rule of a writing, as the repair message states it."""
    prefix = "" if writing_round == POOL_ROUND else f"r{writing_round}_"
    if language is Language.PYTHON:
        return f"{PYTHON_H_DIRECTORY}/test_spec_h_{prefix}c<n>_<name>.py"
    return f"h_{prefix}c<n>_<name>_test.go inside a package directory of the Interface"


def _file_match(language: Language, path: str) -> re.Match[str] | None:
    pattern = PYTHON_H_FILE if language is Language.PYTHON else GO_H_FILE
    return pattern.match(posixpath.basename(path))


def _layout_problem(layout: HLayout, writing: HWriting, file: HFile) -> str | None:
    directory = posixpath.dirname(file.path) or "."
    match = _file_match(layout.language, file.path)
    allowed = (
        directory == PYTHON_H_DIRECTORY if layout.language is Language.PYTHON else directory in layout.go_directories
    )
    file_round = int(match.group("round") or POOL_ROUND) if match is not None else None
    if not allowed or file_round != writing.round:
        return f"{file.path}: H files are named {file_name_rule(layout.language, writing.round)}"
    return None


def file_criterion(language: Language, path: str) -> int | None:
    """The criterion a file's name states, or None when the name follows no H layout."""
    match = _file_match(language, path)
    return int(match.group("number")) if match is not None else None


def _go_import_problems(file: HFile, module: str) -> list[str]:
    problems: list[str] = []
    for imported in GO_IMPORT.findall(file.content):
        first = imported.split("/")[0]
        if "." in first and not imported.startswith(module):
            problems.append(f"{file.path}: Go H tests import the standard library and the module only, not {imported}")
    return problems


def _imported_modules(tree: ast.Module) -> Iterable[tuple[str, int]]:
    """Each imported module of a parsed file, with its relative level (0 for an absolute import)."""
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            for alias in node.names:
                yield alias.name, 0
        elif isinstance(node, ast.ImportFrom):
            yield node.module or "", node.level


def python_import_problems(file: HFile, packages: frozenset[str]) -> list[str]:
    """Imports outside the standard library, pytest, Hypothesis and the Interface's top-level packages."""
    try:
        tree = ast.parse(file.content)
    except SyntaxError as error:
        return [f"{file.path}: does not parse as Python ({error.msg}, line {error.lineno})"]
    allowed = frozenset(sys.stdlib_module_names) | PYTHON_TEST_PACKAGES | packages
    problems: list[str] = []
    for name, level in _imported_modules(tree):
        if level:
            problems.append(f"{file.path}: relative import; an H file imports no other test file")
        elif name.split(".")[0] not in allowed:
            problems.append(
                f"{file.path}: imports {name}, outside the standard library, pytest, Hypothesis and the Interface"
            )
    return list(dict.fromkeys(problems))


def _determinism_problems(language: Language, file: HFile) -> list[str]:
    if language is Language.PYTHON:
        if "hypothesis" in file.content and "derandomize=True" not in file.content:
            return [f"{file.path}: Hypothesis tests must set derandomize=True"]
        return []
    if "math/rand" in file.content and "NewSource(" not in file.content:
        return [f"{file.path}: randomised Go H tests must use rand.New(rand.NewSource(<fixed seed>))"]
    return []


def _decorator_name(node: ast.expr) -> str:
    target = node.func if isinstance(node, ast.Call) else node
    if isinstance(target, ast.Attribute):
        return target.attr
    return target.id if isinstance(target, ast.Name) else ""


def _python_property_tests(path: str, source: str) -> frozenset[str]:
    try:
        tree = ast.parse(source)
    except SyntaxError:
        return frozenset()
    found: set[str] = set()

    def visit(body: list[ast.stmt], prefix: tuple[str, ...]) -> None:
        for node in body:
            if isinstance(node, ast.ClassDef):
                visit(node.body, (*prefix, node.name))
            elif isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and any(
                _decorator_name(d) == PYTHON_PROPERTY_DECORATOR for d in node.decorator_list
            ):
                found.add("::".join([path, *prefix, node.name]))

    visit(tree.body, ())
    return frozenset(found)


def _go_property_tests(path: str, source: str) -> frozenset[str]:
    lines = source.splitlines()
    return frozenset(
        test_id
        for test_id, (start, end) in go_test_spans(path, source).items()
        if any(marker in "\n".join(lines[start - 1 : end]) for marker in GO_PROPERTY_MARKERS)
    )


def candidate_tests(language: Language, file: HFile) -> list[HTest]:
    """Every test of one H file with the criterion its name states (0 when it states none) and its kind."""
    pattern = PYTHON_TEST_NAME if language is Language.PYTHON else GO_TEST_NAME
    properties = (
        _python_property_tests(file.path, file.content)
        if language is Language.PYTHON
        else _go_property_tests(file.path, file.content)
    )
    named = _file_match(language, file.path)
    file_round = int(named.group("round") or POOL_ROUND) if named is not None else POOL_ROUND
    tests: list[HTest] = []
    for test_id in test_ids(language, file.path, file.content):
        match = pattern.search(test_id)
        number = int(match.group("number")) if match is not None else 0
        kind = TestKind.PROPERTY if test_id in properties else TestKind.EXAMPLE
        tests.append(HTest(test_id, number, kind, file.path, file_round))
    return tests


def go_top_level_identifiers(source: str) -> list[str]:
    """The package-level names a Go file declares (functions, types, variables, constants), methods excluded."""
    names = [*GO_TOP_LEVEL_FUNCTION.findall(source)]
    names += [n for group in GO_TOP_LEVEL_DECLARATION.findall(source) for n in IDENTIFIER_SEPARATOR.split(group)]
    in_group = False
    for line in source.splitlines():
        if GO_GROUP_START.match(line):
            in_group = True
        elif in_group and line.startswith(GO_GROUP_END):
            in_group = False
        elif in_group and (member := GO_GROUP_MEMBER.match(line)) is not None:
            names += IDENTIFIER_SEPARATOR.split(member.group(1))
    return [n for n in names if n not in GO_REPEATABLE_IDENTIFIERS]


def go_collision_problems(files: Sequence[HFile], existing: Sequence[HFile] = ()) -> list[str]:
    """Top-level identifiers declared twice among the H files of one package: the package would not build."""
    declared: dict[tuple[str, str], str] = {}
    for file in existing:
        for name in go_top_level_identifiers(file.content):
            declared.setdefault((posixpath.dirname(file.path) or ".", name), file.path)
    problems: list[str] = []
    for file in files:
        package = posixpath.dirname(file.path) or "."
        for name in dict.fromkeys(go_top_level_identifiers(file.content)):
            if (package, name) in declared:
                problems.append(
                    f"{file.path}: top-level identifier {name} is declared by another H file of package {package}; "
                    "start every helper's name with the file's own prefix"
                )
            else:
                declared[(package, name)] = file.path
    return problems


def _file_problems(layout: HLayout, writing: HWriting, file: HFile) -> tuple[list[str], list[HTest]]:
    layout_problem = _layout_problem(layout, writing, file)
    if layout_problem is not None:
        return [layout_problem], []
    problems = _determinism_problems(layout.language, file)
    if layout.language is Language.PYTHON:
        problems += python_import_problems(file, layout.python_packages)
    else:
        problems += _go_import_problems(file, layout.module)
    owner = file_criterion(layout.language, file.path)
    tests = candidate_tests(layout.language, file)
    for test in tests:
        if test.number == 0:
            problems.append(f"{test.test_id}: name must state its criterion (test_c<n>_... or TestH<n>_...)")
        elif test.number != owner:
            problems.append(f"{test.test_id}: criterion {test.number} in a file of criterion {owner}")
    return problems, [t for t in tests if t.number]


def _pool_problems(writing: HWriting, tests: Sequence[HTest]) -> list[str]:
    problems: list[str] = []
    for number in writing.criteria:
        own = [t for t in tests if t.number == number]
        properties = sum(t.kind is TestKind.PROPERTY for t in own)
        if len(own) != POOL_SIZE:
            problems.append(f"criterion {number} has {len(own)} H tests, {POOL_SIZE} required")
        if properties < MIN_PROPERTY_TESTS:
            problems.append(f"criterion {number} has no property test, {MIN_PROPERTY_TESTS} required")
        if len(own) - properties < MIN_EXAMPLE_TESTS:
            problems.append(
                f"criterion {number} has {len(own) - properties} example tests, {MIN_EXAMPLE_TESTS} required"
            )
    asked = set(writing.criteria)
    problems += [
        f"{t.test_id}: names criterion {t.number}, which was not asked for" for t in tests if t.number not in asked
    ]
    return problems


def h_problems(
    layout: HLayout, writing: HWriting, files: Sequence[HFile], existing: Sequence[HFile] = ()
) -> tuple[list[str], list[HTest]]:
    """Problems with the files of one writing, and the candidate tests they define.

    `existing` holds the files of the earlier writings as they were written: neither a test id nor, in Go, a top-level
    identifier may repeat one of theirs.
    """
    problems: list[str] = []
    tests: list[HTest] = []
    if not files or len(files) > MAX_FILES:
        problems.append(f"{len(files)} H files, 1 to {MAX_FILES} required")
    for file in files:
        file_problems, file_tests = _file_problems(layout, writing, file)
        problems += file_problems
        tests += file_tests
    if layout.language is Language.GO:
        problems += go_collision_problems(files, existing)
    problems += _pool_problems(writing, tests)
    if len({t.test_id for t in tests}) != len(tests):
        problems.append("H test ids repeat")
    earlier = {test_id for file in existing for test_id in test_ids(layout.language, file.path, file.content)}
    problems += [f"{t.test_id}: repeats the id of a test of an earlier writing" for t in tests if t.test_id in earlier]
    return problems, tests


def python_pruned(path: str, source: str, keep: frozenset[str]) -> str | None:
    """The file without its tests outside `keep` (decorators included), or None when no kept test remains.

    A class left with nothing but pruned tests goes with them, so the file still parses.
    """
    spans = python_test_spans(path, source)
    if not any(test_id in keep for test_id in spans):
        return None
    removed = [span for test_id, span in spans.items() if test_id not in keep]
    removed += _emptied_classes(ast.parse(source).body, removed)
    lines = source.splitlines(keepends=True)
    kept = [line for number, line in enumerate(lines, start=1) if not any(s <= number <= e for s, e in removed)]
    return "".join(kept)


def _emptied_classes(body: list[ast.stmt], removed: list[tuple[int, int]]) -> list[tuple[int, int]]:
    emptied: list[tuple[int, int]] = []

    def gone(node: ast.stmt, extra: list[tuple[int, int]]) -> bool:
        return any(s <= node.lineno and (node.end_lineno or node.lineno) <= e for s, e in [*removed, *extra])

    for node in body:
        if isinstance(node, ast.ClassDef):
            inner = _emptied_classes(node.body, removed)
            emptied += inner
            if all(gone(child, inner) for child in node.body):
                first = min([node.lineno, *(d.lineno for d in node.decorator_list)])
                emptied.append((first, node.end_lineno or node.lineno))
    return emptied
