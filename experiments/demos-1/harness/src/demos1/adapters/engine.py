"""The `docker` client against the colima profile's socket, and the colima machine itself."""

from __future__ import annotations

import json
import os
import signal
import subprocess
from collections.abc import Callable, Mapping, Sequence
from datetime import datetime
from pathlib import Path, PurePosixPath
from typing import Final

from demos1.application.ports import HostRun, OperatorStopped
from demos1.domain.container import ContainerCommand, ContainerState
from demos1.domain.environment import Fact
from demos1.domain.machine import (
    MachineMounts,
    list_argv,
    machine_settings,
    start_argv,
    stop_argv,
)

SLOT_WAIT_SECONDS: Final = 3600
STOP_GRACE_SECONDS: Final = 10
CLIENT_GRACE_SECONDS: Final = 30
SHORT_TIMEOUT_SECONDS: Final = 120
MACHINE_TIMEOUT_SECONDS: Final = 900
CLIENT_STDOUT: Final = ".client.out"
CLIENT_STDERR: Final = ".client.err"
DOCKER_HOST: Final = "DOCKER_HOST"


class DockerEngine:
    """Every docker command names the profile's socket through DOCKER_HOST, and nothing else of the host's config."""

    def __init__(self, socket: str, script_dir: PurePosixPath, now: Callable[[], datetime]) -> None:
        self._environment = {**os.environ, DOCKER_HOST: socket}
        self._script_dir = script_dir
        self._now = now

    def _docker(self, *arguments: str, timeout: float = SHORT_TIMEOUT_SECONDS) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["docker", *arguments],
            capture_output=True,
            text=True,
            env=self._environment,
            timeout=timeout,
            check=False,
        )

    def available(self) -> bool:
        try:
            return self._docker("info", "--format", "{{.ServerVersion}}").returncode == 0
        except subprocess.TimeoutExpired:
            return False

    def host_argv(self, command: ContainerCommand, slots: int) -> list[str]:
        """PREREG 9.1: `with-slot`, then `mem-guard run` (which applies nice), then the `docker run` client."""
        return [
            str(self._script_dir / "with-slot"),
            str(slots),
            "--wait",
            str(SLOT_WAIT_SECONDS),
            "--",
            str(self._script_dir / "mem-guard"),
            "run",
            "--",
            *command.docker_argv(),
        ]

    def run(self, command: ContainerCommand, slots: int, cap_seconds: float | None, prefix: PurePosixPath) -> HostRun:
        Path(prefix).parent.mkdir(parents=True, exist_ok=True)
        started = self._now()
        stderr_path = Path(str(prefix) + CLIENT_STDERR)
        with Path(str(prefix) + CLIENT_STDOUT).open("w") as stdout, stderr_path.open("w") as stderr:
            process = subprocess.Popen(
                self.host_argv(command, slots),
                stdout=stdout,
                stderr=stderr,
                env=self._environment,
                start_new_session=True,
            )
            killed_at_cap = False
            try:
                process.wait(timeout=cap_seconds)
            except subprocess.TimeoutExpired:
                killed_at_cap = True
                self._end_client(process, command.name)
            except KeyboardInterrupt:
                self._end_client(process, command.name)
                raise OperatorStopped(self._host_run(process, stderr_path, False, started)) from None
        return self._host_run(process, stderr_path, killed_at_cap, started)

    def _host_run(self, process: subprocess.Popen[bytes], stderr: Path, killed: bool, started: datetime) -> HostRun:
        return HostRun(
            exit_status=process.returncode if process.returncode is not None else -1,
            client_stderr=stderr.read_text(errors="replace") if stderr.exists() else "",
            killed_at_cap=killed,
            started_at=started,
            ended_at=self._now(),
        )

    def _end_client(self, process: subprocess.Popen[bytes], name: str) -> None:
        """Stop the container, then wait for the client; a client still waiting for a slot gets SIGTERM."""
        if self.exists(name):
            self.stop(name)
        try:
            process.wait(timeout=CLIENT_GRACE_SECONDS)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGTERM)
            process.wait()

    def exists(self, name: str) -> bool:
        return self._docker("container", "inspect", "--format", "{{.Name}}", name).returncode == 0

    def running(self, name: str) -> bool:
        result = self._docker("container", "inspect", "--format", "{{.State.Running}}", name)
        return result.returncode == 0 and result.stdout.strip() == "true"

    def inspect(self, name: str) -> ContainerState:
        result = self._docker("inspect", "-f", "{{.State.ExitCode}} {{.State.OOMKilled}}", name)
        if result.returncode != 0:
            raise RuntimeError(f"docker inspect {name}: {result.stderr.strip()}")
        return ContainerState.parse(result.stdout.strip())

    def remove(self, name: str) -> None:
        result = self._docker("rm", name)
        if result.returncode != 0:
            raise RuntimeError(f"docker rm {name}: {result.stderr.strip()}")

    def stop(self, name: str) -> None:
        self._docker("stop", "-t", str(STOP_GRACE_SECONDS), name, timeout=SHORT_TIMEOUT_SECONDS + STOP_GRACE_SECONDS)

    def names_with_prefix(self, prefix: str) -> Sequence[str]:
        result = self._docker("ps", "-a", "--filter", f"name={prefix}", "--format", "{{.Names}}")
        return sorted(name for name in result.stdout.split() if name.startswith(prefix))

    def engine_facts(self) -> Mapping[str, str]:
        result = self._docker("info", "--format", "{{.ServerVersion}} {{.MemTotal}} {{.NCPU}}")
        words = result.stdout.split()
        if result.returncode != 0 or len(words) != 3:
            return {}
        return {
            Fact.ENGINE_VERSION.value: words[0],
            Fact.ENGINE_MEMORY_BYTES.value: words[1],
            Fact.ENGINE_CPUS.value: words[2],
        }


class ColimaMachine:
    """The `spec-proof-demos` profile: started with its two mounts, stopped whatever the verb's exit."""

    def __init__(self) -> None:
        self._mounts: MachineMounts | None = None

    def start(self, mounts: MachineMounts) -> None:
        Path(mounts.work).mkdir(parents=True, exist_ok=True)
        Path(mounts.binaries).mkdir(parents=True, exist_ok=True)
        self._mounts = mounts
        result = subprocess.run(
            start_argv(mounts), capture_output=True, text=True, timeout=MACHINE_TIMEOUT_SECONDS, check=False
        )
        if result.returncode != 0:
            raise RuntimeError(f"colima start failed: {result.stderr.strip()[-2000:]}")

    def stop(self) -> None:
        subprocess.run(stop_argv(), capture_output=True, text=True, timeout=MACHINE_TIMEOUT_SECONDS, check=False)

    def facts(self) -> Mapping[str, str]:
        version = subprocess.run(["colima", "version"], capture_output=True, text=True, check=False).stdout
        listing = subprocess.run(list_argv(), capture_output=True, text=True, check=False).stdout
        return machine_settings(version, listing, self._mounts)

    def status(self) -> str:
        """`Running`, `Stopped`, or empty when the profile does not exist."""
        listing = subprocess.run(list_argv(), capture_output=True, text=True, check=False).stdout
        for line in listing.splitlines():
            if '"status"' in line:
                return str(json.loads(line).get("status", ""))
        return ""
