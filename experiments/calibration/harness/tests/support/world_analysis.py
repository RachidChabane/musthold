"""Runs `AnalyzeUseCase` over a synthetic world with fakes for everything outside the bytes."""

from __future__ import annotations

import json
from datetime import datetime
from pathlib import Path
from typing import Final

from calibration.adapters.pilot_matrix import SnapshotPilotMatrix
from calibration.adapters.pilot_snapshot import UnpackedSnapshot
from calibration.analysis.cell_metrics import MetricSettings
from calibration.analysis.pipeline import AnalysisOutput, AnalyzeDeps, AnalyzeSettings, AnalyzeUseCase
from calibration.domain.ports import PilotMatrix, ProductReader, TimestampReader, VersionControl
from pilot_layout import unpack_snapshot
from support.fake_ports import FakeTimestampReader, FakeVersionControl
from support.fake_product_reader import FakeProductReader
from support.synthetic_world import World

TEST_RESAMPLES: Final = 100
PILOT_SWITCHES: Final = {"SW2a": {"fired": False}, "SW3": {"fired": True}}


def snapshot_matrix(root: Path) -> SnapshotPilotMatrix:
    """The pilot's own matrix module, unpacked from the working tree below `root`."""
    return SnapshotPilotMatrix(UnpackedSnapshot(unpack_snapshot(root)))


def analysis_settings(world: World, resamples: int = TEST_RESAMPLES) -> AnalyzeSettings:
    return AnalyzeSettings(
        run_dir=world.run_dir,
        pilot_run_dir=world.pilot_run_dir,
        snapshot_prereg_dir=world.prereg_dir,
        freeze_receipt=world.freeze_receipt,
        metric=MetricSettings(resamples=resamples),
    )


def analysis_deps(
    matrix: PilotMatrix,
    *,
    reader: ProductReader | None = None,
    vcs: VersionControl | None = None,
    timestamps: TimestampReader | None = None,
) -> AnalyzeDeps:
    return AnalyzeDeps(
        reader=reader if reader is not None else FakeProductReader(),
        matrix=matrix,
        vcs=vcs if vcs is not None else FakeVersionControl(blob_text=json.dumps(PILOT_SWITCHES).encode()),
        timestamps=timestamps if timestamps is not None else FakeTimestampReader(),
    )


def analyze_world(
    world: World, matrix: PilotMatrix, *, resamples: int = TEST_RESAMPLES, generated_at: datetime | None = None
) -> AnalysisOutput:
    timestamps = FakeTimestampReader(generated_at) if generated_at is not None else None
    return AnalyzeUseCase(analysis_settings(world, resamples), analysis_deps(matrix, timestamps=timestamps)).run()
