"""`PinnedClaude` over the private copy `script/claude-code-install` leaves in `.tools/claude-code/<version>/bin/claude`.

The version is the one line of `script/claude-code.pin`. The owner's own `claude` is never run by the bench: it updates
itself, so a sample that used it could span two versions. A session started from the pinned copy runs with
`DISABLE_AUTOUPDATER=1` in its own process environment, which nothing else inherits.
"""

from __future__ import annotations

import os
from collections.abc import Mapping
from pathlib import Path
from typing import Final

from specbench.application.ports import PinnedClaudeUnavailable

PIN_PATH: Final = Path("script") / "claude-code.pin"
TOOLS_DIRECTORY: Final = Path(".tools") / "claude-code"
BINARY_DIRECTORY: Final = "bin"
BINARY_NAME: Final = "claude"
INSTALL_COMMAND: Final = "script/claude-code-install"
UPDATER_VARIABLE: Final = "DISABLE_AUTOUPDATER"
UPDATER_DISABLED: Final = "1"


def updater_disabled(environment: Mapping[str, str]) -> dict[str, str]:
    """The environment of a session with the CLI's self-update off, set on this copy only."""
    return {**environment, UPDATER_VARIABLE: UPDATER_DISABLED}


def pinned_binary(repository: Path, version: str) -> Path:
    return repository / TOOLS_DIRECTORY / version / BINARY_DIRECTORY / BINARY_NAME


class PinnedClaudeCode:
    def __init__(self, repository: Path) -> None:
        self._repository = repository

    @property
    def version(self) -> str:
        pin = self._repository / PIN_PATH
        try:
            version = pin.read_text(encoding="utf-8").strip()
        except FileNotFoundError as error:
            raise PinnedClaudeUnavailable(f"{pin} is missing: it holds the pinned Claude Code version") from error
        if not version:
            raise PinnedClaudeUnavailable(f"{pin} is empty: it holds the pinned Claude Code version")
        return version

    def executable(self) -> str:
        version = self.version
        binary = pinned_binary(self._repository, version)
        if not (binary.is_file() and os.access(binary, os.X_OK)):
            raise PinnedClaudeUnavailable(
                f"the pinned Claude Code {version} is not installed at {binary}: run {INSTALL_COMMAND}"
            )
        return str(binary)
