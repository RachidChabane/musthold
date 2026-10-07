"""Leak controls of ADR 0004: literal tuples of the human oracle must not reach the spec, and the spec must not make
boundary inputs predictable beyond what the issue already did.

One extractor serves both controls: an input tuple is the set of normalised literal values of one call (or one
table row) in a test. The literal check compares them with the spec's examples; the probe scoring compares them
with the tuples a model predicts.
"""

from __future__ import annotations

import ast
import re
from collections.abc import Iterable, Mapping, Sequence
from dataclasses import dataclass
from decimal import Decimal, InvalidOperation
from typing import Final, cast

from specbench.domain.languages import Language
from specbench.domain.spec import ISO_DATETIME, NUMBER, SpecDraft, parse_example, split_arguments

MAX_EXCESS_PROBE_HITS: Final = 2
"""Predicted oracle tuples the spec may add over the issue alone before the task is fixed or dropped."""
TRIVIAL_VALUES: Final = frozenset({"n:0", "n:1", "n:-1", "b:true", "b:false", "null", "s:"})
GO_CALL: Final = re.compile(r"\b[A-Za-z_][A-Za-z0-9_.]*\(([^()]*)\)")
GO_ROW: Final = re.compile(r"\{([^{}]*)\}")
GO_FIELD_KEY: Final = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*\s*:\s*")
KEYWORD_VALUES: Final = {"true": "b:true", "false": "b:false", "null": "null", "nil": "null", "none": "null"}

LiteralTuple = frozenset[str]


def normalize_literal(raw: str) -> str | None:
    """The canonical form of one literal as a spec example, a Python test or a Go test writes it; None if it is not one."""
    value = raw.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'`":
        return f"s:{value[1:-1]}"
    lowered = value.lower()
    if lowered in KEYWORD_VALUES:
        return KEYWORD_VALUES[lowered]
    if ISO_DATETIME.match(value):
        return f"d:{value}"
    if NUMBER.match(value):
        try:
            return f"n:{format(Decimal(value).normalize(), 'f')}"
        except InvalidOperation:
            return None
    return None


def significant(values: LiteralTuple) -> bool:
    return bool(values) and not values <= TRIVIAL_VALUES


def _tuple_of(parts: Iterable[str]) -> LiteralTuple | None:
    normalised = [normalize_literal(part) for part in parts]
    if not normalised or any(value is None for value in normalised):
        return None
    return frozenset(value for value in normalised if value is not None)


def _python_literal(node: ast.expr) -> list[str] | None:
    try:
        value = ast.literal_eval(node)
    except (ValueError, TypeError, SyntaxError):
        return None
    return _flatten(value)


def _flatten(value: object) -> list[str] | None:
    if isinstance(value, (list, tuple, set, frozenset)):
        flat: list[str] = []
        for item in cast(Iterable[object], value):
            inner = _flatten(item)
            if inner is None:
                return None
            flat += inner
        return flat
    return _scalar(value)


def _scalar(value: object) -> list[str] | None:
    if isinstance(value, bool):
        return ["true" if value else "false"]
    if value is None:
        return ["null"]
    if isinstance(value, (int, float, Decimal)):
        return [str(value)]
    return [f'"{value}"'] if isinstance(value, str) else None


def python_tuples(source: str) -> set[LiteralTuple]:
    """Literal-only calls and parametrize rows of Python test source."""
    found: set[LiteralTuple] = set()
    try:
        tree = ast.parse(source)
    except SyntaxError:
        return found
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue
        if _is_parametrize(node) and len(node.args) >= 2:
            found |= _parametrize_rows(node.args[1])
            continue
        parts: list[str] = []
        for argument in [*node.args, *(k.value for k in node.keywords)]:
            literal = _python_literal(argument)
            if literal is None:
                parts = []
                break
            parts += literal
        values = _tuple_of(parts)
        if values is not None:
            found.add(values)
    return found


def _is_parametrize(node: ast.Call) -> bool:
    return isinstance(node.func, ast.Attribute) and node.func.attr == "parametrize"


def _parametrize_rows(node: ast.expr) -> set[LiteralTuple]:
    rows: set[LiteralTuple] = set()
    if not isinstance(node, (ast.List, ast.Tuple)):
        return rows
    for element in node.elts:
        literal = _python_literal(element)
        values = _tuple_of(literal) if literal is not None else None
        if values is not None:
            rows.add(values)
    return rows


def go_tuples(source: str) -> set[LiteralTuple]:
    """Literal-only calls and literal-only table rows of Go test source."""
    found: set[LiteralTuple] = set()
    for match in [*GO_CALL.finditer(source), *GO_ROW.finditer(source)]:
        parts = [GO_FIELD_KEY.sub("", part) for part in split_arguments(match.group(1))]
        values = _tuple_of(parts)
        if values is not None:
            found.add(values)
    return found


def oracle_tuples(language: Language, test_sources: Mapping[str, str]) -> set[LiteralTuple]:
    """Significant literal tuples of the human tests, given each test's source."""
    extract = python_tuples if language is Language.PYTHON else go_tuples
    found: set[LiteralTuple] = set()
    for source in test_sources.values():
        found |= extract(source)
    return {values for values in found if significant(values)}


def spec_tuples(draft: SpecDraft) -> set[LiteralTuple]:
    """Literal tuples of the spec's examples."""
    found: set[LiteralTuple] = set()
    for criterion in draft.criteria:
        for example in criterion.examples:
            parsed = parse_example(example)
            values = _tuple_of(parsed.arguments) if parsed is not None else None
            if values is not None:
                found.add(values)
    return {values for values in found if significant(values)}


def _contained(value: str, text: str) -> bool:
    kind, _, body = value.partition(":")
    if kind in ("b",) or value == "null":
        return True
    if kind == "n":
        return re.search(rf"(?<![\w.]){re.escape(body)}(?![\w])", text) is not None
    return body in text


def in_sources(values: LiteralTuple, texts: Sequence[str]) -> bool:
    """Whether every literal of the tuple occurs in the issue, the pull request description or the upstream docs."""
    return any(all(_contained(value, text) for value in values) for text in texts)


def literal_leaks(oracle: set[LiteralTuple], draft: SpecDraft, texts: Sequence[str]) -> list[LiteralTuple]:
    """Oracle tuples that a spec example spells out and that no public source states, in a stable order."""
    return sorted(
        (values for values in oracle & spec_tuples(draft) if not in_sources(values, texts)),
        key=sorted,
    )


def predicted_tuples(predictions: Iterable[Sequence[str]]) -> set[LiteralTuple]:
    """Normalised tuples of a probe's predicted argument lists; a prediction that is not all literals is dropped."""
    found = (_tuple_of(arguments) for arguments in predictions)
    return {values for values in found if values is not None and significant(values)}


def probe_hits(oracle: set[LiteralTuple], predictions: Iterable[Sequence[str]]) -> int:
    return len(oracle & predicted_tuples(predictions))


@dataclass(frozen=True)
class ProbeOutcome:
    hits_issue_only: int
    hits_with_spec: int

    @property
    def excess(self) -> int:
        return self.hits_with_spec - self.hits_issue_only

    @property
    def leaks(self) -> bool:
        return self.excess > MAX_EXCESS_PROBE_HITS
