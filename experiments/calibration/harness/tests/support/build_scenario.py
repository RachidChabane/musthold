"""A consistent world for `build-items`: a frozen pre-registration, a qualifying pilot run and builders that agree."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass, field
from datetime import UTC, datetime
from pathlib import Path

from calibration.application.build_items import (
    FREEZE_COMMIT_PATH,
    BuildItemsDeps,
    BuildItemsSettings,
    BuildItemsUseCase,
)
from calibration.application.build_ports import BuilderBuild
from calibration.application.build_records import PROMPT_FILES, SNAPSHOT_PREREG_PREFIX
from calibration.application.snapshot import SnapshotUseCase
from calibration.domain.canonical import JsonValue, canonical_json, sha256_hex
from calibration.domain.constants import (
    PILOT_BINDINGS_FILE,
    PILOT_DIFFERENTIAL_FILE,
    PILOT_FREEZE_TAG,
    PILOT_MUTANTS_DIR,
    PILOT_PROCESSING_FILE,
    PREREG_TAG,
)
from calibration.domain.models import identifiers
from calibration.domain.nodeids import nodeid_of
from calibration.domain.ports import ProductDefaults, Timestamp
from support.build_fakes import (
    FakeAlignmentDriver,
    FakeBuilders,
    FakeBuilderToolchain,
    FakeFixtureWorkspace,
    FakePilotDomain,
    FakePilotGit,
    FakePilotRunReader,
    FakeProductExporter,
    FakeProductReader,
    FakeShallnotProvider,
    FakeTimestampReader,
    FakeToolsFactory,
    FakeTreeDigester,
    FakeVersionControl,
    InMemoryRunStore,
    fake_mutant_id,
    snapshot_source_factory,
)

RUN_ID = "run-test"
PREREG_COMMIT = "a" * 40
PILOT_FREEZE_COMMIT = "b" * 40
PILOT_ANALYSIS_COMMIT = "c" * 40
HARNESS_COMMIT = "d" * 40
ALIGNMENT_TEMPLATE_DIGEST = "alignment-template-digest"
SCREEN_TEMPLATE_DIGEST = "screen-template-digest"
CACHE_DIR = Path("/cache")
BUILDER_DIR = Path("/builder")
FREEZE_RECEIPT = Path("/freeze/prereg-commit.tsr")
HARNESS_PATHS = ("experiments/calibration/harness",)
GO_PROCS = 2
FIXTURE_SOURCE_FILE = "src/parkfee/fees.py"
FIXTURE_SOURCES = {
    FIXTURE_SOURCE_FILE: (
        "from datetime import datetime\nfrom decimal import Decimal\n\n\n"
        "def compute_fee(start: datetime, end: datetime) -> Decimal:\n    return Decimal(0)\n\n\n"
        "def count(n):\n    return n\n"
    )
}
MAPPABLE_ARGUMENTS: dict[str, JsonValue] = {"start": "2026-01-01T00:00:00", "end": "2026-01-01T01:00:00"}
UNMAPPABLE_ARGUMENTS: dict[str, JsonValue] = {"n": 3}
ARM_X = "X"
STATUS_DUPLICATE = "DUPLICATE"
MASKER_REASON = "unmaskable"
BINARY_DIGESTS = {"alignment-builder": "binary-1", "discpower-builder": "binary-2"}
SNAPSHOT_FILE_DIGESTS = {
    f"{SNAPSHOT_PREREG_PREFIX}{path}": sha256_hex(path) for pair in PROMPT_FILES.values() for path in pair
}
EXPORT_DIGESTS = {"go.work": "export-1", "format/go.mod": "export-2"}


@dataclass(frozen=True)
class AlignmentTest:
    file: str
    classname: str
    name: str
    reference: str
    rejected: bool = False

    @property
    def nodeid(self) -> str:
        return nodeid_of(self.file, self.classname, self.name)


@dataclass(frozen=True)
class PilotMutant:
    """A processing row, its mutant document and optionally its differential record."""

    qualname: str
    arm: str
    stillborn: bool = False
    status: str = "OK"
    differential: Mapping[str, JsonValue] | None = None
    recorded_id: str | None = None

    @property
    def edits(self) -> list[dict[str, str]]:
        source = f"def {self.qualname}(): return 1"
        return [{"file": FIXTURE_SOURCE_FILE, "qualname": self.qualname, "new_source": source}]

    @property
    def mutant_id(self) -> str:
        return self.recorded_id or fake_mutant_id(self.edits)


def differs(mapped: bool) -> dict[str, JsonValue]:
    """An F3 record that DIFFERS; the arguments of an unmapped one carry no literal type."""
    return {
        "status": "DIFFERS",
        "function": "compute_fee" if mapped else "count",
        "args": MAPPABLE_ARGUMENTS if mapped else UNMAPPABLE_ARGUMENTS,
        "original_out": {"ok": ["decimal", "1.00"]},
        "mutant_out": {"ok": ["decimal", "2.00"]},
    }


DEFAULT_TESTS = (
    AlignmentTest("tests/test_fee.py", "tests.test_fee.TestFee", "test_charges", "PARK-1~1"),
    AlignmentTest("tests/test_fee.py", "tests.test_fee", "test_free_hour", "PARK-2~1"),
    AlignmentTest("tests/test_fee.py", "tests.test_fee.TestFee", "test_masked", "PARK-3~1", rejected=True),
)
DEFAULT_MUTANTS = (
    PilotMutant("mapped_target", "T", differential=differs(mapped=True)),
    PilotMutant("unmapped_pocket", "P", differential=differs(mapped=False)),
    PilotMutant("no_differential", "T"),
    PilotMutant("stillborn_one", "T", stillborn=True),
    PilotMutant("duplicate_one", "T", status=STATUS_DUPLICATE),
    PilotMutant("operator_arm", ARM_X),
)


def alignment_build_output(tests: tuple[AlignmentTest, ...], template_digest: str) -> dict[str, JsonValue]:
    items: list[JsonValue] = []
    for test in tests:
        closed: dict[str, JsonValue] = {"questions": {"aligned": f"does {test.name} check {test.reference}"}}
        entry: dict[str, JsonValue] = {
            "file": test.file,
            "classname": test.classname,
            "name": test.name,
            "label": f"label of {test.name}",
            "declared": [test.reference],
            "closed": canonical_json(closed),
            "template_digest": template_digest,
            "bodies": {identifier: f"body of {identifier} for {test.name}" for identifier in identifiers()},
            "token_to_ref": {"T1": test.reference},
            "rejected": {"reason": MASKER_REASON, "detail": "the test names its criterion"} if test.rejected else None,
        }
        items.append(entry)
    criteria: list[JsonValue] = [{"ref": test.reference} for test in tests]
    return {"items": items, "criteria": criteria, "template_digest": template_digest}


def pilot_run_files(tests: tuple[AlignmentTest, ...], mutants: tuple[PilotMutant, ...]) -> dict[str, str]:
    files = {PILOT_BINDINGS_FILE: json.dumps({test.nodeid: test.reference for test in tests})}
    files[PILOT_PROCESSING_FILE] = "\n".join(
        json.dumps({"mutant_id": m.mutant_id, "stillborn": m.stillborn, "status": m.status, "duplicate_of": None})
        for m in mutants
    )
    files[PILOT_DIFFERENTIAL_FILE] = "\n".join(
        json.dumps({"mutant_id": m.mutant_id, **m.differential}) for m in mutants if m.differential is not None
    )
    for mutant in mutants:
        files[f"{PILOT_MUTANTS_DIR}/{mutant.mutant_id}.json"] = json.dumps(
            {"mutant_id": mutant.mutant_id, "arm": mutant.arm, "normalized_edits": mutant.edits}
        )
    return files


@dataclass
class BuildWorld:
    """Every port of `build-items` over fakes, sharing one event log; tests change a fake before `use_case().run()`."""

    tests: tuple[AlignmentTest, ...] = DEFAULT_TESTS
    mutants: tuple[PilotMutant, ...] = DEFAULT_MUTANTS
    clean: bool = True
    events: list[str] = field(default_factory=list[str])

    def __post_init__(self) -> None:
        recorded = f"{PREREG_COMMIT}\n"
        stamp = Timestamp(sha256_hex(recorded), datetime(2026, 10, 7, tzinfo=UTC))
        tags = {PREREG_TAG: PREREG_COMMIT, PILOT_FREEZE_TAG: PILOT_FREEZE_COMMIT}
        self.store = InMemoryRunStore()
        self.vcs = FakeVersionControl(
            tags, HARNESS_COMMIT, {("HEAD", FREEZE_COMMIT_PATH): recorded.encode()}, clean=self.clean
        )
        self.pilot_git = FakePilotGit(tags, PILOT_ANALYSIS_COMMIT)
        self.timestamps = FakeTimestampReader(stamp)
        self.exporter = FakeProductExporter(EXPORT_DIGESTS)
        self.shallnot = FakeShallnotProvider()
        self.toolchain = FakeBuilderToolchain(BuilderBuild({"alignment": "source-1"}, BINARY_DIGESTS))
        self.workspace = FakeFixtureWorkspace()
        self.digester = FakeTreeDigester({**SNAPSHOT_FILE_DIGESTS, FIXTURE_SOURCE_FILE: "fixture-1"}, self.events)
        self.reader = FakePilotRunReader(pilot_run_files(self.tests, self.mutants))
        self.pilot_domain = FakePilotDomain()
        self.builders = FakeBuilders(
            alignment_build_output(self.tests, ALIGNMENT_TEMPLATE_DIGEST),
            {"exported": 3, "rebuilt": 3, "matched": 3, "unmatched_exported": [], "unmatched_rebuilt": []},
            SCREEN_TEMPLATE_DIGEST,
        )
        self.driver = FakeAlignmentDriver("systemone export text")
        defaults = ProductDefaults(0.2, 0.5, SCREEN_TEMPLATE_DIGEST, ALIGNMENT_TEMPLATE_DIGEST)
        self.tools = FakeToolsFactory(self.builders, FakeProductReader(defaults), self.driver)

    def settings(self, *, allow_dirty: bool = False) -> BuildItemsSettings:
        return BuildItemsSettings(RUN_ID, CACHE_DIR, BUILDER_DIR, GO_PROCS, allow_dirty, HARNESS_PATHS, FREEZE_RECEIPT)

    def use_case(self, *, allow_dirty: bool = False) -> BuildItemsUseCase:
        deps = BuildItemsDeps(
            vcs=self.vcs,
            pilot_git=self.pilot_git,
            timestamps=self.timestamps,
            exporter=self.exporter,
            shallnot=self.shallnot,
            toolchain=self.toolchain,
            tools=self.tools,
            workspace=self.workspace,
            snapshot=SnapshotUseCase(self.vcs, self.store, self.digester, CACHE_DIR),
            snapshot_source=snapshot_source_factory(FIXTURE_SOURCES),
            pilot_domain=lambda _source: self.pilot_domain,
            pilot_reader=self.reader,
            digester=self.digester,
        )
        return BuildItemsUseCase(self.settings(allow_dirty=allow_dirty), deps, self.store)

    def mutant(self, qualname: str) -> PilotMutant:
        return next(mutant for mutant in self.mutants if mutant.qualname == qualname)
