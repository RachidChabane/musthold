"""Copies the three Go builders into the product export and compiles them with the product binary they drive."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Final

from calibration.adapters.go_layout import (
    ALIGNMENT_MODULE,
    BUILDER_TARGETS,
    CMD_DIR,
    PRODUCT_ALIGNMENT_BINARY,
    PRODUCT_COMMAND_DIR,
    BuilderFailure,
    BuilderTarget,
)
from calibration.application.build_ports import BuilderBuild
from calibration.domain.canonical import sha256_hex
from calibration.domain.constants import ENCODING, EXIT_OK
from calibration.domain.ports import ProcessRunner

GO_BUILD_TIMEOUT_SECONDS: Final = 1800.0
GO_MAX_PROCS_ENV: Final = "GOMAXPROCS"
GO_MAX_PROCS: Final = "2"
NICE_LEVEL: Final = "10"
GO_BUILD_COMMAND: Final = ("go", "build")


class GoBuilderToolchain:
    """`BuilderToolchain` over the `go` command, run in the module directory under an optional memory guard."""

    def __init__(self, runner: ProcessRunner, guard_prefix: Sequence[str], go_env: Mapping[str, str]) -> None:
        self._runner = runner
        self._guard_prefix = tuple(guard_prefix)
        self._env = {GO_MAX_PROCS_ENV: GO_MAX_PROCS, **go_env}

    def build(self, export_dir: Path, builder_dir: Path, output_dir: Path, procs: int) -> BuilderBuild:
        output_dir.mkdir(parents=True, exist_ok=True)
        source_digests = {
            target.source: self._copy_source(target, builder_dir, export_dir) for target in BUILDER_TARGETS
        }
        for target in BUILDER_TARGETS:
            self._compile(export_dir / target.module, target.command_dir, output_dir / target.binary, procs)
        self._compile(export_dir / ALIGNMENT_MODULE, PRODUCT_COMMAND_DIR, output_dir / PRODUCT_ALIGNMENT_BINARY, procs)
        binary_digests = {
            target.binary: sha256_hex((output_dir / target.binary).read_bytes()) for target in BUILDER_TARGETS
        }
        return BuilderBuild(source_digests=source_digests, binary_digests=binary_digests)

    @staticmethod
    def _copy_source(target: BuilderTarget, builder_dir: Path, export_dir: Path) -> str:
        content = (builder_dir / target.source).read_bytes()
        destination = export_dir / target.destination
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_bytes(content)
        return sha256_hex(content)

    def _compile(self, module_dir: Path, command_dir: str, binary: Path, procs: int) -> None:
        command = [
            *self._guard_prefix,
            "nice",
            "-n",
            NICE_LEVEL,
            *GO_BUILD_COMMAND,
            "-p",
            str(procs),
            "-trimpath",
            "-o",
            str(binary),
            f"./{CMD_DIR}/{command_dir}",
        ]
        result = self._runner.run(command, cwd=module_dir, env=self._env, timeout_seconds=GO_BUILD_TIMEOUT_SECONDS)
        if result.returncode != EXIT_OK:
            output = (result.stderr + result.stdout).decode(ENCODING, "replace")
            raise BuilderFailure(command, f"the go build failed with exit {result.returncode}", output)
