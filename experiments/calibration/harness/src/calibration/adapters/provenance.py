"""`ProvenanceProbe`: the operating system build, the Python and the uv the run executes with (§14.2)."""

from __future__ import annotations

import platform
from collections.abc import Callable
from typing import Final

from calibration.application.ports import ProvenanceReading
from calibration.domain.ports import ProcessRunner

OS_BUILD_KEY: Final = "os_build"
PYTHON_KEY: Final = "python"
UV_KEY: Final = "uv"
DARWIN: Final = "Darwin"
MACOS_BUILD_ARGV: Final = ("sw_vers", "-buildVersion")
KERNEL_RELEASE_ARGV: Final = ("uname", "-r")
UV_VERSION_ARGV: Final = ("uv", "--version")
COMMAND_TIMEOUT_SECONDS: Final = 30.0


class ProvenanceUnavailable(RuntimeError):
    """A provenance value whose command failed."""


class SystemProvenanceProbe:
    def __init__(
        self,
        runner: ProcessRunner,
        *,
        system: Callable[[], str] = platform.system,
        python_version: Callable[[], str] = platform.python_version,
    ) -> None:
        self._runner = runner
        self._system = system
        self._python_version = python_version

    def reading(self) -> ProvenanceReading:
        build_argv = MACOS_BUILD_ARGV if self._system() == DARWIN else KERNEL_RELEASE_ARGV
        return ProvenanceReading(
            {
                OS_BUILD_KEY: self._output(build_argv),
                PYTHON_KEY: self._python_version(),
                UV_KEY: self._output(UV_VERSION_ARGV),
            }
        )

    def _output(self, argv: tuple[str, ...]) -> str:
        result = self._runner.run(argv, timeout_seconds=COMMAND_TIMEOUT_SECONDS)
        if result.returncode != 0:
            raise ProvenanceUnavailable(f"{' '.join(argv)} exited with {result.returncode}")
        return result.stdout.decode("utf-8", errors="replace").strip()
