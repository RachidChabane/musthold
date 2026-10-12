"""The fixed commands of PREREG 8.2 to 9.2, as the argv each container command runs."""

from __future__ import annotations

import shlex
import tomllib
from collections.abc import Collection, Mapping, Sequence
from dataclasses import dataclass
from enum import StrEnum
from typing import Final, cast

from demos1.domain.ecosystem import Ecosystem

SHELL: Final = ("/bin/sh", "-c")
VENV_PYTHON: Final = ".venv/bin/python"
GITMODULES: Final = ".gitmodules"
UV_LOCK: Final = "uv.lock"
PROJECT_FILES: Final = ("pyproject.toml", "setup.py", "setup.cfg")
EXTRA_NAMES: Final = ("test", "tests", "testing", "dev")
"""The optional-dependency keys and dependency groups the recipe installs, in this order."""
REQUIREMENTS_TXT: Final = "requirements.txt"
EXTRA_REQUIREMENTS: Final = (
    "requirements-dev.txt",
    "requirements_dev.txt",
    "dev-requirements.txt",
    "requirements-test.txt",
    "requirements_test.txt",
    "test-requirements.txt",
    "requirements/dev.txt",
    "requirements/test.txt",
    "requirements/tests.txt",
)
SCAN_SEED: Final = 7
SCAN_WORKERS: Final = 2
FULL_BUDGET: Final = "60m"
GO_TEST_PROCESSES: Final = 2


def shell(script: Sequence[str]) -> list[str]:
    """One container command that runs several commands, stopping at the first that fails."""
    return [*SHELL, "\n".join(["set -eu", *script])]


def quoted(*words: str) -> str:
    return " ".join(shlex.quote(word) for word in words)


def clone_command(url: str, clone: str, commit: str, base: str | None) -> list[str]:
    """PREREG 8.2: the full clone, the detached checkout, the submodules, and for a B pick the base commit."""
    script = [
        quoted("git", "clone", "--no-checkout", url, clone),
        quoted("git", "-C", clone, "checkout", "--detach", commit),
        f"if [ -e {quoted(clone + '/' + GITMODULES)} ]; then "
        + quoted("git", "-C", clone, "submodule", "update", "--init", "--recursive")
        + "; fi",
    ]
    if base is not None:
        script.append(quoted("git", "-C", clone, "cat-file", "-e", f"{base}^{{commit}}"))
    return shell(script)


def python_install_command(request: str) -> list[str]:
    return ["uv", "python", "install", request]


def clean_tree_command(clone: str) -> list[str]:
    return ["git", "-C", clone, "status", "--porcelain", "--untracked-files=no"]


@dataclass(frozen=True)
class ProjectFacts:
    """What the host reads from the checkout's files to choose the recipe of PREREG 8.3."""

    present: Collection[str]
    pyproject: str | None

    def has(self, path: str) -> bool:
        return path in self.present


def _child(node: object, name: str) -> object:
    if not isinstance(node, Mapping):
        return None
    return cast(Mapping[str, object], node).get(name)


def _table_keys(document: object, *path: str) -> set[str]:
    node = document
    for name in path:
        node = _child(node, name)
    if not isinstance(node, Mapping):
        return set()
    return {str(key) for key in cast(Mapping[str, object], node)}


@dataclass(frozen=True)
class PyprojectTables:
    extras: tuple[str, ...]
    groups: tuple[str, ...]
    unreadable: bool = False


def pyproject_tables(pyproject: str | None) -> PyprojectTables:
    """The listed keys of `[project.optional-dependencies]` and of `[dependency-groups]`, in the recipe's order."""
    if pyproject is None:
        return PyprojectTables((), ())
    try:
        document = tomllib.loads(pyproject)
    except tomllib.TOMLDecodeError:
        return PyprojectTables((), (), unreadable=True)
    extras = _table_keys(document, "project", "optional-dependencies")
    groups = _table_keys(document, "dependency-groups")
    return PyprojectTables(
        tuple(name for name in EXTRA_NAMES if name in extras),
        tuple(name for name in EXTRA_NAMES if name in groups),
    )


def _install_pytest_when_missing() -> str:
    return (
        f'if ! {VENV_PYTHON} -c "import pytest"; then '
        + quoted("uv", "pip", "install", "--python", VENV_PYTHON, "pytest")
        + "; fi"
    )


def install_command(ecosystem: Ecosystem, facts: ProjectFacts, request: str | None) -> list[str]:
    """PREREG 8.3: the first applicable recipe, as one command run from the clone's root."""
    if ecosystem is Ecosystem.GO:
        return ["go", "mod", "download"]
    if request is None:
        raise ValueError("a pytest recipe needs the Python request")
    if facts.has(UV_LOCK):
        return shell(
            [
                quoted("uv", "sync", "--frozen", "--all-extras", "--all-groups", "--python", request),
                _install_pytest_when_missing(),
            ]
        )
    script = [quoted("uv", "venv", ".venv", "--python", request)]
    project_installed = any(facts.has(name) for name in PROJECT_FILES)
    tables = pyproject_tables(facts.pyproject if facts.has("pyproject.toml") else None)
    if project_installed:
        target = f".[{','.join(tables.extras)}]" if tables.extras else "."
        script.append(quoted("uv", "pip", "install", "--python", VENV_PYTHON, "-e", target))
    script += [quoted("uv", "pip", "install", "--python", VENV_PYTHON, "--group", name) for name in tables.groups]
    requirements = ([] if project_installed else [REQUIREMENTS_TXT]) + list(EXTRA_REQUIREMENTS)
    script += [
        quoted("uv", "pip", "install", "--python", VENV_PYTHON, "-r", name) for name in requirements if facts.has(name)
    ]
    script.append(_install_pytest_when_missing())
    return shell(script)


def recipe_reads(ecosystem: Ecosystem) -> tuple[str, ...]:
    """The root paths whose presence the recipe of an ecosystem depends on."""
    if ecosystem is Ecosystem.GO:
        return ()
    return (UV_LOCK, *PROJECT_FILES, REQUIREMENTS_TXT, *EXTRA_REQUIREMENTS)


class SuiteFormat(StrEnum):
    JUNIT = ".xml"
    GO_JSON = ".jsonl"


SUITE_FORMAT: Final = {Ecosystem.PYTEST: SuiteFormat.JUNIT, Ecosystem.GO: SuiteFormat.GO_JSON}


def suite_command(ecosystem: Ecosystem, output: str) -> list[str]:
    """PREREG 8.4: one fixed command per ecosystem, run from the clone's root."""
    if ecosystem is Ecosystem.PYTEST:
        return [VENV_PYTHON, "-m", "pytest", "-p", "no:cacheprovider", "-q", f"--junitxml={output}"]
    return shell(
        [quoted("go", "test", "-count=1", "-p", str(GO_TEST_PROCESSES), "-json", "./...") + " > " + shlex.quote(output)]
    )


class ScanName(StrEnum):
    CHANGE = "change"
    QUICK = "quick"
    CHANGE_FULL = "change-full"
    FULL = "full"


B_SCANS: Final = (ScanName.CHANGE, ScanName.QUICK, ScanName.CHANGE_FULL)
A_SCANS: Final = (ScanName.QUICK, ScanName.FULL)
SHOWN_ORDER: Final = {
    "B": (ScanName.CHANGE_FULL, ScanName.CHANGE, ScanName.QUICK),
    "A": (ScanName.FULL, ScanName.QUICK),
}
"""PREREG 12.1: the scans whose `#1` is shown, in the order they are tried."""


def scan_flags(scan: ScanName, base: str | None) -> list[str]:
    """The added flags of the table of PREREG 9.1."""
    match scan:
        case ScanName.CHANGE:
            return ["--base", _required(base)]
        case ScanName.QUICK:
            return []
        case ScanName.CHANGE_FULL:
            return ["--base", _required(base), "--full", "--budget", FULL_BUDGET]
        case ScanName.FULL:
            return ["--full", "--budget", FULL_BUDGET]


def _required(base: str | None) -> str:
    if base is None:
        raise ValueError("a change scan needs the base commit")
    return base


def scan_command(clone: str, scan: ScanName, base: str | None) -> list[str]:
    return [
        "spec-proof",
        "scan",
        "--root",
        clone,
        "--seed",
        str(SCAN_SEED),
        "--workers",
        str(SCAN_WORKERS),
        "--format",
        "text",
        *scan_flags(scan, base),
    ]


def replay_command(clone: str, finding_folder: str) -> list[str]:
    return ["spec-proof", "scan", "replay", "--root", clone, finding_folder]
