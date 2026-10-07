"""Composition root of the run-side verbs: the adapters behind each use case's ports."""

from __future__ import annotations

import fcntl
import os
import sys
from collections.abc import Generator, Mapping
from contextlib import contextmanager
from pathlib import Path
from typing import Final

from calibration.adapters.alignment_driver import BinaryAlignmentDriver
from calibration.adapters.file_run_store import FileRunStore
from calibration.adapters.fixture_workspace import PytestFixtureWorkspace
from calibration.adapters.git_vcs import GitVersionControl
from calibration.adapters.go_builders import BinaryBuilders, BinaryProductReader
from calibration.adapters.go_layout import ALIGNMENT_BUILDER_BINARY, DECODE_BINARY, DISCPOWER_BUILDER_BINARY
from calibration.adapters.go_toolchain import GoBuilderToolchain
from calibration.adapters.heavy_work import SystemHeavyWorkProbe
from calibration.adapters.http_transport import UrllibTransport
from calibration.adapters.hub_catalog import HttpModelHub
from calibration.adapters.kev_environment import SubprocessKevEnvironment
from calibration.adapters.memory_probe import SystemMemoryProbe
from calibration.adapters.ollama_provisioner import ServerOllamaProvisioner
from calibration.adapters.ollama_registry import HttpOllamaRegistry
from calibration.adapters.openssl_timestamp import OpensslTimestampReader
from calibration.adapters.pilot_git import GitPilotHistory
from calibration.adapters.pilot_run_reader import AllowlistedPilotRunReader
from calibration.adapters.pilot_snapshot import PilotSnapshotDomain, UnpackedSnapshot
from calibration.adapters.product_export import GitProductExporter
from calibration.adapters.provenance import SystemProvenanceProbe
from calibration.adapters.server_factory import ExperimentServerFactory
from calibration.adapters.shallnot_build import PinnedShallnotProvider
from calibration.adapters.subprocess_runner import SubprocessRunner
from calibration.adapters.system_time import SystemClock, SystemSleeper
from calibration.adapters.tree_digest import FileTreeDigester, file_digest
from calibration.application.block import BlockDeps
from calibration.application.build_items import BuildItemsDeps
from calibration.application.build_ports import BuiltTools
from calibration.application.manifest import RunManifest
from calibration.application.snapshot import SnapshotUseCase
from calibration.cli.layout import Layout
from calibration.domain.constants import (
    DEFAULT_RUN_WINDOW,
    ENCODING,
    KEV_COMMIT,
    KEV_REPO_URL,
    MEM_GUARD_ENV,
    RUN_WINDOW_ENV,
)
from calibration.domain.ports import ProcessRunner
from calibration.domain.provenance_rules import builder_rollup
from calibration.domain.window import RunWindow

BUILDER_BINARIES: Final = (ALIGNMENT_BUILDER_BINARY, DISCPOWER_BUILDER_BINARY, DECODE_BINARY)
BUILDERS_DIR_PREFIX: Final = "builders-"
HARNESS_COMMIT_KEY: Final = "harness_commit"
BUILDER_DIGEST_KEY: Final = "builder_digest"
EMPTY_PREFIX: Final[tuple[str, ...]] = ()


class BinaryToolsFactory:
    """Implements `ToolsFactory` over the compiled builders of a directory."""

    def __init__(self, runner: ProcessRunner) -> None:
        self._runner = runner

    def open(self, binaries_dir: Path, extra_env: Mapping[str, str]) -> BuiltTools:
        return BuiltTools(
            BinaryBuilders(self._runner, binaries_dir, extra_env),
            BinaryProductReader(self._runner, binaries_dir, extra_env),
            BinaryAlignmentDriver(self._runner, binaries_dir),
        )


class SessionBusy(Exception):
    """Another session holds the lock of the run."""


@contextmanager
def exclusive_session_lock(path: Path) -> Generator[None]:
    """An exclusive, non-blocking `flock` on `runs/<run-id>/.session.lock`."""
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a+") as handle:
        try:
            fcntl.flock(handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except OSError as error:
            raise SessionBusy(f"another session holds {path}") from error
        try:
            yield
        finally:
            fcntl.flock(handle.fileno(), fcntl.LOCK_UN)


def run_window() -> RunWindow | None:
    return RunWindow.parse(os.environ.get(RUN_WINDOW_ENV, DEFAULT_RUN_WINDOW))


def block_environment_unset() -> tuple[str, ...]:
    """The guard's own floor stays unset for a model block (§6.1)."""
    return (MEM_GUARD_ENV,)


class GuardedBlockLauncher:
    """Implements `BlockLauncher`: one `script/mem-guard run -- <harness> run --model <id>` job."""

    def __init__(self, runner: ProcessRunner, layout: Layout, run_id: str) -> None:
        self._runner = runner
        self._layout = layout
        self._run_id = run_id

    def launch(self, model_id: str) -> int:
        command = [
            str(self._layout.mem_guard),
            "run",
            "--",
            "uv",
            "run",
            "--frozen",
            "--project",
            str(self._layout.harness_dir),
            "python",
            "-m",
            "calibration.cli",
            "run",
            "--run-id",
            self._run_id,
            "--model",
            model_id,
        ]
        return self._runner.run(command, cwd=self._layout.root, unset=block_environment_unset()).returncode


class ManifestHarnessIdentity:
    """Implements `HarnessIdentity`: the harness commit and the digest of the builder binaries on disk, under the
    keys the manifest records them."""

    def __init__(self, vcs: GitVersionControl, binaries_dir: Path) -> None:
        self._vcs = vcs
        self._binaries_dir = binaries_dir

    def identity(self) -> Mapping[str, str]:
        digests = {
            name: file_digest(self._binaries_dir / name)
            for name in BUILDER_BINARIES
            if (self._binaries_dir / name).is_file()
        }
        return {HARNESS_COMMIT_KEY: self._vcs.head_commit(), BUILDER_DIGEST_KEY: builder_rollup(digests)}


class RunServices:
    """The adapters every verb shares, built once per invocation."""

    def __init__(self, layout: Layout, run_id: str) -> None:
        self.layout = layout
        self.run_id = run_id
        self.runner = SubprocessRunner()
        self.transport = UrllibTransport()
        self.clock = SystemClock()
        self.sleeper = SystemSleeper()
        self.store = FileRunStore(layout.run_dir(run_id))
        self.vcs = GitVersionControl(layout.root, self.runner)

    def tools_factory(self) -> BinaryToolsFactory:
        return BinaryToolsFactory(self.runner)

    def server_factory(self) -> ExperimentServerFactory:
        return ExperimentServerFactory(
            self.transport,
            kev_directory=self.layout.kev_directory,
            kev_commit=KEV_COMMIT,
            clock=self.clock,
            sleeper=self.sleeper,
            process_runner=self.runner,
        )

    def snapshot_use_case(self) -> SnapshotUseCase:
        return SnapshotUseCase(self.vcs, self.store, FileTreeDigester(), self.layout.cache_dir)

    def build_items_deps(self) -> BuildItemsDeps:
        layout = self.layout
        shallnot = PinnedShallnotProvider(self.runner, layout.mem_guard, layout.shallnot_source())
        return BuildItemsDeps(
            vcs=self.vcs,
            pilot_git=GitPilotHistory(self.vcs),
            timestamps=OpensslTimestampReader(self.runner),
            exporter=GitProductExporter(self.vcs, layout.root),
            shallnot=shallnot,
            toolchain=GoBuilderToolchain(self.runner, EMPTY_PREFIX, {}),
            tools=self.tools_factory(),
            workspace=PytestFixtureWorkspace(sys.executable),
            snapshot=self.snapshot_use_case(),
            snapshot_source=UnpackedSnapshot,
            pilot_domain=PilotSnapshotDomain,
            pilot_reader=AllowlistedPilotRunReader(layout.pilot_run_dir),
            digester=FileTreeDigester(),
        )

    def kev_environment(self) -> SubprocessKevEnvironment:
        return SubprocessKevEnvironment(self.runner, self.layout.kev_directory, KEV_REPO_URL, KEV_COMMIT)

    def registry(self) -> HttpOllamaRegistry:
        return HttpOllamaRegistry(self.transport)

    def hub(self) -> HttpModelHub:
        return HttpModelHub(self.transport)

    def provisioner(self) -> ServerOllamaProvisioner:
        factory = self.server_factory()
        return ServerOllamaProvisioner(factory.ollama_server)

    def memory_probe(self) -> SystemMemoryProbe:
        return SystemMemoryProbe(self.runner)

    def block_deps(self) -> BlockDeps:
        manifest = RunManifest(self.store).read()
        commit = str(manifest.get(HARNESS_COMMIT_KEY, ""))
        binaries_dir = self.layout.cache_dir / f"{BUILDERS_DIR_PREFIX}{commit}"
        return BlockDeps(
            store=self.store,
            clock=self.clock,
            sleeper=self.sleeper,
            transport=self.transport,
            servers=self.server_factory(),
            tools=self.tools_factory().open(binaries_dir, {}),
            heavy=SystemHeavyWorkProbe(self.runner, self.transport),
            provenance=SystemProvenanceProbe(self.runner),
            harness=ManifestHarnessIdentity(self.vcs, binaries_dir),
            window=run_window(),
            canary_closed_text=self.layout.canary_prompt.read_text(encoding=ENCODING),
        )
