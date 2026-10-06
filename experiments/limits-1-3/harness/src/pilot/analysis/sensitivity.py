"""Sensitivity analyses of PREREG 13: each re-evaluates endpoints under one changed input."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass

from pilot.analysis.endpoints import (
    MatrixNulls,
    cross_check_endpoint,
    gate_endpoints,
    judge_endpoint,
    judge_verdicts,
    matrix_endpoint,
    matrix_signatures,
)
from pilot.analysis.model import Context
from pilot.analysis.results import EndpointResult
from pilot.analysis.scenarios import GateVariant, Scenario, TruthKind
from pilot.analysis.settings import AnalysisSettings
from pilot.analysis.tables import consensus_tests
from pilot.domain import analysis_constants as k
from pilot.domain.gate import G3, G3_LENIENT, G3_NOISO, G3_PREREG, G3_STRICT


@dataclass(frozen=True)
class SensitivityRow:
    variant: str
    primary: EndpointResult
    result: EndpointResult

    def to_mapping(self) -> dict[str, object]:
        return {
            "variant": self.variant,
            "endpoint": self.result.id,
            "primary": _figure(self.primary),
            "variant_result": _figure(self.result),
        }


def _figure(result: EndpointResult) -> dict[str, object]:
    return {"n": result.n, "point": result.point, "lo": result.lo, "hi": result.hi, "outcome": result.outcome.value}


def sensitivity_rows(
    ctx: Context,
    scenarios: Sequence[Scenario],
    base: Mapping[str, EndpointResult],
    signatures_nulls: MatrixNulls,
    settings: AnalysisSettings,
) -> list[SensitivityRow]:
    rows: list[SensitivityRow] = []
    gate_variants: list[tuple[str, GateVariant, TruthKind]] = [
        ("S1", GateVariant("G3-S1", G3, use_s1=True), TruthKind.ORACLE),
        (G3_PREREG.label, GateVariant(G3_PREREG.name, G3_PREREG), TruthKind.ORACLE),
        (G3_NOISO.label, GateVariant(G3_NOISO.name, G3_NOISO), TruthKind.ORACLE),
        ("label-based truth", GateVariant("G3", G3), TruthKind.LABEL),
        (G3_STRICT.label, GateVariant(G3_STRICT.name, G3_STRICT), TruthKind.ORACLE),
        (G3_LENIENT.label, GateVariant(G3_LENIENT.name, G3_LENIENT), TruthKind.ORACLE),
        ("J-X", GateVariant("G3 with J-X", G3, judge=k.JUDGE_X), TruthKind.ORACLE),
    ]
    for name, variant, truth in gate_variants:
        rows += [SensitivityRow(name, base[r.id], r) for r in gate_endpoints(ctx, scenarios, variant, truth, settings)]
    signatures = matrix_signatures(ctx)
    consensus = consensus_tests(ctx)
    masked = judge_verdicts(ctx, k.JUDGE_H, k.ALIGNMENT_ARM_MASKED, False)
    if consensus:
        rows += [
            SensitivityRow("CONSENSUS", base["D9"], judge_endpoint(ctx, masked, settings, consensus)),
            SensitivityRow("CONSENSUS", base["D10"], matrix_endpoint(ctx, signatures, signatures_nulls, settings, consensus)),
            SensitivityRow("CONSENSUS", base["D11"], cross_check_endpoint(ctx, masked, signatures, settings, consensus)),
        ]
    s1 = judge_verdicts(ctx, k.JUDGE_H, k.ALIGNMENT_ARM_MASKED, True)
    rows += [
        SensitivityRow("S1", base["D9"], judge_endpoint(ctx, s1, settings, label="J-H NAME-MASKED S1")),
        SensitivityRow("S1", base["D11"], cross_check_endpoint(ctx, s1, signatures, settings, label="J-H S1")),
    ]
    gpt = judge_verdicts(ctx, k.JUDGE_X, k.ALIGNMENT_ARM_MASKED, False)
    rows += [
        SensitivityRow("J-X", base["D9"], judge_endpoint(ctx, gpt, settings, label="J-X NAME-MASKED")),
        SensitivityRow("J-X", base["D11"], cross_check_endpoint(ctx, gpt, signatures, settings, label="J-X")),
    ]
    return rows
