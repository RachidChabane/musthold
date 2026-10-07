"""Runs a command with outbound network denied, so a test that passes here passes offline."""

from __future__ import annotations

import shutil
import sys
import tempfile
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Final

from specbench.application.ports import ProcessResult, ProcessRunner

SANDBOX_EXEC: Final = "sandbox-exec"
UNSHARE: Final = "unshare"
DARWIN_PROFILE: Final = (
    '(version 1)\n(allow default)\n(deny network-outbound)\n(allow network-outbound (remote ip "localhost:*"))\n'
)
"""Outbound traffic is denied except to loopback, which tests use for local servers."""
PROBE_ARGV: Final = ("curl", "-sS", "--max-time", "5", "-o", "/dev/null", "https://1.1.1.1")
PROBE_TIMEOUT_EXIT_CODES: Final = frozenset({7, 28})


class NetworkDenialUnavailable(RuntimeError):
    """The machine offers no way to deny the network, or the denial does not hold."""


def _wrapper(profile_path: Path) -> list[str]:
    if sys.platform == "darwin" and shutil.which(SANDBOX_EXEC):
        return [SANDBOX_EXEC, "-f", str(profile_path)]
    if shutil.which(UNSHARE):
        return [UNSHARE, "--net", "--map-root-user"]
    raise NetworkDenialUnavailable(f"neither {SANDBOX_EXEC} nor {UNSHARE} is available")


class NetworkDeniedRunner:
    """Decorator: every command runs inside a sandbox that refuses outbound network connections."""

    def __init__(self, inner: ProcessRunner) -> None:
        self._inner = inner
        self._profile = Path(tempfile.mkdtemp(prefix="specbench-sandbox-")) / "deny-network.sb"
        self._profile.write_text(DARWIN_PROFILE, encoding="utf-8")
        self._prefix = _wrapper(self._profile)

    def run(self, argv: Sequence[str], cwd: Path | None = None, env: Mapping[str, str] | None = None) -> ProcessResult:
        return self._inner.run([*self._prefix, *argv], cwd, env)

    def assert_denied(self) -> None:
        """Fail unless a scripted request to a public address is refused inside the sandbox."""
        result = self.run(PROBE_ARGV)
        if result.returncode == 0 or result.returncode not in PROBE_TIMEOUT_EXIT_CODES | {6}:
            raise NetworkDenialUnavailable(
                f"the sandbox let a request through or could not run curl (exit {result.returncode}): "
                f"{result.stderr.strip()}"
            )
