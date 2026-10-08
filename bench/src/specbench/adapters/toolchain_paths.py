"""The install directories of the toolchain binaries, resolved once per run and recorded in it.

A coder session runs with a home that is not the owner's, so a mise shim cannot resolve there. The directories come
from `script/resolve-tool-dirs`, which the night-job installer shares, run with the owner's own environment. The
record in the run directory makes a resumed run use the same paths as the run it resumes.
"""

from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Final

from specbench.adapters.run_store import write_atomically
from specbench.application.ports import ProcessRunner
from specbench.domain.canonical import canonical_json
from specbench.domain.jsonread import mapping, text
from specbench.domain.manifest import RunLayout

RESOLVER_SCRIPT: Final = Path(__file__).resolve().parents[4] / "script" / "resolve-tool-dirs"
TOOLS: Final = ("uv", "go", "python3", "node", "npm", "java", "mvn", "gradle", "jq")
TOOLS_KEY: Final = "tools"
FIELD_SEPARATOR: Final = "\t"
ENCODING: Final = "utf-8"


class ToolchainResolutionError(RuntimeError):
    """The resolver script failed."""


class ToolchainPaths:
    def __init__(
        self,
        runner: ProcessRunner,
        layout: RunLayout,
        environ: Mapping[str, str] | None = None,
        script: Path = RESOLVER_SCRIPT,
        tools: Sequence[str] = TOOLS,
    ) -> None:
        self._runner = runner
        self._layout = layout
        self._environ = environ
        self._script = script
        self._tools = tuple(tools)

    def resolve(self) -> dict[str, str]:
        """The install directory of each tool that resolves; read from the run when an earlier attempt recorded it."""
        record = self._layout.toolchain_paths
        if record.is_file():
            return _read(record)
        result = self._runner.run([str(self._script), *self._tools], None, self._environ)
        if result.returncode != 0:
            raise ToolchainResolutionError(f"{self._script} failed: {result.stderr.strip()}")
        resolved = dict(
            line.split(FIELD_SEPARATOR, 1) for line in result.stdout.splitlines() if FIELD_SEPARATOR in line
        )
        write_atomically(record, canonical_json({TOOLS_KEY: resolved}))
        return resolved

    def directories(self) -> tuple[str, ...]:
        """The distinct directories, in tool order, for the front of a session's `PATH`."""
        return tuple(dict.fromkeys(self.resolve().values()))


def _read(record: Path) -> dict[str, str]:
    tools = mapping(
        mapping(json.loads(record.read_text(encoding=ENCODING)), "toolchain record").get(TOOLS_KEY), TOOLS_KEY
    )
    return {name: text(directory, f"{TOOLS_KEY}.{name}") for name, directory in tools.items()}
