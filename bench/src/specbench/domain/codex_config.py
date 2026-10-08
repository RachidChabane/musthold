"""The table Codex persists in its configuration file for a project it was told to trust, and its exact removal.

Pure: it reads and writes no file. The adapter that owns the file applies it after every Codex session.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Final

PROJECTS_TABLE: Final = "projects."
TABLE_OPEN: Final = "["
ARRAY_TABLE_OPEN: Final = "[["
TABLE_CLOSE: Final = "]"
COMMENT_MARK: Final = "#"
LITERAL_QUOTE: Final = "'"
NEWLINE: Final = "\n"
LINE_ENDING_CHARS: Final = "\r\n"
TRUSTED: Final = "trusted"


@dataclass(frozen=True)
class TableRemoval:
    """`table` is the text taken out (header and keys); `cleaned` is the configuration without it, or the input
    itself when it is not there."""

    found: bool
    table: str
    cleaned: str


def project_key(project_dir: str) -> str:
    """The `-c` key that trusts a project: `projects."<dir>".trust_level`."""
    return f"{PROJECTS_TABLE}{_basic_string(project_dir)}.trust_level"


def trust_override(project_dir: str) -> str:
    """The whole `-c` argument that trusts a project for one invocation."""
    return f"{project_key(project_dir)}={_basic_string(TRUSTED)}"


def names_project(content: str, project_dir: str) -> bool:
    """Whether the configuration mentions the project directory anywhere, in a table or in a key."""
    return project_dir in content


def remove_project_table(content: str, project_dir: str) -> TableRemoval:
    """Remove the table `[projects."<dir>"]` and its keys, and nothing else.

    When the table ends the file, the one blank line Codex puts before a table it appends goes with it.
    """
    lines = _split_after_newline(content)
    start = _table_start(lines, project_dir)
    if start < 0:
        return TableRemoval(False, "", content)
    end = _table_end(lines, start)
    kept = [*lines[:start], *lines[end:]]
    if end == len(lines):
        kept = _drop_one_trailing_blank_line(kept)
    return TableRemoval(True, "".join(lines[start:end]), "".join(kept))


def _basic_string(value: str) -> str:
    return json.dumps(value, ensure_ascii=False)


def _split_after_newline(content: str) -> list[str]:
    """Split after every newline so that joining the parts gives the input back byte for byte."""
    parts = content.split(NEWLINE)
    return [part + NEWLINE for part in parts[:-1]] + [parts[-1]]


def _is_project_header(line: str, project_dir: str) -> bool:
    text = line.strip()
    headers = (
        f"{TABLE_OPEN}{PROJECTS_TABLE}{_basic_string(project_dir)}{TABLE_CLOSE}",
        f"{TABLE_OPEN}{PROJECTS_TABLE}{LITERAL_QUOTE}{project_dir}{LITERAL_QUOTE}{TABLE_CLOSE}",
    )
    for header in headers:
        if text.startswith(header):
            rest = text[len(header) :].strip()
            return rest == "" or rest.startswith(COMMENT_MARK)
    return False


def _table_start(lines: list[str], project_dir: str) -> int:
    return next((index for index, line in enumerate(lines) if _is_project_header(line, project_dir)), -1)


def _table_end(lines: list[str], start: int) -> int:
    """The index of the line after the table that starts at `start`: the next table header, or the end of the file."""
    for index in range(start + 1, len(lines)):
        text = lines[index].strip()
        if text.startswith((TABLE_OPEN, ARRAY_TABLE_OPEN)):
            return index
    return len(lines)


def _drop_one_trailing_blank_line(lines: list[str]) -> list[str]:
    last = len(lines) - 1
    while last >= 0 and lines[last] == "":
        last -= 1
    if last >= 0 and lines[last].rstrip(LINE_ENDING_CHARS) == "":
        return [*lines[:last], *lines[last + 1 :]]
    return lines
