"""Where a gate runs: the deterministic clean room of one (task, arm, tree) and its two working areas."""

from __future__ import annotations

from collections.abc import Iterable, Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.domain.arms import Arm
from specbench.domain.canonical import JsonValue

TREE_DIRECTORY: Final = "tree"
STATE_DIRECTORY: Final = "state"
CACHE_DIRECTORY: Final = "cache"
CONFIG_DIRECTORY: Final = "claude-config"
READY_MARKER: Final = "ready"
SESSION_AREA: Final = "session"
SETTLE_AREA: Final = "settle"
PORTABLE_ROOT: Final = "<clean-room>"
"""What the clean room's tree path reads as in a stored report, so that a report does not depend on where it ran."""


@dataclass(frozen=True)
class RoomArea:
    """One working area: a copy of the tree with the host store and the proof cache of the modules beside it."""

    root: Path

    @property
    def tree(self) -> Path:
        return self.root / TREE_DIRECTORY

    @property
    def state(self) -> Path:
        return self.root / STATE_DIRECTORY

    @property
    def cache(self) -> Path:
        return self.root / CACHE_DIRECTORY

    @property
    def config_directory(self) -> Path:
        return self.root / CONFIG_DIRECTORY

    @property
    def ready_marker(self) -> Path:
        return self.root / READY_MARKER

    @property
    def is_ready(self) -> bool:
        return self.ready_marker.is_file()


@dataclass(frozen=True)
class CleanRoom:
    """`<rooms root>/<arm>/<tree digest>/`: the session area survives a killed gate, the settle area is rebuilt each time."""

    root: Path

    @classmethod
    def locate(cls, rooms_root: Path, arm: Arm, tree_digest: str) -> CleanRoom:
        return cls(rooms_root / arm.value / tree_digest)

    @property
    def session(self) -> RoomArea:
        return RoomArea(self.root / SESSION_AREA)

    @property
    def settle(self) -> RoomArea:
        return RoomArea(self.root / SETTLE_AREA)


def portable(value: JsonValue, tree_paths: Iterable[str]) -> JsonValue:
    """`value` with each of `tree_paths` replaced by `PORTABLE_ROOT` in every string, so that it names no machine path."""
    prefixes = sorted(set(tree_paths), key=len, reverse=True)
    if isinstance(value, str):
        for prefix in prefixes:
            value = value.replace(prefix, PORTABLE_ROOT)
        return value
    if isinstance(value, Mapping):
        return {key: portable(item, prefixes) for key, item in value.items()}
    if isinstance(value, list | tuple):
        return [portable(item, prefixes) for item in value]
    return value
