"""The CSV tables of `results/` derived from `decisions.json` (PREREG 15.2)."""

from __future__ import annotations

import csv
import io
from typing import Final

import pytest

from calibration.analysis.result_files import (
    COMPARISON_HEADER,
    MODEL_SIZE_FILE,
    SOURCE_COMPARISON_FILE,
    TWO_SIGNAL_FILE,
    result_files,
)
from calibration.domain.canonical import JsonValue
from calibration.domain.constants import CHECKS_FILE

DIFFERENCE: Final[dict[str, JsonValue]] = {"point": 0.1, "lo": -0.1, "hi": 0.3}
ACCURACY: Final[dict[str, JsonValue]] = {"first": 0.8, "second": 0.7, "difference": DIFFERENCE}
COMPARISON: Final[dict[str, JsonValue]] = {"shared": 5, "accuracy": ACCURACY, "auroc": {"first": None, "second": 0.5}}
DECISIONS: Final[dict[str, JsonValue]] = {
    "comparisons": {
        "source1_vs_source2": {"K3|M1": COMPARISON},
        "model_sizes": {"K3": {"M3-M2": "BLOCKED", "M4-M5": COMPARISON}},
        "two_signal": {
            "M1": {
                "threshold": 0.9,
                "tests": 12,
                "two_signal_aligned": 3,
                "false_aligned": {"point": 0.0, "lo": 0.0, "hi": 0.56},
                "kappa": None,
                "both_wrong": 1,
            }
        },
    },
    "checks": {
        "per_model": {
            "M1": {
                "spearman_confidence_vs_returned": {"K1": 0.9, "K2": None},
                "answers_without_confidence": 0,
                "formula_answers_checked": 112,
                "formula_findings": [{"request": "A|x"}, {"request": "A|y"}],
                "screen_regression": {"failed": True, "without_probability": 3},
            }
        }
    },
}


def _rows(text: str) -> list[list[str]]:
    return list(csv.reader(io.StringIO(text)))


@pytest.fixture(scope="module")
def files() -> dict[str, str]:
    return result_files(DECISIONS)


class TestResultFiles:
    def test_the_four_tables_are_produced(self, files: dict[str, str]) -> None:
        assert set(files) == {SOURCE_COMPARISON_FILE, MODEL_SIZE_FILE, TWO_SIGNAL_FILE, CHECKS_FILE}

    def test_a_document_without_comparisons_still_yields_header_only_tables(self) -> None:
        produced = result_files({})
        assert _rows(produced[SOURCE_COMPARISON_FILE]) == [list(COMPARISON_HEADER)]
        assert _rows(produced[CHECKS_FILE]) == [["model", "check", "kind", "value"]]

    def test_each_comparison_metric_with_a_difference_is_a_row(self, files: dict[str, str]) -> None:
        rows = _rows(files[SOURCE_COMPARISON_FILE])
        assert rows[0] == list(COMPARISON_HEADER)
        assert rows[1:] == [["K3|M1", "5", "accuracy", "0.8", "0.7", "0.1", "-0.1", "0.3"]]

    def test_model_size_rows_are_labelled_with_kind_and_pair_and_blocked_pairs_say_so(
        self, files: dict[str, str]
    ) -> None:
        rows = _rows(files[MODEL_SIZE_FILE])[1:]
        assert rows[0][0] == "K3 M3-M2"
        assert rows[0][1] == "BLOCKED"
        assert rows[1][:3] == ["K3 M4-M5", "5", "accuracy"]

    def test_the_two_signal_table_leaves_undefined_figures_empty(self, files: dict[str, str]) -> None:
        rows = _rows(files[TWO_SIGNAL_FILE])
        assert rows[1] == ["M1", "0.9", "12", "3", "0.0", "0.0", "0.56", "", "1"]

    def test_the_checks_table_has_one_row_per_check_of_a_model(self, files: dict[str, str]) -> None:
        rows = _rows(files[CHECKS_FILE])[1:]
        assert ["M1", "spearman_confidence_vs_returned", "K1", "0.9"] in rows
        assert ["M1", "spearman_confidence_vs_returned", "K2", ""] in rows
        assert ["M1", "formula_findings", "", "2"] in rows
        assert ["M1", "screen_regression_failed", "K4", "True"] in rows
        assert ["M1", "screen_without_probability", "K4", "3"] in rows
        assert ["M1", "formula_answers_checked", "", "112"] in rows
