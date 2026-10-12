"""The abstractions the use cases depend on; the adapters implement them at the edge."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from datetime import datetime
from pathlib import PurePosixPath
from typing import Protocol

from demos1.domain.api import ApiResponse
from demos1.domain.container import ContainerCommand, ContainerState, Measure
from demos1.domain.deviations import Deviation
from demos1.domain.jsonvalue import JsonValue
from demos1.domain.machine import MachineMounts
from demos1.domain.queries import ApiRequest


class GitHubApi(Protocol):
    """Read-only GitHub API: one GET request, one response, whatever its status."""

    def get(self, request: ApiRequest) -> ApiResponse: ...


class Clock(Protocol):
    def now(self) -> datetime: ...

    def monotonic(self) -> float: ...

    def sleep(self, seconds: float) -> None: ...


@dataclass(frozen=True)
class HostRun:
    """How the host process that ran one `docker run` client ended."""

    exit_status: int
    client_stderr: str
    killed_at_cap: bool
    started_at: datetime
    ended_at: datetime


class ContainerEngine(Protocol):
    """The Docker engine, reached only through the `docker` client on the host."""

    def available(self) -> bool:
        """`docker info` succeeds."""
        ...

    def host_argv(self, command: ContainerCommand, slots: int) -> list[str]:
        """The full host command line: `with-slot`, then `mem-guard run`, then `docker run`."""
        ...

    def run(self, command: ContainerCommand, slots: int, cap_seconds: float | None, prefix: PurePosixPath) -> HostRun:
        """Run the command's container, stopping it with `docker stop -t 10` at the cap if one is set.

        The client's stdout and stderr go next to the wrapper's files under the prefix. When the verb itself is
        interrupted, the container is stopped and OperatorStopped carries how the client ended.
        """
        ...

    def exists(self, name: str) -> bool: ...

    def running(self, name: str) -> bool: ...

    def inspect(self, name: str) -> ContainerState: ...

    def remove(self, name: str) -> None: ...

    def stop(self, name: str) -> None:
        """`docker stop -t 10 <name>`."""
        ...

    def names_with_prefix(self, prefix: str) -> Sequence[str]:
        """`docker ps -a --filter name=<prefix>`: every container whose name starts with the prefix."""
        ...


class OperatorStopped(Exception):
    """The verb was interrupted while a container ran; the container was stopped before this was raised."""

    def __init__(self, host: HostRun) -> None:
        super().__init__("the verb was interrupted while a container ran")
        self.host = host


class Files(Protocol):
    """Plain file reads, writes and moves on the host, under the work directory."""

    def make_fresh(self, directories: Sequence[PurePosixPath]) -> None:
        """Create each directory new and empty."""
        ...

    def ensure_directory(self, directory: PurePosixPath) -> None: ...

    def exists(self, path: PurePosixPath) -> bool: ...

    def is_directory(self, path: PurePosixPath) -> bool: ...

    def read_text(self, path: PurePosixPath) -> str | None: ...

    def write_text(self, path: PurePosixPath, content: str) -> None: ...

    def append_text(self, path: PurePosixPath, content: str) -> None: ...

    def list_names(self, directory: PurePosixPath) -> list[str]: ...

    def move(self, source: PurePosixPath, target: PurePosixPath) -> None:
        """Move a tree as plain files, never through git."""
        ...

    def sha256(self, path: PurePosixPath) -> str | None: ...

    def read_measure(self, prefix: PurePosixPath) -> Measure | None:
        """The wrapper's record next to the command's log, or None when the wrapper never wrote it."""
        ...

    def read_json(self, path: PurePosixPath) -> JsonValue | None: ...

    def write_json(self, path: PurePosixPath, document: JsonValue) -> None: ...

    def tree_size(self, path: PurePosixPath) -> int:
        """The bytes the files under a directory occupy."""
        ...

    def free_bytes(self, path: PurePosixPath) -> int:
        """The free bytes of the volume that holds the path."""
        ...

    def remove_tree(self, path: PurePosixPath) -> None:
        """Delete a file or a directory tree, read-only entries included; nothing when it is absent."""
        ...


class Machine(Protocol):
    """The colima machine of the engine: started for a verb's containers, stopped when the verb ends."""

    def start(self, mounts: MachineMounts) -> None: ...

    def stop(self) -> None: ...

    def facts(self) -> Mapping[str, str]:
        """The colima version and the machine's settings."""
        ...


class DeviationLog(Protocol):
    def append(self, deviation: Deviation) -> None: ...


class Repository(Protocol):
    """The spec-proof repository on the host: tags, commits and the visibility of its origin."""

    def commit_of(self, ref: str) -> str | None: ...

    def is_ancestor(self, ancestor: str, descendant: str) -> bool: ...

    def commits_touching(self, since: str, until: str, paths: Sequence[str]) -> Sequence[str]: ...

    def head(self) -> str: ...

    def origin_visibility(self) -> str: ...


class Probe(Protocol):
    """Facts about the tools and the engine the manifest and every attempt record."""

    def facts(self) -> Mapping[str, str]: ...
