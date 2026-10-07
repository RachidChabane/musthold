"""A hand-written `decisions.json` document with every cell the report and the derivation read."""

from __future__ import annotations

from collections.abc import Mapping, Sequence

from calibration.analysis.lint import expected_band_rows
from calibration.domain.canonical import JsonValue
from calibration.domain.models import MODEL_BY_ID

TAUS = (0.05, 0.10, 0.15, 0.20, 0.25, 0.30, 0.40, 0.50)
ENDPOINT_DIGEST = "d" * 64
OLLAMA_DIGEST = "e" * 64


def estimate(point: float, lo: float | None = None, hi: float | None = None) -> dict[str, JsonValue]:
    return {
        "point": point,
        "lo": point if lo is None else lo,
        "hi": point if hi is None else hi,
        "method": "cluster bootstrap",
    }


def grid(miss_upper: Sequence[float], flag_point: Sequence[float], positives: int = 40) -> list[JsonValue]:
    return [
        {
            "tau": tau,
            "positives": positives,
            "rows": 400,
            "miss_rate": estimate(upper / 2, 0.0, upper),
            "flag_rate": estimate(flag),
        }
        for tau, upper, flag in zip(TAUS, miss_upper, flag_point, strict=True)
    ]


GOOD_MISS: tuple[float, ...] = (0.02, 0.04, 0.06, 0.08, 0.09, 0.12, 0.2, 0.4)
LOW_FLAG: tuple[float, ...] = (0.9, 0.8, 0.7, 0.6, 0.4, 0.3, 0.2, 0.1)


def source_text(model: str) -> str:
    spec = MODEL_BY_ID[model]
    return f"system-one:{spec.identifier}@{ENDPOINT_DIGEST}"


def cell(
    kind: str,
    source: str,
    model: str,
    *,
    band: str = "ROUTE_ONLY",
    miss_upper: Sequence[float] = GOOD_MISS,
    flag_point: Sequence[float] = LOW_FLAG,
    positives: int = 40,
    fit_decisions: int = 300,
    fit_clusters: int = 12,
    false_aligned: tuple[int, int] | None = None,
    capped_by: Sequence[str] = (),
    auroc_point: float = 0.7,
) -> dict[str, JsonValue]:
    metrics: dict[str, JsonValue] = {
        "auroc": estimate(auroc_point, auroc_point - 0.1, auroc_point + 0.1),
        "ece": estimate(0.08, 0.04, 0.12),
        "q09": estimate(0.8, 0.6, 0.9),
        "brier": estimate(0.2),
        "aurc": estimate(0.2),
        "c05": estimate(0.1),
        "c05u": 0.0,
        "accuracy": estimate(0.8),
        "screen_stratum": grid(miss_upper, flag_point, positives) if kind == "K4" else [],
        "screen_all": [],
        "false_aligned_counts": None
        if false_aligned is None
        else {"successes": false_aligned[0], "total": false_aligned[1]},
    }
    return {
        "kind": kind,
        "source": source,
        "model": model,
        "confidence_source": source_text(model) if source == "S1" else f"host-votes({model})",
        "counts": {"units": 100, "decisions": 100, "with_prediction": 100, "clusters": 10},
        "metrics": metrics,
        "source3": [],
        "admission": {"main": {"outcome": "UNREACHABLE", "fit_decisions": fit_decisions, "fit_clusters": fit_clusters}},
        "admission_outcome": "UNREACHABLE",
        "band": band,
        "band_reason": "fixture",
        "annotations": [],
        "capped_by": list(capped_by),
        "conclusion": f"{source} ({model}) on {kind}: fixture sentence.",
        "determinism": None,
        "sensitivity": {"primary": {"n": 100, "accuracy": estimate(0.8), "auroc": estimate(0.7), "ece": estimate(0.1)}},
    }


def decisions(
    overrides: Mapping[str, Mapping[str, JsonValue]] | None = None,
    protocol_scopes: Sequence[Sequence[str]] = (),
    switches: Mapping[str, bool] | None = None,
    comparisons: Mapping[str, JsonValue] | None = None,
) -> dict[str, JsonValue]:
    cells: dict[str, JsonValue] = {}
    for kind, source, model in sorted(expected_band_rows()):
        cells[f"{kind}|{source}|{model}"] = cell(kind, source, model)
    for key, replacement in (overrides or {}).items():
        cells[key] = dict(replacement)
    entries: list[JsonValue] = [
        {"kind": "protocol", "scope": list(scope), "text": "fixture"} for scope in protocol_scopes
    ]
    return {
        "run_id": "run-fixture",
        "seed": 20261007,
        "resamples": 200,
        "provenance": {"template_digests": {"alignment": "a" * 64, "discpower": "b" * 64}},
        "pilot_switches": dict(switches or {"SW2a": False, "SW3": False}),
        "cells": cells,
        "comparisons": dict(comparisons or {"source1_vs_source2": {}, "two_signal": {}, "model_sizes": {}}),
        "checks": {"per_model": {}, "keys": {m: source_text(m) for m in MODEL_BY_ID}, "key_collisions": []},
        "deviations": {
            "counts": {"provenance": 0, "pause": 0, "blocked": 0, "protocol": len(entries)},
            "entries": len(entries),
            "details": entries,
        },
        "safety": {
            "per_model": {m: {"deferrals": 0, "pressure_stops": 0, "blocked": False} for m in MODEL_BY_ID},
            "pauses": 0,
            "heavy_work_waits": 0,
        },
    }
