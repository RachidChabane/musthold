"""`ProvenanceSource` over the local machine: `.tools/bin` and `.tools/plugins` as `script/product-build` leaves them."""

from __future__ import annotations

import hashlib
from pathlib import Path
from typing import Final

from specbench.adapters.workspace_repo import DiskTreeReader
from specbench.application.ports import ProcessRunner
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


class ProvenanceError(RuntimeError):
    """A product artifact, pin or CLI that cannot be read."""


def file_digest(path: Path) -> str:
    with path.open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


class LocalProvenance:
    """Digests the built binaries and the umbrella plugin tree, reads the Shallnot pin and asks each CLI its version."""

    def __init__(self, repository: Path, runner: ProcessRunner) -> None:
        self._repository = repository
        self._runner = runner

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
        result = self._runner.run([cli, VERSION_FLAG])
        lines = [line.strip() for line in result.stdout.splitlines() if line.strip()]
        if result.returncode != 0 or not lines:
            raise ProvenanceError(f"{cli} {VERSION_FLAG} reported no version")
        return lines[0]
