from __future__ import annotations

import sys
from pathlib import Path

import pytest

from calibration.adapters.pilot_matrix import SnapshotPilotMatrix
from calibration.adapters.pilot_snapshot import UnpackedSnapshot
from calibration.domain.ports import MatrixSignature
from pilot_layout import unpack_snapshot

NODEIDS = ["spec", "over", "part", "off", "inert", "undefined"]
DECLARED = {"spec": 1, "over": 1, "part": 1, "off": 1, "inert": 1, "undefined": 3}
VALID_POOL: list[tuple[int, frozenset[str]]] = [
    (1, frozenset({"spec", "over", "part"})),
    (1, frozenset({"spec", "over"})),
    (1, frozenset({"spec", "over"})),
    (1, frozenset({"spec", "over"})),
    (2, frozenset({"over", "off"})),
    (2, frozenset({"unrelated"})),
]
UNATTRIBUTED = [frozenset({"spec"}), frozenset[str]()]


@pytest.fixture
def matrix(tmp_path: Path) -> SnapshotPilotMatrix:
    return SnapshotPilotMatrix(UnpackedSnapshot(unpack_snapshot(tmp_path / "snapshot")))


def test_classify_returns_the_signature_and_verdict_of_the_pilots_matrix(matrix: SnapshotPilotMatrix) -> None:
    signatures = matrix.classify(NODEIDS, DECLARED, VALID_POOL, UNATTRIBUTED)
    assert signatures == {
        "spec": MatrixSignature("spec", 1, "SPECIFIC", "ALIGNED", 0.5),
        "over": MatrixSignature("over", 1, "OVERBROAD", "ALIGNED", 0.0),
        "part": MatrixSignature("part", 1, "PARTIAL", "MISALIGNED", 0.0),
        "off": MatrixSignature("off", 1, "OFF_TARGET", "MISALIGNED", 0.0),
        "inert": MatrixSignature("inert", 1, "INERT", "MISALIGNED", 0.0),
        "undefined": MatrixSignature("undefined", 3, "UNDEFINED_OWN", "UNDETERMINED", 0.0),
    }


def test_classify_covers_only_the_requested_tests(matrix: SnapshotPilotMatrix) -> None:
    signatures = matrix.classify(["spec"], DECLARED, VALID_POOL, UNATTRIBUTED)
    assert list(signatures) == ["spec"]


def test_a_test_with_no_unattributed_mutants_has_no_none_share(matrix: SnapshotPilotMatrix) -> None:
    signatures = matrix.classify(["spec"], DECLARED, VALID_POOL, [])
    assert signatures["spec"].none_share == 0.0


def test_building_the_matrix_leaves_no_pilot_module_loaded(matrix: SnapshotPilotMatrix) -> None:
    matrix.classify(NODEIDS, DECLARED, VALID_POOL, UNATTRIBUTED)
    assert not [name for name in sys.modules if name == "pilot" or name.startswith("pilot.")]
