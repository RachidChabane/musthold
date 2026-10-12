"""One container command of PREREG 13: its `docker run` options, its environment and its in-image wrapper."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Final

from demos1.domain.jsonvalue import JsonObject

CPUS: Final = "2"
MEMORY: Final = "3g"
BINARIES_TARGET: Final = "/opt/spec-proof/bin"
WRAPPER: Final = "/usr/local/bin/demos1-run"
ENV_PROGRAM: Final = "/usr/bin/env"
OFFLINE_NETWORK: Final = "none"
CONTAINER_PATH: Final = (
    f"{BINARIES_TARGET}:/usr/local/go/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/usr/bin:/sbin:/bin"
)
"""The product binaries, the image's Go and uv, and the system directories."""
WORKERS: Final = 2
GO_FLAGS: Final = f"-p={WORKERS}"
MOUNT_FORBIDDEN: Final = (",", "=")
"""Characters a `--mount` source or target cannot hold."""


@dataclass(frozen=True)
class ContainerCommand:
    """Everything that makes one `docker run`: never with `--rm`, with the limits and mounts of PREREG 13."""

    name: str
    image: str
    attempt_dir: str
    binaries_dir: str
    workdir: str
    network: bool
    environment: Mapping[str, str]
    argv: Sequence[str]
    record_prefix: str
    limit_seconds: int | None
    uid: int
    gid: int
    memory: str = MEMORY
    cpus: str = CPUS

    def __post_init__(self) -> None:
        for path in (self.attempt_dir, self.binaries_dir):
            if any(character in path for character in MOUNT_FORBIDDEN):
                raise ValueError(f"a mounted path cannot hold ',' or '=': {path!r}")

    def docker_argv(self) -> list[str]:
        argv = [
            "docker",
            "run",
            "--name",
            self.name,
            "--user",
            f"{self.uid}:{self.gid}",
            "--cap-drop",
            "ALL",
            "--security-opt",
            "no-new-privileges",
            "--cpus",
            self.cpus,
            "--memory",
            self.memory,
            "--memory-swap",
            self.memory,
        ]
        if not self.network:
            argv += ["--network", OFFLINE_NETWORK]
        argv += [
            "--mount",
            f"type=bind,source={self.attempt_dir},target={self.attempt_dir}",
            "--mount",
            f"type=bind,source={self.binaries_dir},target={BINARIES_TARGET},readonly",
            "--workdir",
            self.workdir,
            self.image,
            *self.wrapped_argv(),
        ]
        return argv

    def wrapped_argv(self) -> list[str]:
        """What the container runs: the wrapper under `env -i`, with only the listed variables."""
        argv = [ENV_PROGRAM, "-i", *(f"{name}={value}" for name, value in self.environment.items())]
        argv += [WRAPPER, "--record", self.record_prefix]
        if self.limit_seconds is not None:
            argv += ["--limit", str(self.limit_seconds)]
        return [*argv, "--", *self.argv]

    def describe(self) -> JsonObject:
        return {
            "name": self.name,
            "image": self.image,
            "network": self.network,
            "environment": dict(self.environment),
            "command": list(self.argv),
            "limit_seconds": self.limit_seconds,
            "docker_argv": self.docker_argv(),
        }


@dataclass(frozen=True)
class ContainerState:
    """`docker inspect -f '{{.State.ExitCode}} {{.State.OOMKilled}}' <name>`."""

    exit_code: int
    oom_killed: bool

    @classmethod
    def parse(cls, line: str) -> ContainerState:
        code, killed = line.split()
        return cls(int(code), killed.lower() == "true")


@dataclass(frozen=True)
class Measure:
    """What the wrapper writes next to the command's log: wall time, peak memory, exit status."""

    wall_seconds: float
    memory_peak_bytes: int | None
    exit_status: int
    timed_out: bool
    limit_seconds: int | None


@dataclass(frozen=True)
class XdgDirectories:
    cache_home: str
    state_home: str


@dataclass(frozen=True)
class CommandEnvironment:
    """PREREG 13: the variables every command runs with, and the ones its kind adds."""

    home: str
    tmpdir: str
    go_path: str
    go_mod_cache: str
    go_cache: str
    uv_cache: str
    uv_python: str

    def variables(
        self, *, offline: bool, clone_protocol: str | None = None, xdg: XdgDirectories | None = None
    ) -> dict[str, str]:
        variables = {
            "PATH": CONTAINER_PATH,
            "HOME": self.home,
            "TMPDIR": self.tmpdir,
            "GOPATH": self.go_path,
            "GOMODCACHE": self.go_mod_cache,
            "GOCACHE": self.go_cache,
            "GOTOOLCHAIN": "local",
            "GOFLAGS": GO_FLAGS,
            "UV_CACHE_DIR": self.uv_cache,
            "UV_PYTHON_INSTALL_DIR": self.uv_python,
            "LANG": "C.UTF-8",
            "TZ": "UTC",
        }
        if xdg is not None:
            variables["XDG_CACHE_HOME"] = xdg.cache_home
            variables["XDG_STATE_HOME"] = xdg.state_home
        if clone_protocol is not None:
            variables["GIT_ALLOW_PROTOCOL"] = clone_protocol
        if offline:
            variables["GOPROXY"] = "off"
            variables["UV_OFFLINE"] = "1"
        return variables
