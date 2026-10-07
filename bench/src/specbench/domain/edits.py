"""Model-written changes to the merge code, as exact search and replace edits that the bench turns into patches.

Violations break one criterion; planted refactors break none. A model-written unified diff often fails to apply, so the
model names exact snippets and the bench produces the patch bytes itself.
"""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Final

from specbench.domain.jsonread import integer, mapping, sequence, text
from specbench.domain.oracle import MIN_PLANTED, MIN_VIOLATIONS_PER_CRITERION

MAX_VIOLATIONS_PER_CRITERION: Final = 3
MAX_PLANTED: Final = 3


@dataclass(frozen=True)
class Edit:
    path: str
    search: str
    replace: str


@dataclass(frozen=True)
class Change:
    """One proposed change: the criterion it violates (None for a refactor), a title and its edits."""

    criterion: int | None
    title: str
    edits: tuple[Edit, ...]


def _edits(record: object) -> tuple[Edit, ...]:
    return tuple(
        Edit(
            text(mapping(e, "edit").get("path"), "path"),
            text(mapping(e, "edit").get("search"), "search"),
            text(mapping(e, "edit").get("replace"), "replace"),
        )
        for e in sequence(record, "edits")
    )


def violations_from_record(record: object) -> tuple[Change, ...]:
    body = mapping(record, "violations")
    return tuple(
        Change(
            integer(mapping(v, "violation").get("criterion"), "criterion", 1),
            text(mapping(v, "violation").get("title"), "title"),
            _edits(mapping(v, "violation").get("edits")),
        )
        for v in sequence(body.get("violations"), "violations")
    )


def refactors_from_record(record: object) -> tuple[Change, ...]:
    body = mapping(record, "refactors")
    return tuple(
        Change(None, text(mapping(r, "refactor").get("title"), "title"), _edits(mapping(r, "refactor").get("edits")))
        for r in sequence(body.get("refactors"), "refactors")
    )


def apply_edits(files: Mapping[str, str], edits: Sequence[Edit], allowed: frozenset[str]) -> dict[str, str] | str:
    """The changed files after the edits, or the reason they cannot apply: unknown path, snippet absent or ambiguous."""
    changed: dict[str, str] = {}
    for edit in edits:
        if edit.path not in allowed or edit.path not in files:
            return f"{edit.path} is not a file of the change"
        current = changed.get(edit.path, files[edit.path])
        if not edit.search or current.count(edit.search) != 1:
            return f"{edit.path}: the search snippet must occur exactly once ({current.count(edit.search)} found)"
        changed[edit.path] = current.replace(edit.search, edit.replace, 1)
    if all(changed[path] == files[path] for path in changed):
        return "the edits change nothing"
    return changed


@dataclass(frozen=True)
class AppliedChange:
    change: Change
    files: Mapping[str, str]


def applicable(
    changes: Sequence[Change], files: Mapping[str, str], allowed: frozenset[str]
) -> tuple[list[AppliedChange], list[str]]:
    """The changes that apply to the merge files, and one problem per change that does not."""
    applied: list[AppliedChange] = []
    problems: list[str] = []
    for change in changes:
        result = apply_edits(files, change.edits, allowed)
        if isinstance(result, str):
            problems.append(f"{change.title}: {result}")
        else:
            applied.append(AppliedChange(change, result))
    return applied, problems


def select_violations(applied: Sequence[AppliedChange], criteria_count: int) -> tuple[list[AppliedChange], list[str]]:
    """At most `MAX_VIOLATIONS_PER_CRITERION` per criterion, and a problem for each criterion left short."""
    chosen: list[AppliedChange] = []
    problems: list[str] = []
    for number in range(1, criteria_count + 1):
        mine = [a for a in applied if a.change.criterion == number][:MAX_VIOLATIONS_PER_CRITERION]
        if len(mine) < MIN_VIOLATIONS_PER_CRITERION:
            problems.append(
                f"criterion {number} has {len(mine)} applicable violations, {MIN_VIOLATIONS_PER_CRITERION} required"
            )
        chosen += mine
    return chosen, problems


def select_planted(applied: Sequence[AppliedChange]) -> tuple[list[AppliedChange], list[str]]:
    chosen = list(applied)[:MAX_PLANTED]
    problems = [] if len(chosen) >= MIN_PLANTED else [f"{len(chosen)} applicable refactors, {MIN_PLANTED} required"]
    return chosen, problems
