"""Source (1) for one model: its recorded answers read twice (pass 1, pass 2) into cells, checks and determinism."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass, field

from calibration.analysis.checks import (
    ConfidenceChecks,
    FormulaFinding,
    ScreenRegression,
    confidence_checks,
    formula_checks,
    screen_regression,
)
from calibration.analysis.determinism import DeterminismFigures, answers_identical, determinism_of
from calibration.analysis.model import ALIGNMENT_KINDS, CellData, Decision, Kind
from calibration.analysis.replies import ReplyOutcome, read_outcomes
from calibration.analysis.run_data import RunData
from calibration.analysis.source1 import (
    QUESTION_COVERAGE,
    QUESTION_CRITERION,
    ScreenPairs,
    alignment_cells,
    screen_pairs,
)
from calibration.analysis.truth import PilotTruth
from calibration.domain.constants import KEV_PORT, LOOPBACK, OLLAMA_PORT, SYSTEM_ONE_ROUTE
from calibration.domain.items import Item
from calibration.domain.models import ModelSpec
from calibration.domain.ports import ProductReader
from calibration.domain.schedule import Pass

ALIGNMENT_QUESTIONS: dict[Kind, tuple[str, ...]] = {
    Kind.K1: (QUESTION_CRITERION,),
    Kind.K2: (QUESTION_COVERAGE,),
    Kind.K3: (QUESTION_CRITERION, QUESTION_COVERAGE),
}


def experiment_address(model: ModelSpec) -> str:
    port = OLLAMA_PORT if model.is_ollama else KEV_PORT
    return f"http://{LOOPBACK}:{port}{SYSTEM_ONE_ROUTE}"


@dataclass(frozen=True)
class ModelAnalysis:
    model: ModelSpec
    confidence_source: str
    cells: Mapping[Kind, CellData]
    pass2_cells: Mapping[Kind, CellData]
    determinism: Mapping[Kind, DeterminismFigures]
    confidence: ConfidenceChecks
    formula_checked: int
    formula_findings: tuple[FormulaFinding, ...]
    regression: ScreenRegression
    without_oracle: tuple[str, ...]
    prefix_cache_hits: int = 0
    outcomes: Mapping[str, ReplyOutcome] = field(default_factory=dict[str, ReplyOutcome])


def _reported_model(model: ModelSpec, outcomes: Mapping[str, ReplyOutcome]) -> str:
    """The model the replies name; Kev echoes the request's identifier, Ollama names the tag."""
    for outcome in outcomes.values():
        if outcome.answered and outcome.model_reported:
            return outcome.model_reported
    return model.identifier


def _cells(
    model: ModelSpec,
    source: str,
    outcomes: Mapping[str, ReplyOutcome],
    items: Mapping[str, Item],
    truth: PilotTruth,
    reader: ProductReader,
) -> tuple[dict[Kind, CellData], ScreenPairs]:
    cells = alignment_cells(model.model_id, source, outcomes, items, truth)
    pairs = screen_pairs(model.model_id, source, outcomes, items, truth, reader)
    cells[Kind.K4] = pairs.cell
    return cells, pairs


def _identical_flags(
    kind: Kind, first: Mapping[str, ReplyOutcome], second: Mapping[str, ReplyOutcome], items: Mapping[str, Item]
) -> dict[str, bool]:
    flags: dict[str, bool] = {}
    for request_id, one in first.items():
        two = second.get(request_id)
        item = items[request_id]
        if two is None or not one.answered or not two.answered:
            continue
        if kind is Kind.K4 and item.question_to_ref is not None:
            for question, ref in item.question_to_ref.items():
                flags[f"{item.unit}|{ref}"] = answers_identical(one.answers, two.answers, (question,))
        elif kind in ALIGNMENT_KINDS and item.nodeid is not None:
            flags[item.unit] = answers_identical(one.answers, two.answers, ALIGNMENT_QUESTIONS[kind])
    return flags


def _decisions(cell: CellData) -> dict[str, Decision]:
    return {decision.unit: decision for decision in cell.predicted}


def analyze_model(model: ModelSpec, run: RunData, truth: PilotTruth, reader: ProductReader) -> ModelAnalysis:
    """Reads both passes of a model, builds its cells and runs the confidence, formula, regression and determinism
    checks. A model whose answers were never recorded yields cells without decisions."""
    first = read_outcomes(run, model, Pass.ONE, reader)
    second = read_outcomes(run, model, Pass.TWO, reader)
    source = reader.endpoint_digest(experiment_address(model), _reported_model(model, first)).source
    cells, pairs = _cells(model, source, first, run.items, truth, reader)
    cells_two, _ = _cells(model, source, second, run.items, truth, reader)
    determinism = {
        kind: determinism_of(
            _decisions(cells[kind]), _decisions(cells_two[kind]), _identical_flags(kind, first, second, run.items)
        )
        for kind in cells
    }
    checked, findings = formula_checks(model, first, run.items)
    hits = sum(1 for outcome in first.values() if outcome.cached_hit)
    return ModelAnalysis(
        model=model,
        confidence_source=source,
        cells=cells,
        pass2_cells=cells_two,
        determinism=determinism,
        confidence=confidence_checks({kind: cells[kind] for kind in ALIGNMENT_KINDS}, first),
        formula_checked=checked,
        formula_findings=tuple(findings),
        regression=screen_regression(first, run.items, pairs.readings),
        without_oracle=pairs.without_oracle,
        prefix_cache_hits=hits,
        outcomes=first,
    )
