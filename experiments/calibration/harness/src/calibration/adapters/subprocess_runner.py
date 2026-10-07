"""`ProcessRunner` over `subprocess`: a command runs in its own session so that a timeout ends it with its children."""

from __future__ import annotations

import contextlib
import os
import signal
import subprocess
from collections.abc import Mapping, Sequence
from pathlib import Path

from calibration.domain.ports import ProcessResult, ProcessTimeout


def _environment(overlay: Mapping[str, str] | None, unset: Sequence[str]) -> dict[str, str]:
    environment = dict(os.environ)
    if overlay is not None:
        environment.update(overlay)
    for name in unset:
        environment.pop(name, None)
    return environment


def _kill_group(process: subprocess.Popen[bytes]) -> None:
    with contextlib.suppress(ProcessLookupError):
        os.killpg(process.pid, signal.SIGKILL)


class SubprocessRunner:
    def run(
        self,
        argv: Sequence[str],
        *,
        cwd: Path | None = None,
        env: Mapping[str, str] | None = None,
        unset: Sequence[str] = (),
        input_bytes: bytes | None = None,
        timeout_seconds: float | None = None,
    ) -> ProcessResult:
        with subprocess.Popen(
            list(argv),
            cwd=cwd,
            env=_environment(env, unset),
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            start_new_session=True,
        ) as process:
            try:
                stdout, stderr = process.communicate(input_bytes or b"", timeout=timeout_seconds)
            except subprocess.TimeoutExpired as expired:
                _kill_group(process)
                process.communicate()
                raise ProcessTimeout(f"{list(argv)} exceeded {timeout_seconds} s") from expired
            except BaseException:
                _kill_group(process)
                raise
            return ProcessResult(process.returncode, stdout, stderr)
