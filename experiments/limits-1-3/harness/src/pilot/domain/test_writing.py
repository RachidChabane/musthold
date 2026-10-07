"""Test-writer output: the static checks of a written test and the suite module the harness builds from it (PREREG 6.8).

A writer returns `{tests: [{name, source}]}`. The harness decorates each accepted test
with `@pytest.mark.verifies(<ref>)` and writes one module per criterion.
"""

from __future__ import annotations

import ast
import re
import sys
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from enum import Enum
from typing import Any, Final

from pilot.domain.alignment_input import SIGNATURE_SEPARATOR, signature_of
from pilot.domain.constants import (
    PACKAGE_NAME,
    PUBLIC_FUNCTIONS,
    TESTS_DIRNAME,
    TEST_FUNCTION_PREFIX,
    VERIFIES_MARKER_NAME,
)

ALLOWED_THIRD_PARTY_MODULES: Final = ("pytest", PACKAGE_NAME)
PUBLIC_API_NAMES: Final = ("Breakdown", *PUBLIC_FUNCTIONS)
MIN_TESTS: Final = 2
MAX_TESTS: Final = 5
_TEST_NAME: Final = re.compile(rf"^{TEST_FUNCTION_PREFIX}[A-Za-z0-9_]+$")
_REF_SLUG: Final = re.compile(r"^PARK-(\d+)~\d+$")
SUITE_FILE_TEMPLATE: Final = f"{TESTS_DIRNAME}/test_park_{{number}}__{{name}}.py"
_SUITE_FILE: Final = re.compile(rf"^{TESTS_DIRNAME}/test_park_(?P<number>\d+)__")
NODEID_SEPARATOR: Final = "::"
PYTEST_IMPORT: Final = "pytest"


class TestStatus(Enum):
    __test__ = False

    KEPT = "kept"
    REJECTED_STATIC = "rejected_static"
    DROPPED_REFERENCE = "dropped_reference"


@dataclass(frozen=True)
class WrittenTest:
    name: str
    source: str


def written_tests(payload: Mapping[str, Any]) -> list[WrittenTest]:
    return [WrittenTest(entry["name"], entry["source"]) for entry in payload["tests"]]


def writer_payload_problems(payload: Any) -> list[str]:
    """Constraints the schema cannot express: test names are identifiers with the test prefix, each once."""
    names = [entry["name"] for entry in payload["tests"]]
    problems = [f"name {name!r} is not a test function name" for name in names if not _TEST_NAME.match(name)]
    duplicated = sorted({name for name in names if names.count(name) > 1})
    return problems + [f"name {name!r} appears more than once" for name in duplicated]


def static_problems(test: WrittenTest, stdlib: frozenset[str] = frozenset(sys.stdlib_module_names)) -> list[str]:
    """Empty when the source is one function named `name` plus allowed imports; the AST import allowlist."""
    try:
        tree = ast.parse(test.source)
    except SyntaxError as error:
        return [f"unparseable: {error.msg}"]
    problems = _structure_problems(tree, test.name)
    for node in ast.walk(tree):
        problems += _import_problems(node, stdlib)
    return problems


def _structure_problems(tree: ast.Module, name: str) -> list[str]:
    functions = [node for node in tree.body if isinstance(node, ast.FunctionDef)]
    others = [node for node in tree.body if not isinstance(node, ast.FunctionDef | ast.Import | ast.ImportFrom)]
    problems = [f"top-level {type(node).__name__} is not allowed" for node in others]
    if [f.name for f in functions] != [name]:
        problems.append(f"expected exactly one function named {name!r}")
    return problems


def _import_problems(node: ast.AST, stdlib: frozenset[str]) -> list[str]:
    if isinstance(node, ast.Import):
        modules = [alias.name for alias in node.names]
    elif isinstance(node, ast.ImportFrom):
        if node.level:
            return ["relative import"]
        modules = [node.module or ""]
    else:
        return []
    allowed = stdlib | set(ALLOWED_THIRD_PARTY_MODULES)
    return [f"import of {module!r} is not allowed" for module in modules if module.split(".")[0] not in allowed]


def suite_path(ref: str, name: str) -> str:
    """One module per test: an import that fails to collect errors that test alone, never its siblings."""
    match = _REF_SLUG.match(ref)
    if match is None:
        raise ValueError(f"not a criterion reference: {ref!r}")
    return SUITE_FILE_TEMPLATE.format(number=match.group(1), name=name)


def suite_nodeid(ref: str, name: str) -> str:
    return f"{suite_path(ref, name)}{NODEID_SEPARATOR}{name}"


def criterion_of_suite_nodeid(nodeid: str, refs: Sequence[str]) -> str:
    match = _SUITE_FILE.match(nodeid.split(NODEID_SEPARATOR, 1)[0])
    if match is not None:
        for ref in refs:
            if ref.startswith(f"PARK-{match.group('number')}~"):
                return ref
    raise ValueError(f"no criterion owns {nodeid!r}")


def module_of_test(ref: str, test: WrittenTest) -> str:
    """The module of one test: its imports, then the function marked as verifying `ref`."""
    tree = ast.parse(test.source)
    imports: dict[str, ast.stmt] = {PYTEST_IMPORT: ast.Import(names=[ast.alias(name=PYTEST_IMPORT)])}
    functions: list[ast.FunctionDef] = []
    for node in tree.body:
        if isinstance(node, ast.Import | ast.ImportFrom):
            imports.setdefault(ast.unparse(node), node)
        elif isinstance(node, ast.FunctionDef):
            node.decorator_list.insert(0, _verifies_decorator(ref))
            functions.append(node)
    module = ast.Module(body=[*imports.values(), *functions], type_ignores=[])
    return ast.unparse(ast.fix_missing_locations(module)) + "\n"


def _verifies_decorator(ref: str) -> ast.expr:
    marker = ast.Attribute(
        value=ast.Attribute(value=ast.Name(id=PYTEST_IMPORT, ctx=ast.Load()), attr="mark", ctx=ast.Load()),
        attr=VERIFIES_MARKER_NAME,
        ctx=ast.Load(),
    )
    return ast.Call(func=marker, args=[ast.Constant(value=ref)], keywords=[])


def public_api(package_sources: Mapping[str, str]) -> str:
    """Signatures, without bodies, of the package's public names."""
    stubs = [stub for name in PUBLIC_API_NAMES if (stub := signature_of(name, package_sources)) is not None]
    return SIGNATURE_SEPARATOR.join(stubs)
