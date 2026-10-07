from __future__ import annotations

from pathlib import Path

import pytest

from calibration.adapters.memory_probe import (
    MemoryProbeError,
    SystemMemoryProbe,
    parse_free_percentage,
    parse_meminfo,
)
from calibration.application.ports import MemoryReading
from calibration.domain.ports import ProcessResult
from support.fakes import ScriptedRunner, ok

MEMORY_PRESSURE_OUTPUT = """The system has 25769803776 (1572864 pages with a page size of 16384).

Stats:
Pages free: 12345
Pages purgeable: 4567
Pageins: 7890123

System-wide memory free percentage: 62%
"""

MEMINFO = """MemTotal:       16384000 kB
MemFree:         1000000 kB
MemAvailable:    8192000 kB
SwapTotal:             0 kB
"""

MEMSIZE = ("sysctl", "-n", "hw.memsize")
PRESSURE = ("memory_pressure",)
LEVEL = ("sysctl", "-n", "kern.memorystatus_vm_pressure_level")


def darwin_probe(level: str, output: str = MEMORY_PRESSURE_OUTPUT) -> SystemMemoryProbe:
    runner = ScriptedRunner({MEMSIZE: ok("25769803776\n"), PRESSURE: ok(output), LEVEL: ok(level + "\n")})
    return SystemMemoryProbe(runner, system=lambda: "Darwin")


def test_total_memory_comes_from_sysctl() -> None:
    assert darwin_probe("1").total_memory_bytes() == 25769803776


def test_free_percentage_and_normal_pressure_on_macos() -> None:
    assert darwin_probe("1").reading() == MemoryReading(62, True)


@pytest.mark.parametrize("level", ["2", "4"])
def test_a_pressure_level_other_than_one_is_not_normal(level: str) -> None:
    assert darwin_probe(level).reading() == MemoryReading(62, False)


def test_a_memory_pressure_without_the_percentage_is_an_error() -> None:
    with pytest.raises(MemoryProbeError):
        darwin_probe("1", output="nothing useful").reading()


def test_a_failing_command_is_an_error() -> None:
    runner = ScriptedRunner({MEMSIZE: ProcessResult(1, b"", b"boom")})
    with pytest.raises(MemoryProbeError):
        SystemMemoryProbe(runner, system=lambda: "Darwin").total_memory_bytes()


def test_a_non_numeric_sysctl_is_an_error() -> None:
    runner = ScriptedRunner({MEMSIZE: ok("lots\n")})
    with pytest.raises(MemoryProbeError):
        SystemMemoryProbe(runner, system=lambda: "Darwin").total_memory_bytes()


def test_linux_reads_proc_meminfo(tmp_path: Path) -> None:
    meminfo = tmp_path / "meminfo"
    meminfo.write_text(MEMINFO)
    probe = SystemMemoryProbe(ScriptedRunner({}), system=lambda: "Linux", meminfo_path=meminfo)
    assert probe.total_memory_bytes() == 16384000 * 1024
    assert probe.reading() == MemoryReading(50, True)


def test_meminfo_without_a_total_is_an_error(tmp_path: Path) -> None:
    meminfo = tmp_path / "meminfo"
    meminfo.write_text("MemFree: 1 kB\n")
    with pytest.raises(MemoryProbeError):
        SystemMemoryProbe(ScriptedRunner({}), system=lambda: "Linux", meminfo_path=meminfo).reading()


def test_parsers() -> None:
    assert parse_free_percentage("System-wide memory free percentage: 7%") == 7
    assert parse_meminfo(MEMINFO)["MemAvailable"] == 8192000 * 1024
