"""The CSV files of `results/` that are tables of `decisions.json`: comparisons and checks (§15.2)."""

from __future__ import annotations

import csv
import io
from collections.abc import Mapping, Sequence
from typing import Final

from calibration.domain.canonical import JsonValue
from calibration.domain.constants import CHECKS_FILE, COMPARISONS_DIR
from calibration.domain.jsonread import as_list, as_mapping

CSV_NEWLINE: Final = "\n"
METRICS: Final = ("accuracy", "auroc", "ece", "brier")
SOURCE_COMPARISON_FILE: Final = f"{COMPARISONS_DIR}/source1_vs_source2.csv"
MODEL_SIZE_FILE: Final = f"{COMPARISONS_DIR}/model_sizes.csv"
TWO_SIGNAL_FILE: Final = f"{COMPARISONS_DIR}/two_signal.csv"


def _csv(header: Sequence[str], rows: Sequence[Sequence[object]]) -> str:
    buffer = io.StringIO()
    writer = csv.writer(buffer, lineterminator=CSV_NEWLINE)
    writer.writerow(header)
    writer.writerows(rows)
    return buffer.getvalue()


def _value(value: object) -> object:
    return "" if value is None else value


def _comparison_rows(label: str, comparison: object) -> list[list[object]]:
    record = as_mapping(comparison)
    if record is None:
        return [[label, str(comparison), "", "", "", "", "", ""]]
    rows: list[list[object]] = []
    for metric in METRICS:
        entry = as_mapping(record.get(metric))
        difference = as_mapping(entry.get("difference")) if entry else None
        if entry is None or difference is None:
            continue
        rows.append(
            [
                label,
                record.get("shared"),
                metric,
                _value(entry.get("first")),
                _value(entry.get("second")),
                _value(difference.get("point")),
                _value(difference.get("lo")),
                _value(difference.get("hi")),
            ]
        )
    return rows


COMPARISON_HEADER: Final = ("pair", "shared", "metric", "first", "second", "difference", "lo", "hi")


def _two_signal_rows(simulated: Mapping[str, object]) -> list[list[object]]:
    rows: list[list[object]] = []
    for source, raw in sorted(simulated.items()):
        record = as_mapping(raw)
        if record is None:
            continue
        false_aligned = as_mapping(record.get("false_aligned")) or {}
        rows.append(
            [
                source,
                record.get("threshold"),
                record.get("tests"),
                record.get("two_signal_aligned"),
                _value(false_aligned.get("point")),
                _value(false_aligned.get("lo")),
                _value(false_aligned.get("hi")),
                _value(record.get("kappa")),
                record.get("both_wrong"),
            ]
        )
    return rows


def _check_rows(checks: Mapping[str, object]) -> list[list[object]]:
    per_model = as_mapping(checks.get("per_model")) or {}
    rows: list[list[object]] = []
    for model, raw in sorted(per_model.items()):
        record = as_mapping(raw) or {}
        spearman = as_mapping(record.get("spearman_confidence_vs_returned")) or {}
        for kind, value in sorted(spearman.items()):
            rows.append([model, "spearman_confidence_vs_returned", kind, _value(value)])
        rows.append([model, "answers_without_confidence", "", _value(record.get("answers_without_confidence"))])
        rows.append([model, "formula_answers_checked", "", _value(record.get("formula_answers_checked"))])
        findings = as_list(record.get("formula_findings"))
        rows.append([model, "formula_findings", "", len(findings) if findings is not None else 0])
        regression = as_mapping(record.get("screen_regression")) or {}
        rows.append([model, "screen_regression_failed", "K4", _value(regression.get("failed"))])
        rows.append([model, "screen_without_probability", "K4", _value(regression.get("without_probability"))])
    return rows


def result_files(decisions: Mapping[str, JsonValue]) -> dict[str, str]:
    """`comparisons/*.csv` and `checks.csv`, as text, from the tables of `decisions.json`."""
    comparisons = as_mapping(decisions.get("comparisons")) or {}
    versus = as_mapping(comparisons.get("source1_vs_source2")) or {}
    sizes = as_mapping(comparisons.get("model_sizes")) or {}
    size_rows = [
        row
        for kind, per_kind in sorted(sizes.items())
        for pair, comparison in sorted((as_mapping(per_kind) or {}).items())
        for row in _comparison_rows(f"{kind} {pair}", comparison)
    ]
    versus_rows = [row for key, comparison in sorted(versus.items()) for row in _comparison_rows(key, comparison)]
    return {
        SOURCE_COMPARISON_FILE: _csv(COMPARISON_HEADER, versus_rows),
        MODEL_SIZE_FILE: _csv(COMPARISON_HEADER, size_rows),
        TWO_SIGNAL_FILE: _csv(
            ("source", "threshold", "tests", "two_signal_aligned", "false_aligned", "lo", "hi", "kappa", "both_wrong"),
            _two_signal_rows(as_mapping(comparisons.get("two_signal")) or {}),
        ),
        CHECKS_FILE: _csv(("model", "check", "kind", "value"), _check_rows(as_mapping(decisions.get("checks")) or {})),
    }
