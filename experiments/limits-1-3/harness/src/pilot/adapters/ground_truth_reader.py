"""Reads the ground truth for the analysis: labels, planted items, label matrix, oracle results."""

from __future__ import annotations

import csv
import json
import re
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Any

import yaml

from pilot.adapters.prereg_files import load_labels
from pilot.domain.analysis_constants import (
    ARM_P_LABELS_YAML,
    ORACLE_CLASSES_FILENAME,
)
from pilot.domain.constants import LABEL_MATRIX_FILENAME, VERIFY_LOG_FILENAME
from pilot.domain.isolation_table import ISOLATION_TABLE_FILENAME
from pilot.domain.pipeline_constants import ARM_P_DIRNAME
from pilot.domain.run_records import GroundTruth, OracleRecord, PlantedItem

ENCODING = "utf-8"
_DIGITS = re.compile(r"(\d+)")


def _criterion_of(value: Any) -> int:
    if isinstance(value, int):
        return value
    match = _DIGITS.search(str(value))
    if match is None:
        raise ValueError(f"no criterion number in {value!r}")
    return int(match.group(1))


def read_planted(path: Path) -> dict[str, PlantedItem]:
    if not path.is_file():
        return {}
    document = yaml.safe_load(path.read_text(encoding=ENCODING)) or {}
    entries = (
        {e["mutant_id"]: e for e in document}
        if isinstance(document, list)
        else document
    )
    return {
        mutant_id: PlantedItem(mutant_id, str(raw["source"]), _criterion_of(raw["declared_target"]), str(raw["kind"]))
        for mutant_id, raw in entries.items()
    }


def read_label_matrix_rows(path: Path) -> dict[str, dict[str, str]]:
    with path.open(encoding=ENCODING, newline="") as handle:
        rows = list(csv.reader(handle))
    header = rows[0][1:]
    return {row[0]: dict(zip(header, row[1:], strict=True)) for row in rows[1:]}


def read_oracle_records(path: Path) -> dict[str, OracleRecord]:
    if not path.is_file():
        return {}
    document = json.loads(path.read_text(encoding=ENCODING))
    return {
        mutant_id: OracleRecord(
            mutant_id, raw["arm"], tuple(int(c) for c in raw["failed_criteria"]), raw.get("source", "executed")
        )
        for mutant_id, raw in document["mutants"].items()
    }


def read_ground_truth(
    prereg_dir: Path, run_dir: Path, crosscheck_differences: Sequence[str] | None
) -> GroundTruth:
    log_path = prereg_dir / VERIFY_LOG_FILENAME
    return GroundTruth(
        labels=load_labels(prereg_dir / "labels.yaml"),
        planted=read_planted(prereg_dir / ARM_P_DIRNAME / ARM_P_LABELS_YAML),
        oracle=read_oracle_records(run_dir / ORACLE_CLASSES_FILENAME),
        label_matrix_rows=read_label_matrix_rows(prereg_dir / LABEL_MATRIX_FILENAME),
        verify_log=log_path.read_text(encoding=ENCODING) if log_path.is_file() else "",
        crosscheck_differences=crosscheck_differences,
        d8_criteria=read_d8_criteria(prereg_dir / ISOLATION_TABLE_FILENAME),
    )


def read_d8_criteria(path: Path) -> tuple[int, ...]:
    """C_D8 of `prereg/isolation_table.json`; empty when the file is absent, which the analysis refuses."""
    if not path.is_file():
        return ()
    return tuple(int(number) for number in json.loads(path.read_text(encoding=ENCODING))["c_d8"])


def read_models_lock(path: Path) -> Mapping[str, Any]:
    return json.loads(path.read_text(encoding=ENCODING))
