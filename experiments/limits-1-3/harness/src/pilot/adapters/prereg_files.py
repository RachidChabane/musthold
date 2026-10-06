"""File adapters for the pre-registered artifacts: edit sets, labels, matrix, logs."""

from __future__ import annotations

import csv
import json
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Any

import yaml

from pilot.application.assemble_arm_p import ArmPItem
from pilot.domain.constants import REFACTOR_VARIANT_ID
from pilot.domain.edits import EditSet
from pilot.domain.ground_truth_paths import PLANTED_MAP_FILENAME
from pilot.domain.label_matrix import LabelMatrix
from pilot.domain.labels import TestLabel
from pilot.domain.variants import VariantId, VariantKind

NODEID_COLUMN = "nodeid"
_JSON_GLOB = "*.json"
_TEST_FILE_GLOB = "test_*.py"
_LABEL_COLLECTION_KEYS = ("tests", "labels")


def load_edit_sets(directory: Path, kind: VariantKind) -> dict[str, EditSet]:
    """Edit sets of one kind, keyed by variant id; files of another kind are rejected."""
    found: dict[str, EditSet] = {}
    for path in sorted(directory.glob(_JSON_GLOB)):
        variant = VariantId.parse(path.stem)
        if variant.kind is not kind:
            raise ValueError(f"{path.name} is a {variant.kind.value}, expected {kind.value}")
        found[variant.value] = EditSet.from_mapping(json.loads(path.read_text(encoding="utf-8")))
    return found


def load_refactor(directory: Path) -> dict[str, EditSet]:
    path = directory / f"{REFACTOR_VARIANT_ID}.json"
    return {REFACTOR_VARIANT_ID: EditSet.from_mapping(json.loads(path.read_text(encoding="utf-8")))}


def load_labels(path: Path) -> list[TestLabel]:
    document = yaml.safe_load(path.read_text(encoding="utf-8"))
    return [TestLabel.from_mapping(entry) for entry in _label_entries(document)]


def _label_entries(document: Any) -> list[Mapping[str, Any]]:
    if isinstance(document, list):
        return document
    if isinstance(document, Mapping):
        for key in _LABEL_COLLECTION_KEYS:
            if isinstance(document.get(key), list):
                return document[key]
        return [{"nodeid": nodeid, **fields} for nodeid, fields in document.items()]
    raise ValueError("labels file must hold a list or a mapping")


def write_label_matrix(path: Path, matrix: LabelMatrix) -> None:
    with path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle, lineterminator="\n")
        writer.writerow([NODEID_COLUMN, *matrix.variant_ids])
        for nodeid, cells in matrix.rows.items():
            writer.writerow([nodeid, *cells])


def read_label_matrix(path: Path) -> LabelMatrix:
    with path.open(encoding="utf-8", newline="") as handle:
        rows = list(csv.reader(handle))
    header, body = rows[0], rows[1:]
    return LabelMatrix(tuple(header[1:]), {row[0]: tuple(row[1:]) for row in body})


def write_lines(path: Path, lines: Sequence[str]) -> None:
    path.write_text("".join(f"{line}\n" for line in lines), encoding="utf-8")


class DirectoryTestSourceReader:
    """Implements the `TestSourceReader` port over a fixture's tests directory."""

    def __init__(self, tests_dir: Path, conftest_path: Path) -> None:
        self._tests_dir = tests_dir
        self._conftest_path = conftest_path

    def read(self) -> Mapping[str, str]:
        paths = sorted(self._tests_dir.glob(_TEST_FILE_GLOB))
        return {p.name: p.read_text(encoding="utf-8") for p in paths}

    def read_conftest(self) -> str | None:
        if not self._conftest_path.is_file():
            return None
        return self._conftest_path.read_text(encoding="utf-8")


def write_arm_p(items_dir: Path, labels_path: Path, items: Sequence[ArmPItem]) -> None:
    """Items as edit-set files named by mutant id, the label table and the planted map; deterministic."""
    items_dir.mkdir(parents=True, exist_ok=True)
    for stale in items_dir.glob(_JSON_GLOB):
        stale.unlink()
    for item in items:
        (items_dir / f"{item.mutant_id}.json").write_text(_dump(item.edit_set.to_mapping()), encoding="utf-8")
    (items_dir / PLANTED_MAP_FILENAME).write_text(_dump({i.mutant_id: i.source for i in items}), encoding="utf-8")
    labels = {
        i.mutant_id: {"source": i.source, "declared_target": i.declared_target, "kind": i.kind} for i in items
    }
    labels_path.write_text(yaml.safe_dump(labels, sort_keys=True), encoding="utf-8")


def _dump(value: Any) -> str:
    return json.dumps(value, indent=2, sort_keys=True) + "\n"
