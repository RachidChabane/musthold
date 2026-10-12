"""The probe container: what the image and the mounted product say about their versions (PREREG 2)."""

from __future__ import annotations

import json
from collections.abc import Mapping
from typing import Final, cast

from packaging.version import InvalidVersion, Version

from demos1.domain.commands import shell
from demos1.domain.toolchains import PYTHON_MINORS

GO_VERSION_FILE: Final = "go-version.txt"
UV_VERSION_FILE: Final = "uv-version.txt"
GIT_VERSION_FILE: Final = "git-version.txt"
OS_RELEASE_FILE: Final = "os-release.txt"
ARCH_FILE: Final = "arch.txt"
PRODUCT_VERSION_FILE: Final = "spec-proof-version.txt"
UV_PYTHON_FILE: Final = "uv-python-downloads.json"
CPYTHON: Final = "cpython"
DEFAULT_VARIANT: Final = "default"
PRETTY_NAME: Final = "PRETTY_NAME="


def probe_command(directory: str) -> list[str]:
    """One offline command that writes each version to its own file of the probe directory."""

    def into(name: str) -> str:
        return f'"{directory}/{name}"'

    return shell(
        [
            f"go version > {into(GO_VERSION_FILE)}",
            f"uv --version > {into(UV_VERSION_FILE)}",
            f"git --version > {into(GIT_VERSION_FILE)}",
            f"cat /etc/os-release > {into(OS_RELEASE_FILE)}",
            f"uname -m > {into(ARCH_FILE)}",
            f"spec-proof --version > {into(PRODUCT_VERSION_FILE)}",
            f"uv python list --only-downloads --all-versions --output-format json > {into(UV_PYTHON_FILE)}",
        ]
    )


def pretty_name(os_release: str) -> str | None:
    for line in os_release.splitlines():
        if line.startswith(PRETTY_NAME):
            return line.removeprefix(PRETTY_NAME).strip().strip('"')
    return None


def git_version_word(git_version_output: str) -> str | None:
    """`git version 2.39.5` gives `2.39.5`."""
    words = git_version_output.split()
    return words[2] if len(words) > 2 else None


def cpython_by_minor(listing: str) -> dict[str, str]:
    """The CPython release uv selects for each listed minor: its newest stable default build."""
    try:
        entries = json.loads(listing)
    except json.JSONDecodeError:
        return {}
    if not isinstance(entries, list):
        return {}
    newest: dict[str, Version] = {}
    for entry in cast(list[object], entries):
        if not isinstance(entry, Mapping):
            continue
        fields = cast(Mapping[str, object], entry)
        if fields.get("implementation") != CPYTHON or fields.get("variant", DEFAULT_VARIANT) != DEFAULT_VARIANT:
            continue
        raw = fields.get("version")
        if not isinstance(raw, str):
            continue
        try:
            version = Version(raw)
        except InvalidVersion:
            continue
        if version.is_prerelease:
            continue
        minor = f"{version.major}.{version.minor}"
        if minor in PYTHON_MINORS and (minor not in newest or version > newest[minor]):
            newest[minor] = version
    return {minor: str(newest[minor]) for minor in PYTHON_MINORS if minor in newest}
