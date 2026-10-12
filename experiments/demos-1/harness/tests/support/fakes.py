"""In-memory stand-ins for the ports: a clock that never sleeps, and an engine that plays scripted containers."""

from __future__ import annotations

import json
from collections.abc import Callable, Sequence
from dataclasses import dataclass, field
from datetime import UTC, datetime, timedelta
from pathlib import Path, PurePosixPath

from demos1.application.ports import HostRun
from demos1.domain.container import ContainerCommand, ContainerState
from demos1.domain.deviations import Deviation

START = datetime(2026, 10, 13, 8, 0, 0, tzinfo=UTC)


class FakeClock:
    def __init__(self, start: datetime = START) -> None:
        self.current = start
        self.ticks = 0.0
        self.sleeps: list[float] = []

    def now(self) -> datetime:
        return self.current

    def monotonic(self) -> float:
        return self.ticks

    def sleep(self, seconds: float) -> None:
        self.sleeps.append(seconds)
        self.ticks += seconds
        self.current += timedelta(seconds=seconds)


class RecordedDeviations:
    def __init__(self) -> None:
        self.entries: list[Deviation] = []

    def append(self, deviation: Deviation) -> None:
        self.entries.append(deviation)


@dataclass
class Played:
    """What a scripted container does: its exit, its OOM flag, and the files it writes."""

    exit_code: int = 0
    oom_killed: bool = False
    timed_out: bool = False
    wall_seconds: float = 1.0
    stdout: str = ""
    stderr: str = ""
    writes: dict[str, str | bytes] = field(default_factory=dict[str, str | bytes])
    host_exit: int | None = None
    client_stderr: str = ""
    started: bool = True
    """False for a refusal: the client exits before any container exists."""
    leave_running: bool = False


type Script = Callable[[ContainerCommand], Played]


def write_measure(prefix: str, played: Played, limit: int | None) -> None:
    Path(prefix + ".measure.json").write_text(
        json.dumps(
            {
                "exit_status": played.exit_code,
                "limit_seconds": limit,
                "memory_peak_bytes": 52_428_800,
                "timed_out": played.timed_out,
                "wall_seconds": played.wall_seconds,
            }
        )
    )


class ScriptedEngine:
    """Plays each `docker run` from a script: it writes what the wrapper would, then keeps the container."""

    def __init__(self, script: Script) -> None:
        self.script = script
        self.containers: dict[str, ContainerState] = {}
        self.running_names: set[str] = set()
        self.calls: list[tuple[str, str]] = []
        self.commands: list[ContainerCommand] = []
        self.engine_up = True

    def available(self) -> bool:
        return self.engine_up

    def host_argv(self, command: ContainerCommand, slots: int) -> list[str]:
        return ["with-slot", str(slots), "--wait", "3600", "--", "mem-guard", "run", "--", *command.docker_argv()]

    def run(self, command: ContainerCommand, slots: int, cap_seconds: float | None, prefix: PurePosixPath) -> HostRun:
        self.calls.append(("run", command.name))
        self.commands.append(command)
        played = self.script(command)
        Path(prefix).parent.mkdir(parents=True, exist_ok=True)
        if played.started:
            for relative, content in played.writes.items():
                target = Path(relative)
                target.parent.mkdir(parents=True, exist_ok=True)
                if isinstance(content, bytes):
                    target.write_bytes(content)
                else:
                    target.write_text(content)
            Path(str(prefix) + ".stdout").write_text(played.stdout)
            Path(str(prefix) + ".stderr").write_text(played.stderr)
            write_measure(str(prefix), played, command.limit_seconds)
            self.containers[command.name] = ContainerState(played.exit_code, played.oom_killed)
            if played.leave_running:
                self.running_names.add(command.name)
        host_exit = played.host_exit if played.host_exit is not None else played.exit_code
        return HostRun(host_exit, played.client_stderr, killed_at_cap=False, started_at=START, ended_at=START)

    def exists(self, name: str) -> bool:
        return name in self.containers

    def running(self, name: str) -> bool:
        return name in self.running_names

    def inspect(self, name: str) -> ContainerState:
        self.calls.append(("inspect", name))
        return self.containers[name]

    def remove(self, name: str) -> None:
        self.calls.append(("rm", name))
        del self.containers[name]
        self.running_names.discard(name)

    def stop(self, name: str) -> None:
        self.calls.append(("stop", name))
        self.running_names.discard(name)

    def names_with_prefix(self, prefix: str) -> Sequence[str]:
        return sorted(name for name in self.containers if name.startswith(prefix))


class FakeMachine:
    def __init__(self) -> None:
        self.events: list[str] = []

    def start(self, mounts: object) -> None:
        self.events.append("start")

    def stop(self) -> None:
        self.events.append("stop")

    def facts(self) -> dict[str, str]:
        return {"colima_version": "0.10.3", "machine_cpus": "2", "machine_memory_bytes": "4294967296"}
