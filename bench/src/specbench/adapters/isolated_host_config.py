"""What the isolated gate host needs from the machine: the product build, the plugin trees, the rooms and the pinned
Claude Code CLI its sessions run."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.adapters.area_environment import SHALLNOT_BINARY
from specbench.application.ports import PinnedClaude

GH_SHIM_DIRECTORY: Final = Path("bench") / "script" / "gh-shim"
"""The directory, below the repository root, whose `gh` serves the bench's own `host pull`; no session path holds it."""


@dataclass(frozen=True)
class IsolatedHostConfig:
    repository: Path
    binaries: Path
    umbrella_plugin: Path
    rooms_root: Path
    claude: PinnedClaude
    tool_path: tuple[Path, ...] = ()

    @property
    def shallnot(self) -> Path:
        return self.binaries / SHALLNOT_BINARY

    @property
    def gh_shim(self) -> Path:
        return self.repository / GH_SHIM_DIRECTORY
