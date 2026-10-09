"""`ProvenanceSource` over the local machine: `.tools/bin` and `.tools/plugins` as `script/product-build` leaves them,
and the pinned Claude Code CLI as `script/claude-code-install` leaves it."""

from __future__ import annotations

import hashlib
import os
from pathlib import Path
from typing import Final

from specbench.adapters.pinned_claude_code import updater_disabled
from specbench.adapters.workspace_repo import DiskTreeReader
from specbench.application.ports import PinnedClaude, PinnedClaudeUnavailable, ProcessRunner
from specbench.domain.manifest import (
    CODING_AGENT_CLIS,
    PRODUCT_BINARIES,
    UMBRELLA_PLUGIN,
    Provenance,
)
from specbench.domain.sealing import tree_digest

TOOLS_DIRECTORY: Final = Path(".tools")
BINARY_DIRECTORY: Final = "bin"
PLUGIN_DIRECTORY: Final = "plugins"
UMBRELLA_PLUGIN_DIRECTORY: Final = "spec-proof"
SHALLNOT_PIN_PATH: Final = Path("script") / "shallnot.pin"
VERSION_FLAG: Final = "--version"
BUILD_COMMAND: Final = "script/product-build"
CLAUDE_CLI: Final = "claude"


class ProvenanceError(RuntimeError):
    """A product artifact, pin or CLI that cannot be read."""


def file_digest(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


class LocalProvenance:
    """Digests the built binaries and the umbrella plugin tree, reads the Shallnot pin and asks each CLI its version.

    Claude Code is asked through the pinned binary, and its recorded version is the pin, which it must report.
    """

    def __init__(self, repository: Path, runner: ProcessRunner, claude: PinnedClaude) -> None:
        self._repository = repository
        self._runner = runner
        self._claude = claude

    def read(self) -> Provenance:
        return Provenance(
            product_digests=self._product_digests(),
            shallnot_commit=self._shallnot_commit(),
            cli_versions={cli: self._cli_version(cli) for cli in CODING_AGENT_CLIS},
        )

    def _product_digests(self) -> dict[str, str]:
        binaries = self._repository / TOOLS_DIRECTORY / BINARY_DIRECTORY
        plugin = self._repository / TOOLS_DIRECTORY / PLUGIN_DIRECTORY / UMBRELLA_PLUGIN_DIRECTORY
        missing = [name for name in PRODUCT_BINARIES if not (binaries / name).is_file()]
        if not plugin.is_dir():
            missing.append(UMBRELLA_PLUGIN)
        if missing:
            raise ProvenanceError(f"not built: {', '.join(missing)} (run {BUILD_COMMAND})")
        digests = {name: file_digest(binaries / name) for name in PRODUCT_BINARIES}
        digests[UMBRELLA_PLUGIN] = tree_digest(DiskTreeReader().read(plugin))
        return digests

    def _shallnot_commit(self) -> str:
        return (self._repository / SHALLNOT_PIN_PATH).read_text(encoding="utf-8").strip()

    def _cli_version(self, cli: str) -> str:
        if cli == CLAUDE_CLI:
            return self._claude_version()
        return self._reported_version(cli, cli)

    def _claude_version(self) -> str:
        try:
            pinned = self._claude.version
            reported = self._reported_version(CLAUDE_CLI, self._claude.executable())
        except PinnedClaudeUnavailable as error:
            raise ProvenanceError(str(error)) from error
        if reported.split()[0] != pinned:
            raise ProvenanceError(f"{CLAUDE_CLI} {VERSION_FLAG} reported {reported!r}, the pin is {pinned}")
        return pinned

    def _reported_version(self, cli: str, executable: str) -> str:
        result = self._runner.run([executable, VERSION_FLAG], None, updater_disabled(os.environ))
        lines = [line.strip() for line in result.stdout.splitlines() if line.strip()]
        if result.returncode != 0 or not lines:
            raise ProvenanceError(f"{cli} {VERSION_FLAG} reported no version")
        return lines[0]
