"""The spec of a task as ADR 0003 writes it: criteria `<TASK>-n~1` with closed-grammar examples and an Interface.

The spec author returns structured data; this module checks it against the writing rules and renders `SPEC.md`
deterministically, so the agent-visible text never depends on the model's formatting.
"""

from __future__ import annotations

import re
from collections.abc import Mapping
from dataclasses import dataclass
from enum import StrEnum
from typing import Final

from specbench.domain.canonical import JsonValue
from specbench.domain.hunks import ChangedRegions, CodeHunk, hunk_problems, hunks_from_record
from specbench.domain.jsonread import RecordError, enum_of, integer, mapping, sequence, text
from specbench.domain.task import Criterion, CriterionStatus

MIN_CRITERIA: Final = 2
MAX_CRITERIA: Final = 8
MIN_EXAMPLES: Final = 1
MAX_EXAMPLES: Final = 3
REVISION_SUFFIX: Final = "~1"
STATEMENT_PATTERN: Final = re.compile(r"^WHEN .+ THE SYSTEM SHALL .+$", re.DOTALL)
OPERATION_PATTERN: Final = re.compile(r"^[A-Za-z_][A-Za-z0-9_.]*$")
EXAMPLE_PATTERN: Final = re.compile(r"^(?P<call>[A-Za-z_][A-Za-z0-9_.]*\(.*\)) => (?P<outcome>.+)$", re.DOTALL)
RAISES_PATTERN: Final = re.compile(r"^raises [A-Za-z_][A-Za-z0-9_.]*$")
ISO_DATETIME: Final = re.compile(r"^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}(:\d{2}(\.\d+)?)?)?$")
NUMBER: Final = re.compile(r"^-?\d+(\.\d+)?$")
KEYWORD_LITERALS: Final = frozenset({"true", "false", "null"})
SPEC_FILE_NAME: Final = "SPEC.md"
GAP_CRITERIA_COUNT: Final = 2
"""Criteria drawn for the spec-gap arm; each needs a behaviour the change adds or alters and the hunks that implement it."""


class Dimension(StrEnum):
    """The kind of behaviour a criterion specifies (ADR 0004, spec-gap arm)."""

    ERROR = "error"
    BOUNDARY = "boundary"
    PERMISSION = "permission"
    CONCURRENCY = "concurrency"
    COMPATIBILITY = "compatibility"


CHANGING_STATUSES: Final = (CriterionStatus.NEW, CriterionStatus.REVISED)


class SpecError(RecordError):
    """A spec that breaks the writing rules."""


@dataclass(frozen=True)
class InterfaceEntry:
    import_path: str
    signature: str
    note: str


@dataclass(frozen=True)
class SpecCriterion:
    number: int
    statement: str
    examples: tuple[str, ...]
    status: CriterionStatus
    dimension: Dimension
    implementing_hunks: tuple[CodeHunk, ...]

    @property
    def is_gap_eligible(self) -> bool:
        """A behaviour the change adds or alters can be removed from a spec; an unchanged one carries no change to find."""
        return self.status in CHANGING_STATUSES and bool(self.implementing_hunks)


@dataclass(frozen=True)
class SpecDraft:
    title: str
    overview: str
    interface: tuple[InterfaceEntry, ...]
    criteria: tuple[SpecCriterion, ...]


@dataclass(frozen=True)
class ExampleCall:
    """A parsed example: its operation, its argument literals in order, and what it expects."""

    operation: str
    arguments: tuple[str, ...]
    outcome: str


def split_arguments(arguments: str) -> list[str]:
    """Top-level comma split that respects quotes and brackets."""
    parts: list[str] = []
    depth, quote, current = 0, "", ""
    for character in arguments:
        if quote:
            current += character
            if character == quote:
                quote = ""
        elif character in "\"'":
            quote = character
            current += character
        elif character in "([{":
            depth += 1
            current += character
        elif character in ")]}":
            depth -= 1
            current += character
        elif character == "," and depth == 0:
            parts.append(current.strip())
            current = ""
        else:
            current += character
    if current.strip():
        parts.append(current.strip())
    return parts


def is_literal(value: str) -> bool:
    stripped = value.strip()
    quoted = len(stripped) >= 2 and stripped[0] == '"' and stripped[-1] == '"'
    return (
        quoted
        or ISO_DATETIME.match(stripped) is not None
        or NUMBER.match(stripped) is not None
        or stripped in KEYWORD_LITERALS
    )


def parse_example(example: str) -> ExampleCall | None:
    """The example if it is in the closed grammar `operation(name=literal, ...) => literal | raises Name`."""
    match = EXAMPLE_PATTERN.match(example.strip())
    if match is None:
        return None
    call, outcome = match.group("call"), match.group("outcome").strip()
    operation, _, rest = call.partition("(")
    if OPERATION_PATTERN.match(operation) is None:
        return None
    values: list[str] = []
    for argument in split_arguments(rest[:-1]):
        name, separator, value = argument.partition("=")
        if not separator or OPERATION_PATTERN.match(name.strip()) is None or not is_literal(value):
            return None
        values.append(value.strip())
    if not (is_literal(outcome) or RAISES_PATTERN.match(outcome)):
        return None
    return ExampleCall(operation, tuple(values), outcome)


def spec_problems(draft: SpecDraft, regions: ChangedRegions) -> list[str]:
    """Every writing rule the draft breaks, in a stable order; empty when it may be rendered.

    `regions` are the changed lines of the non-test merge diff that the cited implementing hunks must lie inside.
    """
    problems: list[str] = []
    if not draft.title.strip() or not draft.overview.strip():
        problems.append("title and overview must not be empty")
    if not draft.interface:
        problems.append("the Interface section must name at least one public signature")
    for entry in draft.interface:
        if not (entry.import_path.strip() and entry.signature.strip()):
            problems.append("every Interface entry needs an import path and a signature")
    if not MIN_CRITERIA <= len(draft.criteria) <= MAX_CRITERIA:
        problems.append(f"{len(draft.criteria)} criteria, {MIN_CRITERIA} to {MAX_CRITERIA} required")
    if [c.number for c in draft.criteria] != list(range(1, len(draft.criteria) + 1)):
        problems.append("criterion numbers must run 1, 2, 3, ... in order")
    for criterion in draft.criteria:
        problems += _criterion_problems(criterion, regions)
    if sum(c.is_gap_eligible for c in draft.criteria) < GAP_CRITERIA_COUNT:
        problems.append(f"at least {GAP_CRITERIA_COUNT} criteria must be new or revised and cite implementing hunks")
    return problems


def _criterion_problems(criterion: SpecCriterion, regions: ChangedRegions) -> list[str]:
    label = f"criterion {criterion.number}"
    problems: list[str] = []
    if STATEMENT_PATTERN.match(criterion.statement.strip()) is None or "\n" in criterion.statement.strip():
        problems.append(f"{label}: the statement must be one line `WHEN ... THE SYSTEM SHALL ...`")
    if not MIN_EXAMPLES <= len(criterion.examples) <= MAX_EXAMPLES:
        problems.append(f"{label}: {len(criterion.examples)} examples, {MIN_EXAMPLES} to {MAX_EXAMPLES} required")
    for example in criterion.examples:
        if parse_example(example) is None:
            problems.append(f"{label}: example {example!r} is outside the grammar operation(name=literal) => outcome")
    if criterion.status in CHANGING_STATUSES and not criterion.implementing_hunks:
        problems.append(f"{label}: a new or revised criterion must cite at least one implementing hunk")
    return problems + hunk_problems(label, criterion.implementing_hunks, regions)


def criterion_id(task_id: str, number: int) -> str:
    return f"{task_id}-{number}"


def criteria_of(task_id: str, draft: SpecDraft) -> tuple[Criterion, ...]:
    return tuple(Criterion(criterion_id(task_id, c.number), c.status) for c in draft.criteria)


def render_spec(task_id: str, draft: SpecDraft) -> str:
    """The text of `SPEC.md`: overview, Interface, then one bullet per criterion with its examples."""
    lines = [f"# {draft.title.strip()}", "", draft.overview.strip(), "", "## Interface", ""]
    for entry in draft.interface:
        note = f": {entry.note.strip()}" if entry.note.strip() else ""
        lines.append(f"- `{entry.import_path.strip()}` `{entry.signature.strip()}`{note}")
    lines += ["", "## Requirements", ""]
    for criterion in draft.criteria:
        lines.append(f"- {criterion_id(task_id, criterion.number)}{REVISION_SUFFIX}: {criterion.statement.strip()}")
        lines += [f"  - Example: {example.strip()}" for example in criterion.examples]
    return "\n".join(lines) + "\n"


def spec_from_record(record: JsonValue) -> SpecDraft:
    """A draft from the spec author's structured output."""
    body = mapping(record, "spec")
    return SpecDraft(
        title=text(body.get("title"), "title"),
        overview=text(body.get("overview"), "overview"),
        interface=tuple(
            InterfaceEntry(
                text(mapping(e, "interface entry").get("import_path"), "import_path"),
                text(mapping(e, "interface entry").get("signature"), "signature"),
                text(mapping(e, "interface entry").get("note"), "note"),
            )
            for e in sequence(body.get("interface"), "interface")
        ),
        criteria=tuple(_criterion_from_record(c) for c in sequence(body.get("criteria"), "criteria")),
    )


def _criterion_from_record(record: object) -> SpecCriterion:
    body: Mapping[str, object] = mapping(record, "criterion")
    return SpecCriterion(
        number=integer(body.get("number"), "number", 1),
        statement=text(body.get("statement"), "statement"),
        examples=tuple(text(e, "example") for e in sequence(body.get("examples"), "examples")),
        status=enum_of(CriterionStatus, body.get("status"), "status"),
        dimension=enum_of(Dimension, body.get("dimension"), "dimension"),
        implementing_hunks=hunks_from_record(body.get("implementingHunks")),
    )


def spec_to_record(draft: SpecDraft) -> dict[str, JsonValue]:
    return {
        "criteria": [
            {
                "dimension": c.dimension.value,
                "examples": list(c.examples),
                "implementingHunks": [h.to_record() for h in c.implementing_hunks],
                "number": c.number,
                "statement": c.statement,
                "status": c.status.value,
            }
            for c in draft.criteria
        ],
        "interface": [
            {"import_path": e.import_path, "note": e.note, "signature": e.signature} for e in draft.interface
        ],
        "overview": draft.overview,
        "title": draft.title,
    }
