"""A small deterministic world: a pilot snapshot, a pilot run-02 directory and a calibration run directory."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Final

from calibration.domain.items import Item
from support.world_items import planned_items
from support.world_pilot import PILOT_RUN_ID, write_pilot_run, write_snapshot_prereg
from support.world_run import ANALYSIS_COMMIT, RUN_ID, WorldSpec, write_run

__all__ = ["World", "WorldSpec", "build_world"]

PREREG_DIR_NAME: Final = "prereg"
PILOT_DIR_NAME: Final = "pilot"
RUNS_DIR_NAME: Final = "runs"
RECEIPT_NAME: Final = "prereg-commit.tsr"


@dataclass(frozen=True)
class World:
    """The paths of a built world, the spec it was built from and its items."""

    root: Path
    prereg_dir: Path
    pilot_run_dir: Path
    run_dir: Path
    freeze_receipt: Path
    analysis_commit: str
    spec: WorldSpec
    items: tuple[Item, ...]


def build_world(root: Path, spec: WorldSpec | None = None) -> World:
    """Writes the world below `root`: `prereg/` (the snapshot's labels), `pilot/run-02/` and `runs/run-01/`."""
    chosen = spec if spec is not None else WorldSpec()
    prereg_dir = root / PREREG_DIR_NAME
    pilot_run_dir = root / PILOT_DIR_NAME / PILOT_RUN_ID
    run_dir = root / RUNS_DIR_NAME / RUN_ID
    write_snapshot_prereg(prereg_dir)
    write_pilot_run(pilot_run_dir)
    write_run(run_dir, chosen)
    receipt = root / RECEIPT_NAME
    receipt.write_bytes(b"receipt")
    return World(
        root=root,
        prereg_dir=prereg_dir,
        pilot_run_dir=pilot_run_dir,
        run_dir=run_dir,
        freeze_receipt=receipt,
        analysis_commit=ANALYSIS_COMMIT,
        spec=chosen,
        items=tuple(planned.item for planned in planned_items()),
    )
