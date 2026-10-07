"""`build-items` (§4, §14.1): the product export, the builders and every item, archived before any model call."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from calibration.application.alignment_items import alignment_problems, assemble_alignment_items
from calibration.application.build_ports import (
    BuilderToolchain,
    FixtureWorkspace,
    PilotDomain,
    PilotGit,
    PilotRunReader,
    ProductExporter,
    ShallnotProvider,
    SnapshotSource,
    ToolsFactory,
    TreeDigester,
)
from calibration.application.build_records import BuildFacts, BuildRecordsWriter
from calibration.application.builder_check import (
    RECORDED_REPORT,
    RECORDED_REPORT_ENV,
    BuilderCheck,
    run_builder_check,
)
from calibration.application.deviation_log import DeviationLog
from calibration.application.ports import RunStore
from calibration.application.screen_stage import ScreenStage
from calibration.application.snapshot import SnapshotUseCase
from calibration.domain.constants import (
    PILOT_BINDINGS_FILE,
    PILOT_FIXTURE_DIR,
    PILOT_FREEZE_TAG,
    PILOT_RUN_ID,
    PREREG_TAG,
    SCHEDULE_FILE,
)
from calibration.domain.freeze import PreconditionFailed, freeze_problems
from calibration.domain.models import identifiers
from calibration.domain.pilot_records import parse_bindings
from calibration.domain.ports import TimestampReader, VersionControl

PC1: Final = "PC1"
PC2: Final = "PC2"
PC3: Final = "PC3"
FREEZE_COMMIT_PATH: Final = "experiments/calibration/freeze/prereg-commit.txt"
EXPORT_DIR_PREFIX: Final = "product-"
BUILDERS_DIR_PREFIX: Final = "builders-"
WORK_DIR_PREFIX: Final = "work-"
FIXTURE_COPY_DIR: Final = "project"
PROJECT_IN_FIXTURE: Final = "parkfee"
REVISION_HEAD: Final = "HEAD"


@dataclass(frozen=True)
class BuildItemsSettings:
    run_id: str
    cache_dir: Path
    builder_dir: Path
    go_procs: int
    allow_dirty_harness: bool
    harness_paths: tuple[str, ...]
    freeze_receipt: Path


@dataclass(frozen=True)
class BuildItemsDeps:
    vcs: VersionControl
    pilot_git: PilotGit
    timestamps: TimestampReader
    exporter: ProductExporter
    shallnot: ShallnotProvider
    toolchain: BuilderToolchain
    tools: ToolsFactory
    workspace: FixtureWorkspace
    snapshot: SnapshotUseCase
    snapshot_source: Callable[[Path], SnapshotSource]
    pilot_domain: Callable[[SnapshotSource], PilotDomain]
    pilot_reader: PilotRunReader
    digester: TreeDigester


@dataclass(frozen=True)
class BuildItemsReport:
    alignment_items: int
    masker_rejections: int
    screen_items: int
    witness_unmapped: int
    screen_blocked: bool


class BuildItemsUseCase:
    """Checks PC1 to PC3 and writes `items/`, `schedule.json`, `excluded.json`, `pilot_inputs.json`, `manifest.json`
    and `builder_check.json`. Nothing is written when a precondition fails."""

    def __init__(self, settings: BuildItemsSettings, deps: BuildItemsDeps, store: RunStore) -> None:
        self._settings = settings
        self._deps = deps
        self._store = store
        self._screen = ScreenStage(deps.pilot_reader, DeviationLog(store), deps.pilot_domain)
        self._records = BuildRecordsWriter(store, deps.pilot_reader)

    def run(self) -> BuildItemsReport:
        if self._store.exists(SCHEDULE_FILE):
            raise PreconditionFailed(PC3, "the items of this run are archived; a run directory is built once")
        prereg_commit = self._check_freeze()
        harness_commit, harness_clean = self._check_harness()
        pilot_freeze, analysis_commit = self._check_pilot_history()
        snapshot = self._deps.snapshot.run()
        cache = self._settings.cache_dir
        export_dir = cache / f"{EXPORT_DIR_PREFIX}{prereg_commit}"
        self._deps.digester.remove(export_dir)
        export_digests = self._deps.exporter.export(prereg_commit, export_dir)
        shallnot = self._deps.shallnot.provide(export_dir)
        binaries_dir = cache / f"{BUILDERS_DIR_PREFIX}{harness_commit}"
        build = self._deps.toolchain.build(
            export_dir, self._settings.builder_dir, binaries_dir, self._settings.go_procs
        )
        tools = self._deps.tools.open(binaries_dir, {})
        defaults = tools.reader.defaults()
        check = self._builder_check(binaries_dir, export_dir)
        work_dir = cache / f"{WORK_DIR_PREFIX}{self._settings.run_id}"
        problems: list[str] = [] if check.passed else ["the builder check of PREREG 16 step 3 failed"]
        try:
            project = self._deps.workspace.materialize(
                snapshot.directory / PILOT_FIXTURE_DIR / PROJECT_IN_FIXTURE, work_dir / FIXTURE_COPY_DIR
            )
            alignment = assemble_alignment_items(tools.builders.alignment_build(project, shallnot, identifiers()))
            problems.extend(alignment_problems(alignment, self._bindings(), defaults.alignment_template_digest))
            screen = self._screen.build(
                self._deps.snapshot_source(snapshot.directory),
                tools.builders,
                project,
                shallnot,
                defaults.screen_template_digest,
                problems,
            )
        finally:
            self._deps.digester.remove(work_dir)
        if problems:
            raise PreconditionFailed(PC3, "; ".join(sorted(problems)))
        facts = BuildFacts(
            run_id=self._settings.run_id,
            prereg_commit=prereg_commit,
            harness_commit=harness_commit,
            harness_clean=harness_clean,
            pilot_freeze_commit=pilot_freeze,
            pilot_analysis_commit=analysis_commit,
            snapshot_files=snapshot.files,
            export_digests=export_digests,
            build=build,
            go_procs=self._settings.go_procs,
            alignment_template_digest=defaults.alignment_template_digest,
            screen_template_digest=defaults.screen_template_digest,
        )
        self._records.write(alignment, screen, check, facts)
        return BuildItemsReport(
            len(alignment.items), len(alignment.rejections), len(screen.items), len(screen.unmapped), screen.blocked
        )

    def _check_freeze(self) -> str:
        commit = self._deps.vcs.peel_tag(PREREG_TAG)
        recorded = self._deps.vcs.blob(REVISION_HEAD, FREEZE_COMMIT_PATH)
        stamp = self._deps.timestamps.read(self._settings.freeze_receipt)
        problems = freeze_problems(commit, recorded.decode() if recorded is not None else None, stamp)
        if problems or commit is None:
            raise PreconditionFailed(PC1, "; ".join(problems))
        return commit

    def _check_harness(self) -> tuple[str, bool]:
        commit = self._deps.vcs.head_commit()
        clean = self._deps.vcs.is_clean(self._settings.harness_paths)
        if not clean and not self._settings.allow_dirty_harness:
            raise PreconditionFailed(PC1, "the harness has uncommitted changes, so no commit identifies it")
        return commit, clean

    def _check_pilot_history(self) -> tuple[str, str]:
        freeze = self._deps.pilot_git.tag_commit(PILOT_FREEZE_TAG)
        if freeze is None:
            raise PreconditionFailed(PC2, f"the tag {PILOT_FREEZE_TAG} does not exist")
        analysis = self._deps.pilot_git.analysis_commit(PILOT_RUN_ID, freeze)
        if analysis is None:
            raise PreconditionFailed(
                PC2, f"no commit descending from {PILOT_FREEZE_TAG} records the pilot analysis of {PILOT_RUN_ID}"
            )
        return freeze, analysis

    def _builder_check(self, binaries_dir: Path, export_dir: Path) -> BuilderCheck:
        recorded = {RECORDED_REPORT_ENV: str(export_dir / RECORDED_REPORT)}
        checking = self._deps.tools.open(binaries_dir, recorded)
        return run_builder_check(checking.alignment_driver, checking.builders, export_dir)

    def _bindings(self) -> dict[str, str]:
        if not self._deps.pilot_reader.exists(PILOT_BINDINGS_FILE):
            raise PreconditionFailed(PC2, f"the pilot run lacks {PILOT_BINDINGS_FILE}")
        return parse_bindings(self._deps.pilot_reader.read_text(PILOT_BINDINGS_FILE))
