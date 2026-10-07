"""`analyze` (§1, §15.2): every figure of `results/` recomputed from the recorded bytes, with no network call."""

from __future__ import annotations

import json
import math
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from calibration.analysis.cell_analyzer import CellOutcome, analyze_cell
from calibration.analysis.cell_metrics import MetricSettings
from calibration.analysis.checks import KeyCheck, key_check
from calibration.analysis.comparisons import (
    TwoSignalInputs,
    compare_cells,
    model_size_comparisons,
    two_signal,
)
from calibration.analysis.model import CellData, Kind
from calibration.analysis.model_analysis import ModelAnalysis, analyze_model
from calibration.analysis.preconditions import check_preconditions, qualification
from calibration.analysis.result_files import result_files
from calibration.analysis.run_data import RunData, read_run
from calibration.analysis.safety_summary import safety_summary
from calibration.analysis.source2 import source2_cells
from calibration.analysis.truth import PilotTruth, read_pilot_truth
from calibration.domain.canonical import JsonValue
from calibration.domain.constants import (
    ENCODING,
    GATE_THRESHOLD_JH5,
    GATE_THRESHOLD_S1,
    PILOT_DECISIONS_FILE,
    PILOT_INPUTS_FILE,
    SEED,
)
from calibration.domain.deviations import (
    DeviationEntry,
    DeviationKind,
    counts_by_kind,
    standing_of,
)
from calibration.domain.jsonread import json_object, mapping
from calibration.domain.lock import PinStatus
from calibration.domain.models import MODELS, ModelSpec
from calibration.domain.pilot_records import criterion_number
from calibration.domain.pilot_switches import fired_switches
from calibration.domain.ports import MatrixSignature, PilotMatrix, ProductReader, TimestampReader, VersionControl
from calibration.domain.sources import SourceId

ITEMS_MISSING: Final = "the items of this kind are missing"
NO_ORACLE: Final = "the pilot run holds no oracle record"
BLOCKED_ENTRY: Final = "a blocked entry is in scope"
PROVENANCE_KEYS: Final = (
    "prereg_commit",
    "harness_commit",
    "pilot",
    "snapshot_digest",
    "builders",
    "template_digests",
    "source2_prompt_digests",
    "counts",
)


@dataclass(frozen=True)
class AnalyzeSettings:
    run_dir: Path
    pilot_run_dir: Path
    snapshot_prereg_dir: Path
    freeze_receipt: Path
    metric: MetricSettings


@dataclass(frozen=True)
class AnalyzeDeps:
    reader: ProductReader
    matrix: PilotMatrix
    vcs: VersionControl
    timestamps: TimestampReader


@dataclass(frozen=True)
class AnalysisOutput:
    """`decisions.json` as a document, the other files of `results/` by relative path, and the deviation entries the
    checks added to the log."""

    decisions: dict[str, JsonValue]
    files: dict[str, str]
    new_deviations: tuple[DeviationEntry, ...]
    cell_outcomes: Mapping[str, CellOutcome]


def _blocked_cell(kind: Kind, source: SourceId, model: str, reason: str) -> CellData:
    return CellData(kind, source, model, None, (), 0, blocked_reason=reason)


class AnalyzeUseCase:
    def __init__(self, settings: AnalyzeSettings, deps: AnalyzeDeps) -> None:
        self._settings = settings
        self._deps = deps

    def run(self) -> AnalysisOutput:
        run = read_run(self._settings.run_dir)
        truth = read_pilot_truth(self._settings.snapshot_prereg_dir, self._settings.pilot_run_dir)
        pilot_inputs = json_object(
            json.loads((self._settings.run_dir / PILOT_INPUTS_FILE).read_text(encoding=ENCODING)), PILOT_INPUTS_FILE
        )
        checked = check_preconditions(
            run,
            truth,
            self._deps.vcs,
            self._deps.timestamps,
            self._settings.freeze_receipt,
            self._settings.pilot_run_dir,
            pilot_inputs,
        )
        entries = list(run.deviations)
        models = self._models(run, truth, entries)
        findings = self._regression_findings(models, entries)
        entries.extend(findings)
        cells = self._cells(run, truth, models, entries)
        outcomes = {cell.key: self._outcome(cell, models, entries) for cell in cells}
        comparisons = self._comparisons(run, truth, cells)
        decisions = self._document(run, truth, models, outcomes, comparisons, checked, entries)
        files = {path: text for outcome in outcomes.values() for path, text in outcome.files.items()}
        files.update(result_files(decisions))
        return AnalysisOutput(decisions, files, tuple(findings), outcomes)

    def _outcome(
        self, cell: CellData, models: Mapping[str, ModelAnalysis], entries: list[DeviationEntry]
    ) -> CellOutcome:
        analysis = models.get(cell.model) if cell.source is SourceId.S1 else None
        determinism = analysis.determinism.get(cell.kind) if analysis is not None else None
        pass2 = analysis.pass2_cells.get(cell.kind) if analysis is not None else None
        return analyze_cell(cell, entries, self._settings.metric, determinism, pass2)

    @staticmethod
    def _blocked_reason(run: RunData, model: ModelSpec, entries: list[DeviationEntry]) -> str | None:
        locked = run.lock.entry(model.model_id)
        if locked.status is PinStatus.BLOCKED:
            return locked.block_reason or BLOCKED_ENTRY
        if standing_of(entries, [model.model_id]).blocked:
            return BLOCKED_ENTRY
        return None

    def _models(self, run: RunData, truth: PilotTruth, entries: list[DeviationEntry]) -> dict[str, ModelAnalysis]:
        return {
            model.model_id: analyze_model(model, run, truth, self._deps.reader)
            for model in MODELS
            if self._blocked_reason(run, model, entries) is None
        }

    @staticmethod
    def _regression_findings(
        models: Mapping[str, ModelAnalysis], entries: list[DeviationEntry]
    ) -> list[DeviationEntry]:
        """A failed screen regression check is a `protocol` entry with K4 and the model as its scope (§7.4)."""
        findings: list[DeviationEntry] = []
        for model_id, analysis in models.items():
            if not analysis.regression.failed:
                continue
            text = (
                f"the screen regression check failed on {len(analysis.regression.mismatches)} answers"
                f" (monotone: {analysis.regression.monotone})"
            )
            entry = DeviationEntry.create(DeviationKind.PROTOCOL, [Kind.K4.value, model_id], text)
            if all(known.render() != entry.render() for known in entries):
                findings.append(entry)
        return findings

    def _k4_block(self, run: RunData, truth: PilotTruth) -> str | None:
        counts = run.manifest.get("counts")
        if isinstance(counts, dict) and counts.get("screen_blocked") is True:
            return ITEMS_MISSING
        if qualification(truth).with_oracle == 0:
            return NO_ORACLE
        return None

    def _cells(
        self, run: RunData, truth: PilotTruth, models: Mapping[str, ModelAnalysis], entries: list[DeviationEntry]
    ) -> list[CellData]:
        k4_block = self._k4_block(run, truth)
        cells: list[CellData] = []
        for model in MODELS:
            reason = self._blocked_reason(run, model, entries)
            for kind in Kind:
                if reason is not None or (kind is Kind.K4 and k4_block is not None):
                    cells.append(_blocked_cell(kind, SourceId.S1, model.model_id, reason or k4_block or BLOCKED_ENTRY))
                else:
                    cells.append(models[model.model_id].cells[kind])
        for cell in source2_cells(run.items, truth):
            if cell.kind is Kind.K4 and k4_block is not None:
                cells.append(_blocked_cell(cell.kind, cell.source, cell.model, k4_block))
            else:
                cells.append(cell)
        return cells

    def _comparisons(self, run: RunData, truth: PilotTruth, cells: list[CellData]) -> dict[str, JsonValue]:
        metric = self._settings.metric
        by_key = {cell.key: cell for cell in cells}
        measured = [cell for cell in cells if cell.source is SourceId.S1 and not cell.blocked_reason and cell.predicted]
        versus: dict[str, JsonValue] = {}
        for cell in measured:
            partner_source = SourceId.VH2 if cell.kind is Kind.K4 else SourceId.JH5
            partner = by_key.get(f"{cell.kind.value}|{partner_source.value}|host")
            if partner is not None and partner.predicted:
                versus[f"{cell.kind.value}|{cell.model}"] = compare_cells(cell, partner, metric)
        signatures = self._signatures(truth)
        inputs = TwoSignalInputs(signatures)
        simulated: dict[str, JsonValue] = {}
        for cell in cells:
            if cell.kind is not Kind.K3 or cell.blocked_reason or not cell.predicted:
                continue
            if cell.source is SourceId.S1:
                simulated[cell.model] = two_signal(cell, inputs, GATE_THRESHOLD_S1)
            elif cell.source is SourceId.JH5:
                simulated[SourceId.JH5.value] = two_signal(cell, inputs, GATE_THRESHOLD_JH5)
        sizes: dict[str, JsonValue] = {}
        for kind in Kind:
            per_model = {c.model: c for c in cells if c.source is SourceId.S1 and c.kind is kind}
            sizes[kind.value] = model_size_comparisons(per_model, metric)
        return {"source1_vs_source2": versus, "two_signal": simulated, "model_sizes": sizes}

    def _signatures(self, truth: PilotTruth) -> dict[str, MatrixSignature]:
        nodeids = sorted(label.nodeid for label in truth.labels)
        declared = {nodeid: criterion_number(truth.bindings[nodeid]) for nodeid in nodeids}
        return self._deps.matrix.classify(
            nodeids, declared, list(truth.matrix.valid_pool), list(truth.matrix.unattributed)
        )

    @staticmethod
    def _checks(models: Mapping[str, ModelAnalysis]) -> dict[str, JsonValue]:
        per_model: dict[str, JsonValue] = {}
        for model_id, analysis in models.items():
            per_model[model_id] = {
                "confidence_source": analysis.confidence_source,
                "spearman_confidence_vs_returned": {
                    kind.value: None if math.isnan(value) else value
                    for kind, value in analysis.confidence.spearman.items()
                },
                "answers_without_confidence": analysis.confidence.without_confidence,
                "formula_answers_checked": analysis.formula_checked,
                "formula_findings": [finding.to_record() for finding in analysis.formula_findings],
                "screen_regression": analysis.regression.to_record(),
                "prefix_cache_hits": analysis.prefix_cache_hits,
            }
        keys: KeyCheck = key_check({model_id: analysis.confidence_source for model_id, analysis in models.items()})
        return {
            "per_model": per_model,
            "keys": dict(keys.sources),
            "key_collisions": [list(group) for group in keys.collisions],
        }

    def _document(
        self,
        run: RunData,
        truth: PilotTruth,
        models: Mapping[str, ModelAnalysis],
        outcomes: Mapping[str, CellOutcome],
        comparisons: dict[str, JsonValue],
        checked: dict[str, JsonValue],
        entries: list[DeviationEntry],
    ) -> dict[str, JsonValue]:
        pilot = mapping(run.manifest.get("pilot"), "manifest.pilot")
        switches_text = self._deps.vcs.blob(str(pilot.get("analysis_commit")), PILOT_DECISIONS_FILE)
        return {
            "run_id": run.manifest.get("run_id"),
            "seed": SEED,
            "resamples": self._settings.metric.resamples,
            "provenance": {key: run.manifest.get(key) for key in PROVENANCE_KEYS}
            | {
                "product_export_digest": json_object(run.manifest.get("product_export"), "product_export").get("digest")
            },
            "lock": dict(run.lock.record),
            "preconditions": checked,
            "pilot_switches": dict(fired_switches(switches_text.decode(ENCODING) if switches_text else None)),
            "cells": {key: outcome.record for key, outcome in sorted(outcomes.items())},
            "comparisons": comparisons,
            "checks": self._checks(models),
            "excluded": {
                **dict(run.excluded),
                "without_oracle": sorted({m for a in models.values() for m in a.without_oracle}),
            },
            "deviations": {
                "counts": counts_by_kind(entries),
                "entries": len(entries),
                "details": [
                    {"kind": entry.kind.value, "scope": list(entry.scope), "text": entry.text} for entry in entries
                ],
            },
            "safety": safety_summary(self._settings.run_dir, entries),
            "timing": dict(run.timing),
            "canaries": {model_id: bool(record.get("passed")) for model_id, record in sorted(run.canaries.items())},
        }
