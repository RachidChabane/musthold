"""Syntax highlighting at build time: a small tokenizer per language and a unified diff renderer.

The output is HTML spans with fixed classes, so the same source always gives the same bytes; the stylesheet
colours the classes in both themes.
"""

from __future__ import annotations

import difflib
import re
from collections.abc import Iterator
from dataclasses import dataclass
from enum import Enum
from typing import Final

from specbench.domain.languages import Language
from specbench.site.html import Markup, element, escape, join

DIFF_CONTEXT_LINES: Final = 3
PYTHON_KEYWORDS: Final = frozenset(
    [
        "False",
        "None",
        "True",
        "and",
        "as",
        "assert",
        "async",
        "await",
        "break",
        "class",
        "continue",
        "def",
        "del",
        "elif",
        "else",
        "except",
        "finally",
        "for",
        "from",
        "global",
        "if",
        "import",
        "in",
        "is",
        "lambda",
        "nonlocal",
        "not",
        "or",
        "pass",
        "raise",
        "return",
        "try",
        "while",
        "with",
        "yield",
        "match",
        "case",
    ]
)
GO_KEYWORDS: Final = frozenset(
    [
        "break",
        "case",
        "chan",
        "const",
        "continue",
        "default",
        "defer",
        "else",
        "fallthrough",
        "for",
        "func",
        "go",
        "goto",
        "if",
        "import",
        "interface",
        "map",
        "package",
        "range",
        "return",
        "select",
        "struct",
        "switch",
        "type",
        "var",
        "nil",
        "true",
        "false",
        "iota",
    ]
)
KEYWORDS: Final = {Language.PYTHON: PYTHON_KEYWORDS, Language.GO: GO_KEYWORDS}
COMMENT_PATTERNS: Final = {Language.PYTHON: r"#[^\n]*", Language.GO: r"//[^\n]*"}
STRING_PATTERNS: Final = {
    Language.PYTHON: r"[rbfuRBFU]{0,2}(?:'(?:\\.|[^'\\\n])*'|\"(?:\\.|[^\"\\\n])*\")",
    Language.GO: r"`[^`]*`|\"(?:\\.|[^\"\\\n])*\"|'(?:\\.|[^'\\\n])*'",
}
NUMBER_PATTERN: Final = r"\b\d[\d_]*(?:\.\d[\d_]*)?(?:[eE][+-]?\d+)?\b"
IDENTIFIER_PATTERN: Final = r"[A-Za-z_][A-Za-z0-9_]*"
DECORATOR_PATTERN: Final = r"@[A-Za-z_][A-Za-z0-9_.]*"


class Token(Enum):
    KEYWORD = "tok-k"
    STRING = "tok-s"
    NUMBER = "tok-n"
    COMMENT = "tok-c"
    DECORATOR = "tok-d"
    TEXT = ""


def _scanner(language: Language) -> re.Pattern[str]:
    parts = [
        f"(?P<comment>{COMMENT_PATTERNS[language]})",
        f"(?P<string>{STRING_PATTERNS[language]})",
        f"(?P<number>{NUMBER_PATTERN})",
        f"(?P<word>{IDENTIFIER_PATTERN})",
    ]
    if language is Language.PYTHON:
        parts.insert(2, f"(?P<decorator>{DECORATOR_PATTERN})")
    return re.compile("|".join(parts))


SCANNERS: Final = {language: _scanner(language) for language in Language}


def tokens(language: Language, line: str) -> Iterator[tuple[Token, str]]:
    """The tokens of one line of source, in order; text between tokens is `Token.TEXT`."""
    position = 0
    for match in SCANNERS[language].finditer(line):
        if match.start() > position:
            yield Token.TEXT, line[position : match.start()]
        group, value = match.lastgroup, match.group()
        if group == "word":
            yield (Token.KEYWORD if value in KEYWORDS[language] else Token.TEXT), value
        elif group == "comment":
            yield Token.COMMENT, value
        elif group == "string":
            yield Token.STRING, value
        elif group == "number":
            yield Token.NUMBER, value
        else:
            yield Token.DECORATOR, value
        position = match.end()
    if position < len(line):
        yield Token.TEXT, line[position:]


def highlight_line(language: Language, line: str) -> Markup:
    return join(
        escape(value) if kind is Token.TEXT else element("span", {"class": kind.value}, value)
        for kind, value in tokens(language, line)
    )


def highlight_code(language: Language, source: str) -> Markup:
    lines = source.rstrip("\n").split("\n")
    body = join(element("span", {"class": "line"}, highlight_line(language, line)) for line in lines)
    return element("pre", {"class": "code"}, element("code", None, body))


class DiffLine(Enum):
    FILE = "diff-file"
    HUNK = "diff-hunk"
    ADDED = "diff-add"
    REMOVED = "diff-del"
    CONTEXT = "diff-ctx"


@dataclass(frozen=True)
class DiffStats:
    added: int
    removed: int


def classify_lines(diff: str) -> list[tuple[DiffLine, str]]:
    """Every line of a unified diff with its kind; `---` and `+++` name files only in a file header."""
    classified: list[tuple[DiffLine, str]] = []
    in_header = True
    for line in diff.rstrip("\n").split("\n"):
        if line.startswith("@@"):
            in_header = False
        elif line.startswith("diff "):
            in_header = True
        kind = DiffLine.FILE if in_header and line.startswith(("+++", "---", "diff ")) else classify(line)
        classified.append((kind, line))
    return classified


def classify(line: str) -> DiffLine:
    """The kind of a line inside a hunk."""
    if line.startswith("@@"):
        return DiffLine.HUNK
    if line.startswith("+"):
        return DiffLine.ADDED
    if line.startswith("-"):
        return DiffLine.REMOVED
    return DiffLine.CONTEXT


def diff_stats(diff: str) -> DiffStats:
    kinds = [kind for kind, _ in classify_lines(diff)]
    return DiffStats(kinds.count(DiffLine.ADDED), kinds.count(DiffLine.REMOVED))


def unified_diff(before: str, after: str, path: str) -> str:
    """The unified diff of one file, with `a/` and `b/` prefixes and no timestamps."""
    lines = difflib.unified_diff(
        before.splitlines(keepends=True),
        after.splitlines(keepends=True),
        fromfile=f"a/{path}",
        tofile=f"b/{path}",
        n=DIFF_CONTEXT_LINES,
    )
    return "".join(line if line.endswith("\n") else f"{line}\n" for line in lines)


def _diff_line(language: Language, kind: DiffLine, line: str) -> Markup:
    if kind in {DiffLine.FILE, DiffLine.HUNK}:
        content: Markup = escape(line)
    else:
        content = join([escape(line[:1]), highlight_line(language, line[1:])])
    return element("span", {"class": f"line {kind.value}"}, content)


def render_diff(language: Language, diff: str, label: str) -> Markup:
    """A highlighted diff; added and removed lines are marked by their sign as well as by colour."""
    body = join((_diff_line(language, kind, line) for kind, line in classify_lines(diff)))
    return element("pre", {"class": "code diff", "aria-label": label}, element("code", None, body))
