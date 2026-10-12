"""PREREG 13: the colima machine the engine runs in, and the disk bounds of an attempt."""

from __future__ import annotations

import json
from collections.abc import Iterable
from dataclasses import dataclass
from pathlib import PurePosixPath
from typing import Final, cast

from demos1.domain.environment import Fact

PROFILE: Final = "spec-proof-demos"
MACHINE_CPUS: Final = 2
MACHINE_MEMORY_GIB: Final = 3
MACHINE_DISK_GIB: Final = 40
GIB: Final = 1 << 30
MIN_FREE_BYTES: Final = 50 * GIB
"""An attempt starts only when the volume of $DEMOS1_WORK has at least this much free."""
ATTEMPT_CAP_BYTES: Final = 15 * GIB
"""Above this size of the attempt directory, measured after each container command, the stage ends on disk."""
WRITABLE_SUFFIX: Final = ":w"


def docker_socket(home: PurePosixPath, profile: str = PROFILE) -> str:
    """The `DOCKER_HOST` of the profile: every docker command of the tooling names it."""
    return f"unix://{home}/.colima/{profile}/docker.sock"


@dataclass(frozen=True)
class MachineMounts:
    """The machine's only mounts: $DEMOS1_WORK read-write and the product binaries read-only."""

    work: PurePosixPath
    binaries: PurePosixPath

    def __post_init__(self) -> None:
        if self.binaries.is_relative_to(self.work) or self.work.is_relative_to(self.binaries):
            raise ValueError("the product binaries must lie outside $DEMOS1_WORK to be mounted read-only")


def start_argv(mounts: MachineMounts, profile: str = PROFILE) -> list[str]:
    """`colima start` of the profile; it neither activates a docker context nor writes the host's ssh config."""
    return [
        "colima",
        "start",
        "--profile",
        profile,
        "--cpu",
        str(MACHINE_CPUS),
        "--memory",
        str(MACHINE_MEMORY_GIB),
        "--disk",
        str(MACHINE_DISK_GIB),
        "--activate=false",
        "--ssh-config=false",
        "--mount",
        f"{mounts.work}{WRITABLE_SUFFIX}",
        "--mount",
        str(mounts.binaries),
    ]


def stop_argv(profile: str = PROFILE) -> list[str]:
    return ["colima", "stop", "--profile", profile]


def list_argv(profile: str = PROFILE) -> list[str]:
    return ["colima", "list", "--profile", profile, "--json"]


def machine_settings(colima_version: str, listing: str, mounts: MachineMounts | None) -> dict[str, str]:
    """The colima version and the profile's settings, from `colima version` and `colima list --json`."""
    words = colima_version.split()
    facts = {Fact.COLIMA_VERSION.value: words[2] if len(words) > 2 else ""}
    for line in listing.splitlines():
        try:
            entry: object = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(entry, dict):
            continue
        fields = cast(dict[str, object], entry)
        facts |= {
            Fact.MACHINE_PROFILE.value: str(fields.get("name", "")),
            Fact.MACHINE_CPUS.value: str(fields.get("cpus", "")),
            Fact.MACHINE_MEMORY_BYTES.value: str(fields.get("memory", "")),
            Fact.MACHINE_DISK_BYTES.value: str(fields.get("disk", "")),
            Fact.MACHINE_ARCH.value: str(fields.get("arch", "")),
            Fact.MACHINE_RUNTIME.value: str(fields.get("runtime", "")),
        }
        break
    if mounts is not None:
        facts[Fact.MACHINE_MOUNTS.value] = f"{mounts.work}{WRITABLE_SUFFIX},{mounts.binaries}"
    return facts


def over_cap(size_bytes: int, cap_bytes: int = ATTEMPT_CAP_BYTES) -> bool:
    return size_bytes > cap_bytes


def enough_free(free_bytes: int, minimum_bytes: int = MIN_FREE_BYTES) -> bool:
    return free_bytes >= minimum_bytes


def scratch_paths(
    root: PurePosixPath, clone: PurePosixPath, cache: PurePosixPath, home: PurePosixPath, tmpdirs: Iterable[str]
) -> list[PurePosixPath]:
    """PREREG 13: what an ended attempt deletes; attempt.json, the logs, the suite records and the scans stay."""
    paths = [clone, cache, home, *(PurePosixPath(path) for path in tmpdirs)]
    for path in paths:
        if path == root or not path.is_relative_to(root):
            raise ValueError(f"{path} is not inside the attempt directory {root}")
    return sorted(set(paths))
