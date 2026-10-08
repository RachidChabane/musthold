"""Implementing hunks: the new-side line ranges of the non-test merge diff that a criterion cites, and their validator.

A cited hunk is `(path, startLine, endLine)` in the files as they stand at the merge commit. It is valid when its path
is a non-test file of the diff and every line of its range belongs to the new side of the diff hunks of that file.
The diff is read deterministically; nothing here is a model call.
"""

from __future__ import annotations

import re
from collections.abc import Iterable, Mapping, Sequence
from dataclasses import dataclass
from typing import Final

from specbench.domain.canonical import JsonValue
from specbench.domain.jsonread import integer, mapping, sequence, text
from specbench.domain.languages import Language, is_test_path

HUNK_HEADER: Final = re.compile(r"^@@ -\d+(?:,(?P<old>\d+))? \+(?P<start>\d+)(?:,(?P<new>\d+))? @@")
NEW_FILE_HEADER: Final = "+++ "
NEW_FILE_PREFIX: Final = "b/"
NO_NEW_FILE: Final = "/dev/null"
DIFF_START: Final = "diff --git "
NO_NEWLINE_MARKER: Final = "\\"
PATH_TERMINATOR: Final = "\t"
MAX_HUNKS_PER_CRITERION: Final = 8
NON_EMPTY: Final = re.compile(r".+")


@dataclass(frozen=True, order=True)
class CodeHunk:
    """A line range of one file at the merge commit; both ends are inclusive and 1-based."""

    path: str
    start_line: int
    end_line: int

    def to_record(self) -> dict[str, JsonValue]:
        return {"endLine": self.end_line, "path": self.path, "startLine": self.start_line}

    @property
    def label(self) -> str:
        return f"{self.path}:{self.start_line}-{self.end_line}"


def hunk_from_record(record: object) -> CodeHunk:
    body = mapping(record, "implementing hunk")
    return CodeHunk(
        text(body.get("path"), "path", NON_EMPTY),
        integer(body.get("startLine"), "startLine", 1),
        integer(body.get("endLine"), "endLine", 1),
    )


def hunks_from_record(record: object) -> tuple[CodeHunk, ...]:
    return tuple(hunk_from_record(h) for h in sequence(record, "implementingHunks"))


@dataclass(frozen=True)
class ChangedRegions:
    """The new-side line intervals of the non-test files of a diff, merged where they touch, per path."""

    intervals: Mapping[str, tuple[tuple[int, int], ...]]

    def contains(self, hunk: CodeHunk) -> bool:
        return any(
            start <= hunk.start_line and hunk.end_line <= end for start, end in self.intervals.get(hunk.path, ())
        )

    @property
    def paths(self) -> frozenset[str]:
        return frozenset(self.intervals)


def _new_path(header: str) -> str | None:
    """The path a `+++` line names; None for a deleted file and for a path git had to quote."""
    target = header.removeprefix(NEW_FILE_HEADER).split(PATH_TERMINATOR, 1)[0]
    if target == NO_NEW_FILE or not target.startswith(NEW_FILE_PREFIX):
        return None
    return target.removeprefix(NEW_FILE_PREFIX)


def _merged(intervals: Iterable[tuple[int, int]]) -> tuple[tuple[int, int], ...]:
    merged: list[tuple[int, int]] = []
    for start, end in sorted(intervals):
        if merged and start <= merged[-1][1] + 1:
            merged[-1] = (merged[-1][0], max(merged[-1][1], end))
        else:
            merged.append((start, end))
    return tuple(merged)


def new_side_intervals(unified_diff: str) -> dict[str, list[tuple[int, int]]]:
    """Per file, the new-side line interval of every diff hunk, context lines included.

    A hunk body is consumed by the counts of its header, so a line of the body that looks like a file header or a
    hunk header is read as content. A deleted file and a hunk that keeps no line on the new side have no interval.
    """
    found: dict[str, list[tuple[int, int]]] = {}
    path: str | None = None
    old_left = new_left = 0
    next_line = 0
    for line in unified_diff.splitlines():
        if old_left > 0 or new_left > 0:
            if line.startswith(NO_NEWLINE_MARKER):
                continue
            if line.startswith("-"):
                old_left -= 1
            elif line.startswith("+"):
                new_left -= 1
            else:
                old_left -= 1
                new_left -= 1
            continue
        if line.startswith(DIFF_START):
            path = None
        elif line.startswith(NEW_FILE_HEADER):
            path = _new_path(line)
        elif (header := HUNK_HEADER.match(line)) is not None:
            old_left = int(header.group("old") if header.group("old") is not None else 1)
            new_left = int(header.group("new") if header.group("new") is not None else 1)
            next_line = int(header.group("start"))
            if path is not None and new_left > 0:
                found.setdefault(path, []).append((next_line, next_line + new_left - 1))
    return found


def changed_regions(language: Language, unified_diff: str) -> ChangedRegions:
    """The regions of the non-test files of a diff."""
    return ChangedRegions(
        {
            path: _merged(intervals)
            for path, intervals in sorted(new_side_intervals(unified_diff).items())
            if not is_test_path(language, path)
        }
    )


def hunk_problems(label: str, hunks: Sequence[CodeHunk], regions: ChangedRegions) -> list[str]:
    """Every reason the cited hunks do not lie inside the non-test diff; empty when all of them do."""
    problems: list[str] = []
    if len(hunks) > MAX_HUNKS_PER_CRITERION:
        problems.append(f"{label}: {len(hunks)} implementing hunks, at most {MAX_HUNKS_PER_CRITERION} allowed")
    for hunk in hunks:
        if hunk.start_line < 1 or hunk.end_line < hunk.start_line:
            problems.append(f"{label}: hunk {hunk.label} is not a line range")
        elif hunk.path not in regions.paths:
            problems.append(f"{label}: hunk {hunk.label} names a file that is not a non-test file of the diff")
        elif not regions.contains(hunk):
            problems.append(f"{label}: hunk {hunk.label} is not inside the changed lines of its file")
    return problems
