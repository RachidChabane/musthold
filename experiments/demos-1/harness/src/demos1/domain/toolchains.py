"""The toolchain rules of PREREG 8.2: the Go directives a pick may use and the Python version it requests."""

from __future__ import annotations

import configparser
import re
import tomllib
from collections.abc import Mapping
from dataclasses import dataclass
from enum import StrEnum
from typing import Final

from packaging.specifiers import InvalidSpecifier, SpecifierSet
from packaging.version import InvalidVersion, Version

GO_VERSION: Final = "1.26.4"
PYTHON_MINORS: Final = ("3.13", "3.12", "3.11", "3.10")
DEFAULT_PYTHON: Final = "3.13"

_GO_DIRECTIVE = re.compile(r"^\s*go\s+(\S+)\s*(?://.*)?$")
_TOOLCHAIN_DIRECTIVE = re.compile(r"^\s*toolchain\s+go(\S+)\s*(?://.*)?$")
_GO_VERSION = re.compile(r"^(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:(rc|beta)(\d+))?$")
_RELEASE_KIND = {"beta": 0, "rc": 1, "": 2}
"""Go orders a language version (1.21) before its pre-releases (1.21rc1) and those before the release (1.21.0)."""


class GoVersionError(ValueError):
    """A go.mod version this tooling cannot order."""


def go_version_key(version: str) -> tuple[int, int, int, int, int]:
    """The order of Go versions, as `go/version` compares them."""
    match = _GO_VERSION.match(version)
    if match is None:
        raise GoVersionError(f"unrecognized Go version {version!r}")
    major, minor, patch, kind, number = match.groups()
    if patch is not None:
        return int(major), int(minor or 0), 3, int(patch), 0
    if kind is not None:
        return int(major), int(minor or 0), _RELEASE_KIND[kind], int(number), 0
    return int(major), int(minor or 0), -1, 0, 0


@dataclass(frozen=True)
class GoDirectives:
    go: str | None
    toolchain: str | None


def go_directives(go_mod: str) -> GoDirectives:
    go: str | None = None
    toolchain: str | None = None
    for line in go_mod.splitlines():
        if go is None and (match := _GO_DIRECTIVE.match(line)):
            go = match.group(1)
        elif toolchain is None and (match := _TOOLCHAIN_DIRECTIVE.match(line)):
            toolchain = match.group(1)
    return GoDirectives(go, toolchain)


def go_exceeds_image(directives: GoDirectives) -> bool:
    """`setup-toolchain` when the go directive, or the toolchain directive when present, exceeds 1.26.4."""
    limit = go_version_key(GO_VERSION)
    return any(go_version_key(value) > limit for value in (directives.go, directives.toolchain) if value is not None)


class PythonRule(StrEnum):
    PYTHON_VERSION_FILE = "python-version"
    REQUIRES_PYTHON = "requires-python"
    PYTHON_REQUIRES = "python_requires"
    DEFAULT = "default"


@dataclass(frozen=True)
class PythonRequest:
    """The version request of a pytest pick, or none when no listed version satisfies its specifier."""

    rule: PythonRule
    request: str | None
    specifier: str | None = None
    unreadable: str | None = None
    """A configuration file that could not be parsed and so gave no specifier."""


def first_line(python_version: str) -> str | None:
    for line in python_version.splitlines():
        if line.strip():
            return line.strip()
    return None


def requires_python(pyproject: str) -> str | None:
    project = tomllib.loads(pyproject).get("project")
    if not isinstance(project, dict):
        return None
    value = project.get("requires-python")  # pyright: ignore[reportUnknownMemberType, reportUnknownVariableType]
    return value if isinstance(value, str) else None


def python_requires(setup_cfg: str) -> str | None:
    parser = configparser.ConfigParser(interpolation=None)
    parser.read_string(setup_cfg)
    if parser.has_option("options", "python_requires"):
        return parser.get("options", "python_requires").strip()
    return None


def highest_satisfying(specifier: str, uv_releases: Mapping[str, str]) -> str | None:
    """The highest of 3.13, 3.12, 3.11 and 3.10 whose CPython release, as uv selects it, satisfies the specifier."""
    try:
        specifiers = SpecifierSet(specifier)
    except InvalidSpecifier:
        return None
    for minor in PYTHON_MINORS:
        release = uv_releases.get(minor)
        if release is None:
            continue
        try:
            if specifiers.contains(Version(release), prereleases=True):
                return minor
        except InvalidVersion:
            continue
    return None


def python_request(
    python_version: str | None,
    pyproject: str | None,
    setup_cfg: str | None,
    uv_releases: Mapping[str, str],
) -> PythonRequest:
    """PREREG 8.2: the first applicable rule decides; the files are the root files of the checkout."""
    if python_version is not None and (line := first_line(python_version)) is not None:
        return PythonRequest(PythonRule.PYTHON_VERSION_FILE, line)
    unreadable: str | None = None
    if pyproject is not None:
        try:
            specifier = requires_python(pyproject)
        except tomllib.TOMLDecodeError:
            specifier, unreadable = None, "pyproject.toml"
        if specifier is not None:
            return PythonRequest(PythonRule.REQUIRES_PYTHON, highest_satisfying(specifier, uv_releases), specifier)
    if setup_cfg is not None:
        try:
            specifier = python_requires(setup_cfg)
        except configparser.Error:
            specifier, unreadable = None, unreadable or "setup.cfg"
        if specifier is not None:
            return PythonRequest(PythonRule.PYTHON_REQUIRES, highest_satisfying(specifier, uv_releases), specifier)
    return PythonRequest(PythonRule.DEFAULT, DEFAULT_PYTHON, unreadable=unreadable)


_CPYTHON_INSTALL = re.compile(r"^cpython-(\d+\.\d+\.\d+[a-z0-9]*)(?:\+[a-z0-9]+)?-")


def installed_cpython(directory_names: list[str]) -> list[str]:
    """The CPython versions `uv python install` left in UV_PYTHON_INSTALL_DIR, from their directory names."""
    versions = [match.group(1) for name in sorted(directory_names) if (match := _CPYTHON_INSTALL.match(name))]
    return sorted(set(versions))
