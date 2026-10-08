"""A shell command line split into simple commands, with wrappers and `sh -c` strings unwrapped.

This is a scanner's view of a command, not a shell: it tokenizes quoting and operators well enough to find which
program a segment runs and with which arguments, and it falls back to whitespace splitting on malformed quoting.
"""

from __future__ import annotations

import posixpath
import re
import shlex
from collections.abc import Mapping
from dataclasses import dataclass
from typing import Final

SEGMENT_BOUNDARIES: Final = frozenset({";", "&&", "||", "|", "&", "|&", "(", ")", "{", "}"})
SHELL_PROGRAMS: Final = frozenset({"sh", "bash", "zsh", "dash", "ksh", "fish"})
SHELL_COMMAND_FLAG_LETTER: Final = "c"
"""The letter that makes a shell run a command string, alone (`-c`) or bundled (`-lc`)."""
MAX_UNWRAP_DEPTH: Final = 6

WRAPPER_VALUE_OPTIONS: Final[Mapping[str, frozenset[str]]] = {
    "sudo": frozenset({"-u", "-g", "-C", "-h", "-p"}),
    "nice": frozenset({"-n"}),
    "ionice": frozenset({"-c", "-n", "-p"}),
    "xargs": frozenset({"-I", "-n", "-P", "-d", "-L", "-s"}),
    "timeout": frozenset({"-s", "-k"}),
    "env": frozenset({"-u", "-C", "-S"}),
    "stdbuf": frozenset({"-i", "-o", "-e"}),
    "nohup": frozenset(),
    "time": frozenset(),
    "command": frozenset(),
    "exec": frozenset(),
    "builtin": frozenset(),
    "setsid": frozenset(),
    "caffeinate": frozenset({"-t"}),
}
_ASSIGNMENT = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*=")
_DURATION = re.compile(r"^\d+(\.\d+)?[smhd]?$")
_BACKTICK_OR_NEWLINE = re.compile(r"[`\n]")


@dataclass(frozen=True)
class SimpleCommand:
    """One program invocation: its basename, its path as written, its arguments and every token of the segment."""

    program: str
    program_word: str
    args: tuple[str, ...]
    words: tuple[str, ...]


def split_words(text: str) -> list[str]:
    prepared = _BACKTICK_OR_NEWLINE.sub(" ; ", text)
    lexer = shlex.shlex(prepared, posix=True, punctuation_chars=True)
    lexer.whitespace_split = True
    try:
        return list(lexer)
    except ValueError:
        return prepared.split()


def _is_operator(word: str) -> bool:
    return bool(word) and all(char in "();<>|&{}" for char in word)


def _segments(words: list[str]) -> list[list[str]]:
    segments: list[list[str]] = [[]]
    for word in words:
        if word in SEGMENT_BOUNDARIES:
            segments.append([])
        elif not _is_operator(word):
            segments[-1].append(word)
    return [segment for segment in segments if segment]


def _skip_assignments(words: list[str]) -> list[str]:
    index = 0
    while index < len(words) and _ASSIGNMENT.match(words[index]):
        index += 1
    return words[index:]


def _skip_wrapper(wrapper: str, words: list[str]) -> list[str]:
    value_options = WRAPPER_VALUE_OPTIONS[wrapper]
    index = 1
    while index < len(words):
        word = words[index]
        if word in value_options:
            index += 2
        elif (
            word.startswith("-")
            or _ASSIGNMENT.match(word)
            or (wrapper in {"timeout", "nice"} and _DURATION.match(word))
        ):
            index += 1
        else:
            break
    return words[index:]


def _shell_command_string(words: list[str]) -> str | None:
    """The script of `sh -lc 'script'`, or `None` when the words are not a shell invoked with a command string."""
    for index, word in enumerate(words[1:], start=1):
        if word.startswith("-") and not word.startswith("--") and SHELL_COMMAND_FLAG_LETTER in word:
            return words[index + 1] if index + 1 < len(words) else None
        if not word.startswith("-"):
            return None
    return None


def _unwrap(words: list[str], depth: int) -> list[SimpleCommand]:
    words = _skip_assignments(words)
    if not words:
        return []
    program = posixpath.basename(words[0])
    if program in WRAPPER_VALUE_OPTIONS:
        return _unwrap(_skip_wrapper(program, words), depth)
    if program in SHELL_PROGRAMS and depth < MAX_UNWRAP_DEPTH:
        script = _shell_command_string(words)
        if script is not None:
            return simple_commands(script, depth + 1)
    return [SimpleCommand(program, words[0], tuple(words[1:]), tuple(words))]


def simple_commands(command: str, depth: int = 0) -> list[SimpleCommand]:
    """Every simple command of a command line, including those inside wrappers and `-c` scripts."""
    found: list[SimpleCommand] = []
    for segment in _segments(split_words(command)):
        found.extend(_unwrap(segment, depth))
    return found
