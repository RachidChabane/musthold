"""A `ProcessRunner` over `subprocess`, for the tests that exercise real commands."""

from __future__ import annotations

import os
import subprocess
from collections.abc import Mapping, Sequence
from pathlib import Path

from calibration.domain.ports import ProcessResult


class RealProcessRunner:
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
        environment = {key: value for key, value in os.environ.items() if key not in unset}
        environment.update(env or {})
        completed = subprocess.run(
            list(argv),
            cwd=cwd,
            env=environment,
            input=input_bytes,
            capture_output=True,
            timeout=timeout_seconds,
            check=False,
        )
        return ProcessResult(completed.returncode, completed.stdout, completed.stderr)
