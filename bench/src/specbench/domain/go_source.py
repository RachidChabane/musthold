"""The two header facts of a Go source file that the benchmark reads: its package clause and its import declarations.

A file is scanned as tokens, comments dropped and string literals kept whole, and only up to the end of its import
declarations. Nothing after them is looked at, so a string literal in a function body is never mistaken for an import.
"""

from __future__ import annotations

import re
from collections.abc import Iterator
from dataclasses import dataclass
from typing import Final

TOKEN: Final = re.compile(
    r"""
    (?P<skip>\s+|//[^\n]*|/\*.*?\*/)
    |(?P<string>"(?:\\.|[^"\\\n])*"|`[^`]*`)
    |(?P<rune>'(?:\\.|[^'\\\n])+')
    |(?P<word>[A-Za-z_][A-Za-z0-9_]*)
    |(?P<punct>.)
    """,
    re.VERBOSE | re.DOTALL,
)
PACKAGE_KEYWORD: Final = "package"
IMPORT_KEYWORD: Final = "import"
GROUP_OPEN: Final = "("
GROUP_CLOSE: Final = ")"
DECLARATION_SEPARATOR: Final = ";"
DOT_ALIAS: Final = "."
BLANK_ALIAS: Final = "_"


@dataclass(frozen=True)
class GoImport:
    """One import spec: the path, and the alias when one is written (`.` and `_` included)."""

    path: str
    alias: str | None = None

    @property
    def qualifier(self) -> str:
        """The name the importing file uses for the package: the alias, else the last element of the path."""
        return self.alias if self.alias is not None else self.path.rstrip("/").rsplit("/", 1)[-1]


def _tokens(source: str) -> Iterator[tuple[str, str]]:
    for match in TOKEN.finditer(source):
        kind = match.lastgroup
        if kind is not None and kind != "skip":
            yield kind, match.group()


def _string_value(literal: str) -> str:
    return literal[1:-1]


def go_package_name(source: str) -> str | None:
    """The name in the package clause, or None when the file has none."""
    tokens = _tokens(source)
    for kind, value in tokens:
        if kind == "word" and value == PACKAGE_KEYWORD:
            following = next(tokens, None)
            return following[1] if following is not None and following[0] == "word" else None
        return None
    return None


def go_imports(source: str) -> list[GoImport]:
    """Every import spec of the file's import declarations: single and grouped, aliased, dot and blank."""
    tokens = list(_tokens(source))
    found: list[GoImport] = []
    position = _after_package_clause(tokens)
    while position < len(tokens) and tokens[position] == ("word", IMPORT_KEYWORD):
        position += 1
        if position < len(tokens) and tokens[position][1] == GROUP_OPEN:
            position = _group(tokens, position + 1, found)
        else:
            position = _spec(tokens, position, found)
        while position < len(tokens) and tokens[position][1] == DECLARATION_SEPARATOR:
            position += 1
    return found


def _after_package_clause(tokens: list[tuple[str, str]]) -> int:
    if len(tokens) >= 2 and tokens[0] == ("word", PACKAGE_KEYWORD):
        position = 2
        while position < len(tokens) and tokens[position][1] == DECLARATION_SEPARATOR:
            position += 1
        return position
    return len(tokens)


def _group(tokens: list[tuple[str, str]], position: int, found: list[GoImport]) -> int:
    while position < len(tokens) and tokens[position][1] != GROUP_CLOSE:
        if tokens[position][1] == DECLARATION_SEPARATOR:
            position += 1
            continue
        advanced = _spec(tokens, position, found)
        position = advanced if advanced > position else position + 1
    return position + 1


def _spec(tokens: list[tuple[str, str]], position: int, found: list[GoImport]) -> int:
    alias: str | None = None
    if position < len(tokens) and (tokens[position][0] == "word" or tokens[position][1] == DOT_ALIAS):
        alias = tokens[position][1]
        position += 1
    if position < len(tokens) and tokens[position][0] == "string":
        found.append(GoImport(_string_value(tokens[position][1]), alias))
        return position + 1
    return position
