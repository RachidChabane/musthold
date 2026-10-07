"""Ports of `snapshot` and `build-items`: the product export, the Go builders, the pilot snapshot and the pilot run."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Protocol

from calibration.domain.canonical import JsonValue
from calibration.domain.ports import ProductReader


@dataclass(frozen=True)
class UnitPair:
    """A normalized definition before and after a mutant: what the V-H votes saw (§4.3)."""

    file: str
    qualname: str
    before: str
    after: str


class PilotDomain(Protocol):
    """`pilot.domain` of the snapshot: `normalization`, `definitions`, `ids`, `edits` and `mutants` (§2.4)."""

    def mutant_id(self, normalized_edits: Sequence[Mapping[str, str]]) -> str:
        """The first 16 hex characters of the digest of the canonical normalized edit set."""
        ...

    def unit_pairs(self, sources: Mapping[str, str], normalized_edits: Sequence[Mapping[str, str]]) -> list[UnitPair]:
        """`normalize(definition_source(source, file, qualname))` before and after `apply_edit_set`, per edit."""
        ...


class ProductExporter(Protocol):
    def export(self, revision: str, destination: Path) -> dict[str, str]:
        """Unpacks the product tree of `revision` without `experiments/` and returns the SHA-256 of every file."""
        ...


class ShallnotProvider(Protocol):
    def provide(self, export_dir: Path) -> Path:
        """The `shallnot` executable the gateways run: built by the exported `script/shallnot-build` at the pin."""
        ...


@dataclass(frozen=True)
class BuilderBuild:
    """The digests of the builder sources and binaries (§4.1)."""

    source_digests: Mapping[str, str]
    binary_digests: Mapping[str, str]


class BuilderToolchain(Protocol):
    def build(self, export_dir: Path, builder_dir: Path, output_dir: Path, procs: int) -> BuilderBuild:
        """Copies each builder beside its module's code in the export and compiles it with `go build -p <procs>
        -trimpath` under the memory guard."""
        ...


class Builders(Protocol):
    """The built request builders, driven through their command lines; every result is parsed JSON."""

    def alignment_build(self, project: Path, shallnot: Path, identifiers: Sequence[str]) -> Mapping[str, JsonValue]: ...

    def alignment_defaults(self) -> Mapping[str, JsonValue]: ...

    def render_closed(self, closed_text: str, identifiers: Sequence[str]) -> Mapping[str, JsonValue]:
        """`ParseClosedSet(closed).SystemOneRequest(model)` for each identifier."""
        ...

    def compare_export(self, project: Path, shallnot: Path, export_text: str) -> Mapping[str, JsonValue]:
        """Rebuilds every closed set of an `awaiting_host` export from the same inputs and compares them byte for byte."""
        ...

    def screen_build(
        self, project: Path, shallnot: Path, identifiers: Sequence[str], mutants: Sequence[Mapping[str, JsonValue]]
    ) -> Mapping[str, JsonValue]: ...

    def screen_defaults(self) -> Mapping[str, JsonValue]: ...


class AlignmentDriver(Protocol):
    def first_wave_export(self, export_dir: Path, shallnot_script: Path, report: Path) -> str:
        """Builds the alignment binary of the export, drives it on `s1relay/testdata/alignment/project` to its
        first `awaiting_host` wave by the procedure of `s1relay/internal/e2e/e2e_test.go`, and returns the text of
        `systemone export`."""
        ...


class FixtureWorkspace(Protocol):
    def materialize(self, fixture_dir: Path, destination: Path) -> Path:
        """A copy of `fixture/parkfee` with the `shallnot.yaml` the gate needs (specs, tests, results and a test
        command that runs pytest with JUnit output); returns the project root of the copy."""
        ...


class PilotRunReader(Protocol):
    """Reads the files of run-02 that `build-items` is allowed to read, and records the digest of each (§2.3)."""

    def exists(self, relative: str) -> bool: ...

    def read_text(self, relative: str) -> str: ...

    def digests(self) -> dict[str, str]: ...

    def verify_against_seal(self) -> list[str]:
        """The files read whose digest differs from `SHA256SUMS`; empty when the seal is absent or agrees."""
        ...


class PilotGit(Protocol):
    """The pilot's history: its tags and the commit of its analysis (§2.2, §14.1)."""

    def tag_commit(self, tag: str) -> str | None: ...

    def analysis_commit(self, run_id: str, freeze_commit: str) -> str | None:
        """The earliest commit descending from the freeze commit whose `results/decisions.json` names the run."""
        ...


class SnapshotSource(Protocol):
    """The unpacked pilot snapshot: its `fixture/parkfee` sources and the path its `pilot` package is imported from."""

    @property
    def project_dir(self) -> Path: ...

    @property
    def pilot_source_root(self) -> Path: ...

    def fixture_sources(self) -> dict[str, str]:
        """The Python sources of `fixture/parkfee/src/parkfee/`, keyed by their path relative to the project root."""
        ...


class TreeDigester(Protocol):
    def digest(self, root: Path) -> dict[str, str]:
        """The SHA-256 of every file below `root`, keyed by relative POSIX path and sorted."""
        ...

    def remove(self, root: Path) -> None: ...


@dataclass(frozen=True)
class BuiltTools:
    """The compiled builders behind their ports: request builders, the product reader and the alignment driver."""

    builders: Builders
    reader: ProductReader
    alignment_driver: AlignmentDriver


class ToolsFactory(Protocol):
    def open(self, binaries_dir: Path, extra_env: Mapping[str, str]) -> BuiltTools:
        """The tools over the binaries of `binaries_dir`, run with `extra_env` laid over the environment."""
        ...
