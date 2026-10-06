"""Alignment-judge input built deterministically from the fixture alone (PREREG 7.1).

The test unit is the test function plus the module-level helpers it reaches (resolved
with `ast`), with imports kept for the names used, `verifies` decorators, comments and
docstrings removed, and the result re-emitted with `ast.unparse` and re-parsed.
"""

from __future__ import annotations

import ast
import json
import re
from collections.abc import Iterable, Mapping, Sequence
from dataclasses import dataclass
from enum import Enum

from pilot.domain.definitions import is_verifies_decorator
from pilot.domain.ids import sha256_hex
from pilot.domain.normalization import normalize
from pilot.domain.pipeline_constants import (
    HASH_FIELD_SEPARATOR,
    LEAK_FORBIDDEN_STRINGS,
    MASKED_HELPER_PREFIX,
    MASKED_TEST_NAME,
    NAME_RUN_FORBIDDEN_LENGTH,
    NONE_ANSWER,
    TOKEN_HEX_DIGITS,
    TOKEN_PREFIX,
    TOKEN_REDRAW_LIMIT,
)
from pilot.domain.spec import Criterion

TEST_PREFIX_WORD = "test"
SIGNATURE_BODY = "..."
SIGNATURE_SEPARATOR = "\n\n"
_WORD = re.compile(r"[A-Za-z]+|\d+")
_DefNode = ast.FunctionDef | ast.AsyncFunctionDef | ast.ClassDef
_Importer = ast.Import | ast.ImportFrom


class Arm(Enum):
    NAME_MASKED = "NAME-MASKED"
    NAME_SHOWN = "NAME-SHOWN"


class UnknownTest(Exception):
    pass


@dataclass(frozen=True)
class TestUnit:
    __test__ = False

    source: str
    imported_symbols: tuple[tuple[str, str], ...]


def build_test_unit(module_source: str, test_name: str, arm: Arm) -> TestUnit:
    tree = ast.parse(module_source)
    definitions = _module_definitions(tree)
    if test_name not in definitions or not isinstance(definitions[test_name][0], ast.FunctionDef):
        raise UnknownTest(test_name)
    selected = _reached(definitions, test_name)
    body = [_without_verifies(definitions[name][0]) for name in selected]
    used = {n.id for stmt in body for n in ast.walk(stmt) if isinstance(n, ast.Name)}
    imports = _used_imports(tree, used)
    nodes = [*imports, *body]
    module = ast.Module(body=nodes, type_ignores=[])
    if arm is Arm.NAME_MASKED:
        module = _mask(module, test_name, selected)
    ast.fix_missing_locations(module)
    text = normalize(ast.unparse(module))
    ast.parse(text)
    return TestUnit(text, _imported_symbols(imports))


def _module_definitions(tree: ast.Module) -> dict[str, tuple[ast.stmt, int]]:
    found: dict[str, tuple[ast.stmt, int]] = {}
    for position, node in enumerate(tree.body):
        if isinstance(node, _DefNode):
            found[node.name] = (node, position)
        elif isinstance(node, ast.Assign) and len(node.targets) == 1 and isinstance(node.targets[0], ast.Name):
            found[node.targets[0].id] = (node, position)
        elif isinstance(node, ast.AnnAssign) and isinstance(node.target, ast.Name):
            found[node.target.id] = (node, position)
    return found


def _reached(definitions: Mapping[str, tuple[ast.stmt, int]], test_name: str) -> list[str]:
    """Names of the test and its transitive module-level helpers, helpers first in source order."""
    seen = {test_name}
    pending = [test_name]
    while pending:
        node = definitions[pending.pop()][0]
        for child in ast.walk(node):
            if isinstance(child, ast.Name) and child.id in definitions and child.id not in seen:
                seen.add(child.id)
                pending.append(child.id)
    helpers = sorted(seen - {test_name}, key=lambda name: definitions[name][1])
    return [*helpers, test_name]


def _used_imports(tree: ast.Module, used: set[str]) -> list[_Importer]:
    kept: list[_Importer] = []
    for node in tree.body:
        if isinstance(node, ast.Import):
            aliases = [a for a in node.names if (a.asname or a.name.split(".")[0]) in used]
            if aliases:
                kept.append(ast.Import(names=aliases))
        elif isinstance(node, ast.ImportFrom):
            aliases = [a for a in node.names if (a.asname or a.name) in used]
            if aliases:
                kept.append(ast.ImportFrom(module=node.module, names=aliases, level=node.level))
    return kept


def _imported_symbols(imports: Iterable[_Importer]) -> tuple[tuple[str, str], ...]:
    """(module, name) of every `from module import name` kept in the unit."""
    return tuple(
        (node.module or "", alias.name)
        for node in imports
        if isinstance(node, ast.ImportFrom)
        for alias in node.names
    )


def _without_verifies(node: ast.stmt) -> ast.stmt:
    if isinstance(node, _DefNode):
        node.decorator_list = [d for d in node.decorator_list if not is_verifies_decorator(d)]
    return node


class _Renamer(ast.NodeTransformer):
    def __init__(self, mapping: Mapping[str, str]) -> None:
        self._mapping = mapping

    def visit_Name(self, node: ast.Name) -> ast.AST:
        node.id = self._mapping.get(node.id, node.id)
        return node

    def visit_FunctionDef(self, node: ast.FunctionDef) -> ast.AST:
        node.name = self._mapping.get(node.name, node.name)
        self.generic_visit(node)
        return node

    def visit_ClassDef(self, node: ast.ClassDef) -> ast.AST:
        node.name = self._mapping.get(node.name, node.name)
        self.generic_visit(node)
        return node


def _mask(module: ast.Module, test_name: str, selected: Sequence[str]) -> ast.Module:
    mapping = {name: f"{MASKED_HELPER_PREFIX}{n}" for n, name in enumerate(selected[:-1], start=1)}
    mapping[test_name] = MASKED_TEST_NAME
    renamed = _Renamer(mapping).visit(module)
    assert isinstance(renamed, ast.Module)
    return renamed


def public_signatures(unit: TestUnit, package: str, package_sources: Mapping[str, str]) -> str:
    """Signatures, without bodies, of the package names the test unit imports from `package`."""
    wanted = [
        name
        for module, name in unit.imported_symbols
        if module == package or module.startswith(f"{package}.")
    ]
    stubs = [stub for name in wanted if (stub := _signature_of(name, package_sources)) is not None]
    return SIGNATURE_SEPARATOR.join(stubs)


def _signature_of(name: str, package_sources: Mapping[str, str]) -> str | None:
    for _file, source in sorted(package_sources.items()):
        for node in ast.parse(source).body:
            if isinstance(node, _DefNode) and node.name == name:
                return ast.unparse(_stub(node))
    return None


def _stub(node: _DefNode) -> _DefNode:
    if isinstance(node, ast.ClassDef):
        members: list[ast.stmt] = [
            _stub(m) if isinstance(m, ast.FunctionDef | ast.AsyncFunctionDef) else m
            for m in node.body
            if isinstance(m, ast.AnnAssign | ast.FunctionDef | ast.AsyncFunctionDef)
        ]
        node.body = members or [_ellipsis()]
        return node
    node.body = [_ellipsis()]
    return node


def _ellipsis() -> ast.stmt:
    return ast.Expr(value=ast.Constant(value=Ellipsis))


@dataclass(frozen=True)
class TokenizedCriteria:
    """Criteria as shown to a judge: statements under opaque tokens, in a permuted order."""

    text: str
    token_to_ref: Mapping[str, str]
    tokens: tuple[str, ...]


def tokenize_criteria(
    criteria: Sequence[Criterion], run_seed: str, test_uid: str, arm: Arm, rep: int
) -> TokenizedCriteria:
    token_of = _draw_tokens(criteria, run_seed, test_uid, arm, rep)
    order = sorted(criteria, key=lambda c: _field_hash(test_uid, arm.value, str(rep), c.id))
    lines = [f"{token_of[c.id]}: {c.statement}" for c in order]
    return TokenizedCriteria(
        text="\n".join(lines),
        token_to_ref={token_of[c.id]: c.ref for c in criteria},
        tokens=tuple(token_of[c.id] for c in order),
    )


def _field_hash(*fields: str) -> str:
    return sha256_hex(HASH_FIELD_SEPARATOR.join(fields))


def _draw_tokens(
    criteria: Sequence[Criterion], run_seed: str, test_uid: str, arm: Arm, rep: int
) -> dict[str, str]:
    """First four hex digits of the hash, redrawn until every token is unique."""
    taken: set[str] = set()
    drawn: dict[str, str] = {}
    for criterion in criteria:
        for attempt in range(TOKEN_REDRAW_LIMIT):
            fields = [run_seed, test_uid, arm.value, str(rep), criterion.id]
            if attempt:
                fields.append(str(attempt))
            token = TOKEN_PREFIX + _field_hash(*fields)[:TOKEN_HEX_DIGITS]
            if token not in taken:
                break
        else:
            raise RuntimeError("cannot draw a unique token")
        taken.add(token)
        drawn[criterion.id] = token
    return drawn


def alignment_schema(template_text: str, tokens: Sequence[str]) -> str:
    """The alignment schema with `__TOKENS__` replaced by the call's tokens."""
    schema = json.loads(template_text)
    options = schema["properties"]["verifies"]["enum"]
    expanded = [value for option in options for value in (list(tokens) if option == "__TOKENS__" else [option])]
    schema["properties"]["verifies"]["enum"] = expanded
    return json.dumps(schema, sort_keys=True)


def _words(text: str) -> list[str]:
    return [word.lower() for word in _WORD.findall(text)]


def leaked_name_runs(item: str, test_name: str, run_length: int = NAME_RUN_FORBIDDEN_LENGTH) -> list[str]:
    """Runs of `run_length` or more consecutive words of the original test name found in `item`."""
    name_words = [w for w in _words(test_name) if w != TEST_PREFIX_WORD]
    item_words = _words(item)
    found = []
    for start in range(len(name_words) - run_length + 1):
        run = name_words[start : start + run_length]
        if any(item_words[i : i + run_length] == run for i in range(len(item_words) - run_length + 1)):
            found.append(" ".join(run))
    return found


def forbidden_strings_in(item: str) -> list[str]:
    return [s for s in LEAK_FORBIDDEN_STRINGS if s in item]


__all__ = [
    "NONE_ANSWER",
    "Arm",
    "TestUnit",
    "TokenizedCriteria",
    "UnknownTest",
    "alignment_schema",
    "build_test_unit",
    "forbidden_strings_in",
    "leaked_name_runs",
    "public_signatures",
    "tokenize_criteria",
]
