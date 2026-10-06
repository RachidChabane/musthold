"""Arm-P items on disk: edit-set bodies named by mutant id, and the verbatim planted map."""

from __future__ import annotations

import json
import re
from pathlib import Path

from pilot.domain.edits import EditSet
from pilot.domain.pipeline_constants import PLANTED_MAP_FILENAME

ITEM_NAME = re.compile(r"^[0-9a-f]{16}\.json$")


class DirectoryArmPItems:
    """Implements the `ArmPItems` port over `prereg/arm_p/items/`."""

    def __init__(self, directory: Path) -> None:
        self._directory = directory

    def items(self) -> dict[str, EditSet]:
        return {
            path.stem: EditSet.from_mapping(json.loads(path.read_text(encoding="utf-8")))
            for path in sorted(self._directory.iterdir())
            if ITEM_NAME.match(path.name)
        }

    def planted_map_text(self) -> str:
        return (self._directory / PLANTED_MAP_FILENAME).read_text(encoding="utf-8")
