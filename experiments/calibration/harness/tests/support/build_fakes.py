"""Test doubles of the pin and build side: an in-memory run store and the ports of `pin`, `snapshot` and `build-items`.

Every double implements a whole port; a method the use cases never call raises `AssertionError` so that an
unexpected dependency shows up as a failure.
"""

from __future__ import annotations

from collections.abc import Callable, Mapping, Sequence
from datetime import datetime
from pathlib import Path

from calibration.application.build_ports import (
    AlignmentDriver,
    BuilderBuild,
    Builders,
    BuiltTools,
    UnitPair,
)
from calibration.application.pin_ports import (
    HeadMeta,
    HubRepository,
    KevEnvironmentInfo,
    OllamaManifest,
    OllamaModelRecord,
    OllamaProvision,
)
from calibration.application.ports import MemoryReading
from calibration.domain.canonical import JsonValue, canonical_json, sha256_hex
from calibration.domain.ports import (
    DecodeOutcome,
    DecodeRequest,
    EndpointDigest,
    ProductDefaults,
    ScreenReading,
    ScreenReadRequest,
    ThresholdCheck,
    Timestamp,
)

RUN_ROOT = Path("/runs/run-test")
NEVER_CALLED = "this port method is not used by the code under test"


class InMemoryRunStore:
    """A `RunStore` over a dictionary; `write_log` lists every path written or appended, in order."""

    def __init__(self, files: Mapping[str, str] | None = None) -> None:
        self._files: dict[str, bytes] = {path: text.encode() for path, text in (files or {}).items()}
        self.write_log: list[str] = []

    @property
    def root(self) -> Path:
        return RUN_ROOT

    def exists(self, relative: str) -> bool:
        return relative in self._files

    def read_bytes(self, relative: str) -> bytes:
        return self._files[relative]

    def read_text(self, relative: str) -> str:
        return self._files[relative].decode()

    def write_bytes(self, relative: str, data: bytes) -> None:
        self.write_log.append(relative)
        self._files[relative] = data

    def write_text(self, relative: str, text: str) -> None:
        self.write_bytes(relative, text.encode())

    def append_text(self, relative: str, text: str) -> None:
        self.write_log.append(relative)
        self._files[relative] = self._files.get(relative, b"") + text.encode()

    def list_dir(self, relative: str) -> list[str]:
        prefix = relative.rstrip("/") + "/"
        return sorted({path.removeprefix(prefix).split("/", 1)[0] for path in self._files if path.startswith(prefix)})

    def paths(self) -> list[str]:
        return sorted(self._files)

    def count_writes_to(self, relative: str) -> int:
        return self.write_log.count(relative)


class FakeMemoryProbe:
    def __init__(self, total_bytes: int) -> None:
        self._total = total_bytes

    def total_memory_bytes(self) -> int:
        return self._total

    def reading(self) -> MemoryReading:
        return MemoryReading(free_percent=100, pressure_normal=True)


class FakeOllamaRegistry:
    """Serves manifests by identifier (`library:tag`) and notes each request in the shared event log."""

    def __init__(self, manifests: dict[str, OllamaManifest], events: list[str]) -> None:
        self.manifests = manifests
        self._events = events

    def manifest(self, library: str, tag: str) -> OllamaManifest:
        identifier = f"{library}:{tag}"
        self._events.append(f"manifest:{identifier}")
        return self.manifests[identifier]


class FakeModelHub:
    """Serves repositories by `(repository, revision)`; a named revision resolves to its own commit sha."""

    def __init__(self, repositories: dict[tuple[str, str | None], HubRepository], events: list[str]) -> None:
        self.repositories = repositories
        self._events = events

    def repository(self, repository: str, revision: str | None = None) -> HubRepository:
        self._events.append(f"hub:{repository}@{revision}")
        return self.repositories[(repository, revision)]


class FakeKevEnvironment:
    def __init__(self, info: KevEnvironmentInfo, heads: dict[str, HeadMeta], events: list[str]) -> None:
        self.info = info
        self.heads = heads
        self.downloads: list[tuple[str, str | None]] = []
        self.setups = 0
        self._events = events

    def setup(self) -> KevEnvironmentInfo:
        self.setups += 1
        self._events.append("kev-setup")
        return self.info

    def head_meta(self, repository: str, revision: str) -> HeadMeta:
        self._events.append(f"head:{repository}@{revision}")
        return self.heads[repository]

    def download(self, repository: str, revision: str | None) -> None:
        self._events.append(f"download:{repository}@{revision}")
        self.downloads.append((repository, revision))

    def downloaded_repositories(self) -> list[str]:
        return [repository for repository, _ in self.downloads]


class FakeOllamaProvisioner:
    def __init__(self, version: str, records: dict[str, OllamaModelRecord], events: list[str]) -> None:
        self.version = version
        self.records = records
        self.provisioned: list[list[str]] = []
        self._events = events

    def provision(self, tags: Sequence[str]) -> OllamaProvision:
        self._events.append("provision")
        self.provisioned.append(list(tags))
        return OllamaProvision(self.version, {tag: self.records[tag] for tag in tags})


class FakeVersionControl:
    """Tags, a head commit, blobs and a cleanliness flag; `archive` only records what it is asked to unpack."""

    def __init__(self, tags: dict[str, str], head: str, blobs: dict[tuple[str, str], bytes], *, clean: bool) -> None:
        self.tags = tags
        self.head = head
        self.blobs = blobs
        self.clean = clean
        self.archived: list[tuple[str, tuple[str, ...], Path]] = []
        self.cleanliness_checked: list[tuple[str, ...]] = []

    def peel_tag(self, tag: str) -> str | None:
        return self.tags.get(tag)

    def head_commit(self) -> str:
        return self.head

    def is_clean(self, paths: Sequence[str]) -> bool:
        self.cleanliness_checked.append(tuple(paths))
        return self.clean

    def archive(
        self, revision: str, pathspecs: Sequence[str], destination: Path, *, exclude: Sequence[str] = ()
    ) -> None:
        self.archived.append((revision, tuple(pathspecs), destination))

    def is_ancestor(self, ancestor: str, descendant: str) -> bool:
        raise AssertionError(NEVER_CALLED)

    def commit_time(self, commit: str) -> datetime:
        raise AssertionError(NEVER_CALLED)

    def commits_touching(self, revision: str, path: str) -> list[str]:
        raise AssertionError(NEVER_CALLED)

    def blob(self, revision: str, path: str) -> bytes | None:
        return self.blobs.get((revision, path))


class FakeTimestampReader:
    def __init__(self, stamp: Timestamp | None) -> None:
        self.stamp = stamp
        self.read_from: list[Path] = []

    def read(self, reply: Path) -> Timestamp | None:
        self.read_from.append(reply)
        return self.stamp


class FakePilotGit:
    def __init__(self, tags: dict[str, str], analysis: str | None) -> None:
        self.tags = tags
        self.analysis = analysis

    def tag_commit(self, tag: str) -> str | None:
        return self.tags.get(tag)

    def analysis_commit(self, run_id: str, freeze_commit: str) -> str | None:
        return self.analysis


class FakeProductExporter:
    def __init__(self, digests: dict[str, str]) -> None:
        self.digests = digests
        self.exports: list[tuple[str, Path]] = []

    def export(self, revision: str, destination: Path) -> dict[str, str]:
        self.exports.append((revision, destination))
        return dict(self.digests)


class FakeShallnotProvider:
    def __init__(self) -> None:
        self.provided_for: list[Path] = []

    def provide(self, export_dir: Path) -> Path:
        self.provided_for.append(export_dir)
        return export_dir / "script" / "shallnot"


class FakeBuilderToolchain:
    def __init__(self, build: BuilderBuild) -> None:
        self.result = build
        self.builds: list[tuple[Path, Path, Path, int]] = []

    def build(self, export_dir: Path, builder_dir: Path, output_dir: Path, procs: int) -> BuilderBuild:
        self.builds.append((export_dir, builder_dir, output_dir, procs))
        return self.result


class FakeFixtureWorkspace:
    def __init__(self) -> None:
        self.materialized: list[tuple[Path, Path]] = []

    def materialize(self, fixture_dir: Path, destination: Path) -> Path:
        self.materialized.append((fixture_dir, destination))
        return destination


class FakeTreeDigester:
    """Digests a fixed set of files wherever it is asked; `events` orders removals against other effects."""

    def __init__(self, files: dict[str, str], events: list[str]) -> None:
        self.files = files
        self.removed: list[Path] = []
        self._events = events

    def digest(self, root: Path) -> dict[str, str]:
        return dict(sorted(self.files.items()))

    def remove(self, root: Path) -> None:
        self.removed.append(root)
        self._events.append(f"remove:{root}")


class FakePilotRunReader:
    """Reads files of an in-memory pilot run and digests the ones read, as the real reader does (§2.3)."""

    def __init__(self, files: dict[str, str], seal_mismatches: list[str] | None = None) -> None:
        self.files = files
        self.seal_mismatches = seal_mismatches or []
        self.read_paths: list[str] = []

    def exists(self, relative: str) -> bool:
        return relative in self.files

    def read_text(self, relative: str) -> str:
        self.read_paths.append(relative)
        return self.files[relative]

    def digests(self) -> dict[str, str]:
        return {path: sha256_hex(self.files[path]) for path in sorted(set(self.read_paths))}

    def verify_against_seal(self) -> list[str]:
        return list(self.seal_mismatches)


class FakeSnapshotSource:
    def __init__(self, project_dir: Path, sources: dict[str, str]) -> None:
        self._project_dir = project_dir
        self._sources = sources

    @property
    def project_dir(self) -> Path:
        return self._project_dir

    @property
    def pilot_source_root(self) -> Path:
        return self._project_dir / "pilot-source"

    def fixture_sources(self) -> dict[str, str]:
        return dict(self._sources)


def fake_mutant_id(normalized_edits: Sequence[Mapping[str, str]]) -> str:
    """The id the fake pilot domain derives from normalized edits: 16 hex characters of their canonical digest."""
    edits: list[JsonValue] = [dict(edit) for edit in normalized_edits]
    return sha256_hex(canonical_json(edits))[:16]


class FakePilotDomain:
    def __init__(self) -> None:
        self.identity_checks = 0

    def mutant_id(self, normalized_edits: Sequence[Mapping[str, str]]) -> str:
        self.identity_checks += 1
        return fake_mutant_id(normalized_edits)

    def unit_pairs(self, sources: Mapping[str, str], normalized_edits: Sequence[Mapping[str, str]]) -> list[UnitPair]:
        return [
            UnitPair(edit["file"], edit["qualname"], sources.get(edit["file"], ""), edit["new_source"])
            for edit in normalized_edits
        ]


class FakeAlignmentDriver:
    def __init__(self, exported_text: str) -> None:
        self.exported_text = exported_text
        self.calls: list[tuple[Path, Path, Path]] = []

    def first_wave_export(self, export_dir: Path, shallnot_script: Path, report: Path) -> str:
        self.calls.append((export_dir, shallnot_script, report))
        return self.exported_text


class FakeBuilders:
    """Request builders over scripted outputs; `screen_build` echoes the mutants it is given."""

    def __init__(
        self,
        alignment_output: Mapping[str, JsonValue],
        compare_report: Mapping[str, JsonValue],
        screen_template_digest: str,
    ) -> None:
        self.alignment_output = alignment_output
        self.compare_report = compare_report
        self.screen_template_digest = screen_template_digest
        self.omitted_mutants: set[str] = set()
        self.extra_mutants: list[str] = []
        self.alignment_calls: list[tuple[Path, Path, tuple[str, ...]]] = []
        self.compare_calls: list[tuple[Path, Path, str]] = []
        self.screen_calls: list[list[str]] = []

    def alignment_build(self, project: Path, shallnot: Path, identifiers: Sequence[str]) -> Mapping[str, JsonValue]:
        self.alignment_calls.append((project, shallnot, tuple(identifiers)))
        return self.alignment_output

    def alignment_defaults(self) -> Mapping[str, JsonValue]:
        raise AssertionError(NEVER_CALLED)

    def render_closed(self, closed_text: str, identifiers: Sequence[str]) -> Mapping[str, JsonValue]:
        raise AssertionError(NEVER_CALLED)

    def compare_export(self, project: Path, shallnot: Path, export_text: str) -> Mapping[str, JsonValue]:
        self.compare_calls.append((project, shallnot, export_text))
        return self.compare_report

    def screen_build(
        self, project: Path, shallnot: Path, identifiers: Sequence[str], mutants: Sequence[Mapping[str, JsonValue]]
    ) -> Mapping[str, JsonValue]:
        requested = [str(mutant["mutant_id"]) for mutant in mutants]
        self.screen_calls.append(requested)
        answered = [mutant_id for mutant_id in requested if mutant_id not in self.omitted_mutants]
        items: list[JsonValue] = [
            self._screen_item(mutant_id, identifiers) for mutant_id in [*answered, *self.extra_mutants]
        ]
        return {"items": items}

    def screen_defaults(self) -> Mapping[str, JsonValue]:
        raise AssertionError(NEVER_CALLED)

    def _screen_item(self, mutant_id: str, identifiers: Sequence[str]) -> dict[str, JsonValue]:
        closed: dict[str, JsonValue] = {"questions": {"violates": f"does mutant {mutant_id} violate the criterion"}}
        return {
            "mutant_id": mutant_id,
            "closed": canonical_json(closed),
            "template_digest": self.screen_template_digest,
            "bodies": {identifier: f"body of {identifier} for {mutant_id}" for identifier in identifiers},
            "question_to_ref": {"violates": "PARK-1~1"},
        }


class FakeProductReader:
    """Only `defaults` is used: the template digests of the frozen modules."""

    def __init__(self, defaults: ProductDefaults) -> None:
        self._defaults = defaults

    def decode(self, requests: Sequence[DecodeRequest]) -> list[DecodeOutcome]:
        raise AssertionError(NEVER_CALLED)

    def read_screen(self, requests: Sequence[ScreenReadRequest]) -> list[ScreenReading]:
        raise AssertionError(NEVER_CALLED)

    def endpoint_digest(self, address: str, model: str) -> EndpointDigest:
        raise AssertionError(NEVER_CALLED)

    def check_threshold(self, entry_yaml: str, default: float, sources: Sequence[str]) -> ThresholdCheck:
        raise AssertionError(NEVER_CALLED)

    def defaults(self) -> ProductDefaults:
        return self._defaults


class FakeToolsFactory:
    """Opens the same fake tools for every call and records the binaries directory and the extra environment."""

    def __init__(self, builders: Builders, reader: FakeProductReader, driver: AlignmentDriver) -> None:
        self._builders = builders
        self._reader = reader
        self._driver = driver
        self.opened: list[tuple[Path, dict[str, str]]] = []

    def open(self, binaries_dir: Path, extra_env: Mapping[str, str]) -> BuiltTools:
        self.opened.append((binaries_dir, dict(extra_env)))
        return BuiltTools(self._builders, self._reader, self._driver)


def snapshot_source_factory(sources: dict[str, str]) -> Callable[[Path], FakeSnapshotSource]:
    def make(directory: Path) -> FakeSnapshotSource:
        return FakeSnapshotSource(directory / "fixture" / "parkfee", sources)

    return make
