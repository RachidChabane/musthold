"""Which tests a pull request touched: test functions whose lines the diff adds or edits, as test ids.

A Python test id is `<file>::<Class>::<name>` or `<file>::<name>`; a Go test id is `<package dir>::<Name>`.
Parametrized ids (`name[...]`) are matched on the name before the bracket.
"""

from __future__ import annotations

import ast
import posixpath
import re
from collections.abc import Mapping
from typing import Final

from specbench.domain.languages import Language

HUNK_HEADER: Final = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@")
FILE_HEADER: Final = re.compile(r"^\+\+\+ b/(.+)$")
GO_TEST_FUNCTION: Final = re.compile(r"^func (Test[A-Za-z0-9_]*)\(")
PYTHON_TEST_PREFIX: Final = "test"
PARAMETER_SUFFIX: Final = re.compile(r"\[.*\]$")
NODE_SEPARATOR: Final = "::"


def added_lines(unified_diff: str) -> dict[str, frozenset[int]]:
    """Line numbers, in the new file, that a `git diff -U0` adds or rewrites, per path."""
    found: dict[str, set[int]] = {}
    path: str | None = None
    for line in unified_diff.splitlines():
        header = FILE_HEADER.match(line)
        if header is not None:
            path = str(header.group(1))
            found.setdefault(path, set())
            continue
        hunk = HUNK_HEADER.match(line)
        if hunk is not None and path is not None:
            start, count = int(hunk.group(1)), int(hunk.group(2) if hunk.group(2) is not None else 1)
            found[path].update(range(start, start + count))
    return {name: frozenset(lines) for name, lines in found.items()}


def python_test_spans(path: str, source: str) -> dict[str, tuple[int, int]]:
    spans: dict[str, tuple[int, int]] = {}
    try:
        module = ast.parse(source)
    except SyntaxError:
        return spans

    def visit(body: list[ast.stmt], prefix: tuple[str, ...]) -> None:
        for node in body:
            if isinstance(node, ast.ClassDef):
                visit(node.body, (*prefix, node.name))
            elif isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and node.name.startswith(PYTHON_TEST_PREFIX):
                first = min([node.lineno, *(d.lineno for d in node.decorator_list)])
                spans[NODE_SEPARATOR.join([path, *prefix, node.name])] = (first, node.end_lineno or node.lineno)

    visit(module.body, ())
    return spans


def go_test_spans(path: str, source: str) -> dict[str, tuple[int, int]]:
    package = posixpath.dirname(path) or "."
    spans: dict[str, tuple[int, int]] = {}
    lines = source.splitlines()
    for index, line in enumerate(lines, start=1):
        match = GO_TEST_FUNCTION.match(line)
        if match is None:
            continue
        end = next((n for n in range(index, len(lines) + 1) if lines[n - 1].startswith("}")), len(lines))
        spans[f"{package}{NODE_SEPARATOR}{match.group(1)}"] = (index, end)
    return spans


def touched_tests(language: Language, path: str, merged_source: str, added: frozenset[int]) -> frozenset[str]:
    """The tests of one test file, as it stands at merge, that contain a line the pull request added."""
    spans = (
        python_test_spans(path, merged_source) if language is Language.PYTHON else go_test_spans(path, merged_source)
    )
    return frozenset(test_id for test_id, (start, end) in spans.items() if any(start <= line <= end for line in added))


def without_parameters(test_id: str) -> str:
    return PARAMETER_SUFFIX.sub("", test_id)


def touched_by_diff(
    language: Language, sources: Mapping[str, str], diff_by_path: Mapping[str, frozenset[int]]
) -> frozenset[str]:
    """Touched tests across every changed test file; `sources` holds each file's text at merge."""
    found: set[str] = set()
    for path, text in sources.items():
        found |= touched_tests(language, path, text, diff_by_path.get(path, frozenset()))
    return frozenset(found)


def function_sources(language: Language, path: str, merged_source: str, wanted: frozenset[str]) -> dict[str, str]:
    """The source text of each wanted test (parameter-free ids) of one test file, decorators included."""
    spans = (
        python_test_spans(path, merged_source) if language is Language.PYTHON else go_test_spans(path, merged_source)
    )
    lines = merged_source.splitlines()
    return {test_id: "\n".join(lines[start - 1 : end]) for test_id, (start, end) in spans.items() if test_id in wanted}


def test_ids(language: Language, path: str, source: str) -> list[str]:
    """Every test of one file in source order: its id as the oracle runners report it."""
    spans = python_test_spans(path, source) if language is Language.PYTHON else go_test_spans(path, source)
    return sorted(spans, key=lambda test_id: spans[test_id][0])
