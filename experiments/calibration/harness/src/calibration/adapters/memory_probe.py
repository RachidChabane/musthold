"""`MemoryProbe` with the readings of `script/mem-guard`: `memory_pressure` and `sysctl` on macOS, `/proc/meminfo` elsewhere."""

from __future__ import annotations

import platform
import re
from collections.abc import Callable
from pathlib import Path
from typing import Final

from calibration.application.ports import MemoryReading
from calibration.domain.ports import ProcessRunner

DARWIN: Final = "Darwin"
PRESSURE_NORMAL: Final = 1
COMMAND_TIMEOUT_SECONDS: Final = 30.0
MEMSIZE_ARGV: Final = ("sysctl", "-n", "hw.memsize")
MEMORY_PRESSURE_ARGV: Final = ("memory_pressure",)
PRESSURE_LEVEL_ARGV: Final = ("sysctl", "-n", "kern.memorystatus_vm_pressure_level")
DEFAULT_MEMINFO: Final = Path("/proc/meminfo")
PERCENT: Final = 100
BYTES_PER_KIB: Final = 1024
_FREE_PERCENTAGE: Final = re.compile(r"System-wide memory free percentage:\s*(\d+)\s*%")
_MEMINFO_LINE: Final = re.compile(r"^(\w+):\s+(\d+)\s*kB\s*$", re.MULTILINE)
_MEM_TOTAL: Final = "MemTotal"
_MEM_AVAILABLE: Final = "MemAvailable"


class MemoryProbeError(RuntimeError):
    """A memory figure that cannot be read."""


def parse_free_percentage(output: str) -> int:
    found = _FREE_PERCENTAGE.search(output)
    if found is None:
        raise MemoryProbeError("memory_pressure reports no free percentage")
    return int(found.group(1))


def parse_meminfo(text: str) -> dict[str, int]:
    """The `kB` figures of `/proc/meminfo`, in bytes."""
    return {name: int(value) * BYTES_PER_KIB for name, value in _MEMINFO_LINE.findall(text)}


class SystemMemoryProbe:
    def __init__(
        self,
        runner: ProcessRunner,
        *,
        system: Callable[[], str] = platform.system,
        meminfo_path: Path = DEFAULT_MEMINFO,
    ) -> None:
        self._runner = runner
        self._system = system
        self._meminfo_path = meminfo_path

    def total_memory_bytes(self) -> int:
        if self._is_darwin():
            return self._integer(MEMSIZE_ARGV)
        return self._meminfo_figure(_MEM_TOTAL)

    def reading(self) -> MemoryReading:
        if self._is_darwin():
            free = parse_free_percentage(self._output(MEMORY_PRESSURE_ARGV))
            return MemoryReading(free, self._integer(PRESSURE_LEVEL_ARGV) == PRESSURE_NORMAL)
        total = self._meminfo_figure(_MEM_TOTAL)
        return MemoryReading(PERCENT * self._meminfo_figure(_MEM_AVAILABLE) // total, True)

    def _is_darwin(self) -> bool:
        return self._system() == DARWIN

    def _output(self, argv: tuple[str, ...]) -> str:
        result = self._runner.run(argv, timeout_seconds=COMMAND_TIMEOUT_SECONDS)
        if result.returncode != 0:
            raise MemoryProbeError(f"{' '.join(argv)} exited with {result.returncode}")
        return result.stdout.decode("utf-8", errors="replace")

    def _integer(self, argv: tuple[str, ...]) -> int:
        output = self._output(argv).strip()
        try:
            return int(output)
        except ValueError as error:
            raise MemoryProbeError(f"{' '.join(argv)} printed {output!r}, not an integer") from error

    def _meminfo_figure(self, name: str) -> int:
        figures = parse_meminfo(self._meminfo_path.read_text(encoding="utf-8"))
        if figures.get(name, 0) <= 0:
            raise MemoryProbeError(f"{self._meminfo_path} reports no {name}")
        return figures[name]
