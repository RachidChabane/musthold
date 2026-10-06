"""Static in-universe rules for the fixture's test files (PREREG 2.5 and 2.6)."""

from __future__ import annotations

import ast
import io
import re
import tokenize
from collections.abc import Mapping
from dataclasses import dataclass

from pilot.domain.constants import (
    CONFTEST_FILENAME,
    CRITERION_REF_PATTERN,
    FIXTURE_DECORATOR_NAMES,
    MODULE_MARKS_NAME,
    PARAMETRIZE_DECORATOR_NAMES,
    SHALLNOT_CONFTEST_HOOK,
    TEST_FUNCTION_PREFIX,
    USEFIXTURES_DECORATOR_NAMES,
    VERIFIES_MARKER_NAME,
)

_REF_PATTERN = re.compile(CRITERION_REF_PATTERN)
_DefNode = ast.FunctionDef | ast.AsyncFunctionDef


@dataclass(frozen=True)
class LeakFinding:
    file: str
    message: str
    line: int | None = None

    def __str__(self) -> str:
        where = f"{self.file}:{self.line}" if self.line else self.file
        return f"{where}: {self.message}"


def check_test_files(
    sources: Mapping[str, str],
    expected_files: tuple[str, ...],
    expected_test_count: int,
    conftest_source: str | None,
) -> list[LeakFinding]:
    findings = _file_set_findings(sources, expected_files)
    findings += _conftest_findings(conftest_source)
    total = 0
    for name in sorted(sources):
        file_findings, test_count = _check_file(name, sources[name])
        findings += file_findings
        total += test_count
    if total != expected_test_count:
        findings.append(LeakFinding("*", f"{total} tests, expected {expected_test_count}"))
    return findings


def _conftest_findings(conftest_source: str | None) -> list[LeakFinding]:
    if conftest_source is None:
        return [LeakFinding(CONFTEST_FILENAME, "missing")]
    if conftest_source.strip() != SHALLNOT_CONFTEST_HOOK.strip():
        return [LeakFinding(CONFTEST_FILENAME, "is not Shallnot's documented hook verbatim")]
    return []


def _file_set_findings(
    sources: Mapping[str, str], expected_files: tuple[str, ...]
) -> list[LeakFinding]:
    present, expected = set(sources), set(expected_files)
    return [LeakFinding(name, "missing test file") for name in sorted(expected - present)] + [
        LeakFinding(name, "unexpected test file") for name in sorted(present - expected)
    ]


def _check_file(name: str, source: str) -> tuple[list[LeakFinding], int]:
    try:
        tree = ast.parse(source)
    except SyntaxError as error:
        return [LeakFinding(name, f"does not parse: {error.msg}", error.lineno)], 0
    findings = _comment_findings(name, source) + _docstring_findings(name, tree)
    findings += _decorator_findings(name, tree) + _module_mark_findings(name, tree)
    tests = [n for n in tree.body if _is_test_function(n)]
    findings += _marker_findings(name, tree) + _parameter_findings(name, tests)
    if not tests:
        findings.append(LeakFinding(name, "contains no test"))
    return findings, len(tests)


def _comment_findings(name: str, source: str) -> list[LeakFinding]:
    tokens = tokenize.generate_tokens(io.StringIO(source).readline)
    return [
        LeakFinding(name, "comment", token.start[0])
        for token in tokens
        if token.type == tokenize.COMMENT
    ]


def _docstring_findings(name: str, tree: ast.Module) -> list[LeakFinding]:
    findings = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Module | ast.ClassDef | ast.FunctionDef | ast.AsyncFunctionDef):
            if ast.get_docstring(node, clean=False) is not None:
                findings.append(LeakFinding(name, "docstring", getattr(node, "lineno", 1)))
    return findings


def _decorator_findings(name: str, tree: ast.Module) -> list[LeakFinding]:
    forbidden = {*FIXTURE_DECORATOR_NAMES, *PARAMETRIZE_DECORATOR_NAMES, *USEFIXTURES_DECORATOR_NAMES}
    return [
        LeakFinding(name, f"forbidden decorator {_leaf_name(decorator)}", decorator.lineno)
        for node in ast.walk(tree)
        if isinstance(node, _DefNode)
        for decorator in node.decorator_list
        if _leaf_name(decorator) in forbidden
    ]


def _module_mark_findings(name: str, tree: ast.Module) -> list[LeakFinding]:
    return [
        LeakFinding(name, f"module-level {MODULE_MARKS_NAME}", node.lineno)
        for node in tree.body
        if isinstance(node, ast.Assign)
        and any(isinstance(t, ast.Name) and t.id == MODULE_MARKS_NAME for t in node.targets)
    ]


def _parameter_findings(name: str, tests: list[ast.stmt]) -> list[LeakFinding]:
    return [
        LeakFinding(name, f"{node.name} takes parameters", node.lineno)
        for node in tests
        if isinstance(node, _DefNode) and _has_parameters(node)
    ]


def _has_parameters(node: _DefNode) -> bool:
    arguments = node.args
    return bool(
        arguments.posonlyargs or arguments.args or arguments.kwonlyargs or arguments.vararg or arguments.kwarg
    )


def _marker_findings(name: str, tree: ast.Module) -> list[LeakFinding]:
    findings = []
    for node in tree.body:
        if not isinstance(node, _DefNode):
            continue
        markers = [d for d in node.decorator_list if _leaf_name(d) == VERIFIES_MARKER_NAME]
        if _is_test_function(node):
            findings += _test_marker_findings(name, node, markers)
        elif markers:
            findings.append(LeakFinding(name, f"helper {node.name} carries a marker", node.lineno))
    return findings


def _test_marker_findings(
    name: str, node: _DefNode, markers: list[ast.expr]
) -> list[LeakFinding]:
    if len(markers) != 1:
        return [LeakFinding(name, f"{node.name} has {len(markers)} verifies markers", node.lineno)]
    if not _is_valid_marker(markers[0]):
        return [LeakFinding(name, f"{node.name} has a malformed verifies marker", node.lineno)]
    return []


def _is_valid_marker(decorator: ast.expr) -> bool:
    if not isinstance(decorator, ast.Call) or decorator.keywords or len(decorator.args) != 1:
        return False
    argument = decorator.args[0]
    return (
        isinstance(argument, ast.Constant)
        and isinstance(argument.value, str)
        and _REF_PATTERN.match(argument.value) is not None
    )


def _is_test_function(node: ast.stmt) -> bool:
    return isinstance(node, _DefNode) and node.name.startswith(TEST_FUNCTION_PREFIX)


def _leaf_name(decorator: ast.expr) -> str | None:
    target = decorator.func if isinstance(decorator, ast.Call) else decorator
    if isinstance(target, ast.Attribute):
        return target.attr
    if isinstance(target, ast.Name):
        return target.id
    return None
