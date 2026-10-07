from __future__ import annotations

import pytest

from calibration.adapters.provenance import ProvenanceUnavailable, SystemProvenanceProbe
from calibration.domain.ports import ProcessResult
from support.fakes import ScriptedRunner, ok

UV = ("uv", "--version")


def test_macos_reading() -> None:
    runner = ScriptedRunner({("sw_vers", "-buildVersion"): ok("25G5042c\n"), UV: ok("uv 0.9.1\n")})
    probe = SystemProvenanceProbe(runner, system=lambda: "Darwin", python_version=lambda: "3.13.14")
    assert dict(probe.reading().values) == {"os_build": "25G5042c", "python": "3.13.14", "uv": "uv 0.9.1"}


def test_linux_reading_uses_the_kernel_release() -> None:
    runner = ScriptedRunner({("uname", "-r"): ok("6.8.0-generic\n"), UV: ok("uv 0.9.1\n")})
    probe = SystemProvenanceProbe(runner, system=lambda: "Linux", python_version=lambda: "3.13.14")
    assert probe.reading().values["os_build"] == "6.8.0-generic"


def test_a_failing_command_is_unavailable() -> None:
    runner = ScriptedRunner({("sw_vers", "-buildVersion"): ProcessResult(1, b"", b"")})
    with pytest.raises(ProvenanceUnavailable):
        SystemProvenanceProbe(runner, system=lambda: "Darwin").reading()
