"""Names of the ground-truth artifacts; read only by the pre-registration tools and analysis."""

from __future__ import annotations

from typing import Final

ORACLE_FILE_PATTERN: Final = r"test_oracle_park_(\d+)\.py$"
VIOLATIONS_DIRNAME: Final = "violations"
PLANTED_DIRNAME: Final = "planted"
REFACTOR_DIRNAME: Final = "refactor"
ORACLE_DIRNAME: Final = "oracle"
LABELS_FILENAME: Final = "labels.yaml"
ORACLE_FILE_GLOB: Final = "test_oracle_park_*.py"
ARM_P_LABELS_FILENAME: Final = "labels.yaml"
PLANTED_MAP_FILENAME: Final = "planted_map.json"
LABEL_MATRIX_FILENAME: Final = "label_matrix.csv"
