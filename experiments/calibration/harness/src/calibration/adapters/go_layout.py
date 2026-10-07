"""Where each Go builder lives in the product export, what its binary is called and how a binary is driven."""

from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Final, cast

from calibration.domain.constants import ENCODING, EXIT_OK
from calibration.domain.ports import ProcessRunner

BUILDER_COMMAND_DIR: Final = "calibration-build"
DECODE_COMMAND_DIR: Final = "calibration-decode"
PRODUCT_COMMAND_DIR: Final = "spec-proof-alignment"
CMD_DIR: Final = "cmd"
BUILDER_SOURCE_NAME: Final = "main.go"
ALIGNMENT_MODULE: Final = "alignment"
DISCPOWER_MODULE: Final = "discpower"
S1RELAY_MODULE: Final = "s1relay"
ALIGNMENT_BUILDER_BINARY: Final = "calibration-build-alignment"
DISCPOWER_BUILDER_BINARY: Final = "calibration-build-discpower"
DECODE_BINARY: Final = "calibration-decode"
PRODUCT_ALIGNMENT_BINARY: Final = PRODUCT_COMMAND_DIR
LIST_SEPARATOR: Final = ","
DECIDE_CLOSED_KIND: Final = "decide.closed"
BINARY_TIMEOUT_SECONDS: Final = 600.0


class BuilderFailure(Exception):
    """A compiled builder or the product binary that exited non-zero or printed something that is not its JSON."""

    def __init__(self, command: Sequence[str], reason: str, stderr: str = "") -> None:
        super().__init__(f"{' '.join(command)}: {reason}" + (f"\n{stderr}" if stderr else ""))
        self.command = tuple(command)
        self.reason = reason
        self.stderr = stderr


@dataclass(frozen=True)
class BuilderTarget:
    """One builder: the source it is built from, the module it is copied into and the binary it becomes."""

    source: str
    module: str
    command_dir: str
    binary: str

    @property
    def destination(self) -> Path:
        """The path of the copied source relative to the export root."""
        return Path(self.module) / CMD_DIR / self.command_dir / BUILDER_SOURCE_NAME


BUILDER_TARGETS: Final = (
    BuilderTarget(
        f"{ALIGNMENT_MODULE}/{BUILDER_SOURCE_NAME}", ALIGNMENT_MODULE, BUILDER_COMMAND_DIR, ALIGNMENT_BUILDER_BINARY
    ),
    BuilderTarget(
        f"{DISCPOWER_MODULE}/{BUILDER_SOURCE_NAME}", DISCPOWER_MODULE, BUILDER_COMMAND_DIR, DISCPOWER_BUILDER_BINARY
    ),
    BuilderTarget(f"{S1RELAY_MODULE}/{BUILDER_SOURCE_NAME}", S1RELAY_MODULE, DECODE_COMMAND_DIR, DECODE_BINARY),
)


def parse_json_output(command: Sequence[str], stdout: bytes) -> object:
    try:
        return cast(object, json.loads(stdout.decode(ENCODING)))
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise BuilderFailure(command, f"the output is not JSON: {error}") from error


class BinaryInvoker:
    """Runs one compiled binary through the process runner: text or JSON in on standard input, JSON out."""

    def __init__(self, runner: ProcessRunner, binary: Path, env: Mapping[str, str] | None = None) -> None:
        self._runner = runner
        self._binary = binary
        self._env = dict(env) if env else None

    def call(self, arguments: Sequence[str], stdin: str | None = None, *, cwd: Path | None = None) -> object:
        command = [str(self._binary), *arguments]
        result = self._runner.run(
            command,
            cwd=cwd,
            env=self._env,
            input_bytes=stdin.encode(ENCODING) if stdin is not None else None,
            timeout_seconds=BINARY_TIMEOUT_SECONDS,
        )
        if result.returncode != EXIT_OK:
            raise BuilderFailure(command, f"exit {result.returncode}", result.stderr.decode(ENCODING, "replace"))
        return parse_json_output(command, result.stdout)
