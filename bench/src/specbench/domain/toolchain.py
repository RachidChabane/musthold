"""Which Python a task's environment is built with: the upstream's own pin, else the newest compatible with its metadata.

The rule is deterministic and uses only files of the base tree, so the same task always gets the same interpreter.
"""

from __future__ import annotations

import re
from typing import Final

SUPPORTED_PYTHONS: Final = ((3, 9), (3, 10), (3, 11), (3, 12), (3, 13))
DEFAULT_PYTHON: Final = (3, 13)
PYTHON_VERSION_FILE: Final = ".python-version"
PIN_PATTERN: Final = re.compile(r"^(\d+)\.(\d+)")
SPECIFIER_PATTERN: Final = re.compile(r"^\s*(>=|<=|==|!=|~=|>|<)\s*(\d+)(?:\.(\d+))?")
LOCKED_REQUIRES: Final = re.compile(r'^requires-python = "([^"]+)"', re.MULTILINE)
PROJECT_REQUIRES: Final = re.compile(r'^requires-python\s*=\s*"([^"]+)"', re.MULTILINE)

Version = tuple[int, int]


def format_version(version: Version) -> str:
    return f"{version[0]}.{version[1]}"


def pinned_python(version_file: str | None) -> Version | None:
    """The `major.minor` of the first line of a `.python-version` file that names a CPython release."""
    for line in (version_file or "").splitlines():
        match = PIN_PATTERN.match(line.strip())
        if match is not None:
            return int(match.group(1)), int(match.group(2))
    return None


def satisfies(version: Version, requires: str) -> bool:
    """Whether `major.minor` meets every clause of a `requires-python` specifier, compared at minor granularity."""
    for clause in requires.split(","):
        match = SPECIFIER_PATTERN.match(clause)
        if match is None:
            continue
        operator, bound = match.group(1), (int(match.group(2)), int(match.group(3) or 0))
        allowed = {
            ">=": version >= bound,
            ">": version > bound,
            "<=": version <= bound,
            "<": version < bound,
            "==": version == bound or (match.group(3) is None and version[0] == bound[0]),
            "!=": version != bound,
            "~=": version >= bound and version[0] == bound[0],
        }[operator]
        if not allowed:
            return False
    return True


def declared_requirement(*texts: str | None) -> str | None:
    """The first `requires-python` found in the given `uv.lock` or `pyproject.toml` texts."""
    for body in texts:
        for pattern in (LOCKED_REQUIRES, PROJECT_REQUIRES):
            match = pattern.search(body or "")
            if match is not None:
                return match.group(1)
    return None


def python_for(version_file: str | None, requires: str | None) -> Version:
    """The pin if the upstream has one; else the default when it satisfies the requirement, else the oldest that does."""
    pinned = pinned_python(version_file)
    if pinned is not None:
        return pinned
    if requires is None or satisfies(DEFAULT_PYTHON, requires):
        return DEFAULT_PYTHON
    return next((v for v in SUPPORTED_PYTHONS if satisfies(v, requires)), DEFAULT_PYTHON)
