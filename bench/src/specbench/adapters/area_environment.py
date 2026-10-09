"""The environment of a command run inside a clean-room area: the product binaries first, the area's state and cache."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Final

from specbench.domain.clean_room import RoomArea

SHALLNOT_BINARY: Final = "shallnot"
SHALLNOT_ENV: Final = "SPEC_PROOF_SHALLNOT"
STATE_ENV: Final = "XDG_STATE_HOME"
CACHE_ENV: Final = "XDG_CACHE_HOME"
PATH_ENV: Final = "PATH"
PATH_SEPARATOR: Final = ":"


class AreaEnvironment:
    """The variables every product command of an area runs with; no `gh` shim is ever on this path."""

    def __init__(self, binaries: Path, tool_path: Sequence[Path], base_environment: Mapping[str, str]) -> None:
        self._binaries = binaries
        self._tool_path = tuple(tool_path)
        self._base = base_environment

    def of(self, area: RoomArea) -> dict[str, str]:
        search_path = [str(self._binaries), *map(str, self._tool_path)]
        inherited = self._base.get(PATH_ENV)
        return {
            PATH_ENV: PATH_SEPARATOR.join([*search_path, *([inherited] if inherited else [])]),
            STATE_ENV: str(area.state),
            CACHE_ENV: str(area.cache),
            SHALLNOT_ENV: str(self._binaries / SHALLNOT_BINARY),
        }
