"""`analyze`: every estimand of ADR 0007 over a set of trunk records, as one JSON-ready structure.

All figures are descriptive and labelled with their sample. Harnesses are never pooled: each
harness is analysed on its own trunks, and the harness x arm interaction is a difference of two
per-harness estimates over the same repository resamples.
"""

from __future__ import annotations

import math
from collections import defaultdict
from collections.abc import Sequence
from dataclasses import dataclass
from typing import Final

from specbench.analysis import blind_spot, calibration, composition, judge, outcomes
from specbench.analysis.bootstrap import CONFIDENCE_LEVEL, ClusterTable, Statistic, wilson
from specbench.analysis.estimate import AnalysisConfig, Estimator, total
from specbench.analysis.outcomes import MODES, Mode
from specbench.analysis.selection_null import DEFAULT_PERMUTATIONS, selection_null
from specbench.domain.arms import ACCEPTANCE_ARMS, FROZEN_MODEL, Arm, Harness
from specbench.domain.canonical import JsonValue
from specbench.domain.records import ConfidenceSource, MutantSource, TrunkRecord
from specbench.domain.reporting import DESCRIPTIVE_LABEL

RESULTS_SCHEMA: Final = "specbench.results/1"
INTERVAL_METHOD: Final = "percentile cluster bootstrap by repository"
Record = dict[str, JsonValue]


@dataclass(frozen=True)
class Context:
    table: ClusterTable
    estimator: Estimator
    harness: str

    def est(self, statistic: Statistic) -> Record:
        return self.estimator(statistic).to_record()

    def count(self, *parts: str) -> int:
        return total(self.table, "|".join([self.harness, *parts]))


def build_table(trunks: Sequence[TrunkRecord]) -> ClusterTable:
    """One cluster per repository; each holds the exact sum of every module's contributions.

    Sums use `math.fsum`, so the totals do not depend on the order in which trunks are read.
    """
    amounts: defaultdict[str, defaultdict[str, list[float]]] = defaultdict(lambda: defaultdict(list))
    for trunk in trunks:
        for contribution in (
            outcomes.trunk_counts(trunk),
            blind_spot.trunk_counts(trunk),
            judge.trunk_counts(trunk),
            calibration.trunk_counts(trunk),
        ):
            for name, amount in contribution.items():
                amounts[trunk.repository][name].append(amount)
    clusters = {
        repository: {name: math.fsum(values) for name, values in per.items()} for repository, per in amounts.items()
    }
    names = sorted({name for counts in clusters.values() for name in counts})
    return ClusterTable.build(names, clusters)


def _proportion(successes: int, count: int) -> Record:
    interval = wilson(successes, count)
    return {"hi": interval.hi, "lo": interval.lo, "n": count, "successes": successes}


def _arm_section(ctx: Context, arm: Arm) -> Record:
    h = ctx.harness
    standard = Mode.STANDARD
    modes: Record = {mode.value: ctx.est(outcomes.x_loop(h, arm, mode)) for mode in MODES}
    primary = outcomes.x_loop(h, arm, standard)
    full = outcomes.x_loop(h, arm, standard, full_truth=True)
    return {
        "accepted": ctx.count(arm.value, standard.value, "accepted"),
        "accepted_defective": ctx.count(arm.value, standard.value, "accepted_defective"),
        "calls_per_trunk": ctx.est(outcomes.cost_per_trunk(h, arm, "calls")),
        "clean_trunks": ctx.count(arm.value, "base_clean"),
        "defect_rate_accepted": ctx.est(outcomes.defect_rate_at(h, arm, standard)),
        "defective_trunks": ctx.count(arm.value, "base_defective"),
        "escaped_defects": ctx.est(outcomes.escaped_defects(h, arm, standard)),
        "false_rejection": ctx.est(outcomes.false_rejection(h, arm, standard)),
        "model_truth_inflation": ctx.est(outcomes.difference(full, primary)),
        "o_defect_rate_accepted": ctx.est(outcomes.accepted_o_rate(h, arm, standard)),
        "seconds_per_trunk": ctx.est(outcomes.cost_per_trunk(h, arm, "seconds")),
        "selection": {
            "sensitivity": ctx.est(outcomes.sensitivity(h, arm)),
            "specificity": ctx.est(outcomes.specificity(h, arm)),
            "x_sel": ctx.est(outcomes.x_selection(h, arm)),
        },
        "trunks": ctx.count(arm.value, "base_n"),
        "x_loop": modes,
        "x_loop_full_truth": ctx.est(full),
        "x_loop_full_truth_unexposed": ctx.est(outcomes.x_loop_unexposed(h, arm, standard)),
        "yield": ctx.est(outcomes.yield_rate(h, arm, standard)),
    }


def _placebo(ctx: Context) -> Record | None:
    if not (total(ctx.table, f"{ctx.harness}|ALL|base_n") and total(ctx.table, f"{ctx.harness}|PL|base_n")):
        return None
    primary = Mode.STANDARD
    return {
        "x_all_minus_x_pl": ctx.est(
            outcomes.difference(
                outcomes.x_loop(ctx.harness, Arm.ALL, primary), outcomes.x_loop(ctx.harness, Arm.PL, primary)
            )
        )
    }


def _cost(ctx: Context) -> Record | None:
    if not (total(ctx.table, f"{ctx.harness}|ALL|base_n") and total(ctx.table, f"{ctx.harness}|T|base_n")):
        return None
    record: Record = {}
    for quantity in ("calls", "seconds"):
        record[f"extra_{quantity}_per_accepted_change"] = ctx.est(
            outcomes.difference(
                outcomes.cost_per_accepted(ctx.harness, Arm.ALL, Mode.STANDARD, quantity),
                outcomes.cost_per_accepted(ctx.harness, Arm.T, Mode.STANDARD, quantity),
            )
        )
    return record


def _selection_nulls(
    trunks: Sequence[TrunkRecord], harness: Harness, config: AnalysisConfig, permutations: int
) -> Record:
    nulls: Record = {}
    for arm in ACCEPTANCE_ARMS:
        null = selection_null(trunks, harness, arm, permutations, config.seed)
        if null.permutations:
            nulls[arm.value] = {"hi": null.hi, "lo": null.lo, "mean": null.mean, "permutations": null.permutations}
    return nulls


def _blind_spot(ctx: Context) -> Record:
    record: Record = {}
    for variant in blind_spot.VARIANTS:
        tasks = ctx.count("sb", variant.value, "tasks")
        if tasks == 0:
            continue
        cells: Record = {
            f"{writer.value}|{source.value}": ctx.est(blind_spot.cell_rate(ctx.harness, variant, writer, source))
            for writer in blind_spot.CELL_WRITERS
            for source in (variant, MutantSource.OPERATOR)
        }
        record[variant.value] = {
            "cells": cells,
            "sb_coder": ctx.est(blind_spot.sb_coder(ctx.harness, variant)),
            "tasks": tasks,
        }
    return record


def _judge(ctx: Context) -> Record | None:
    n = ctx.count("judge", "n")
    if n == 0:
        return None
    metrics = {
        "sensitivity": judge.sensitivity,
        "specificity": judge.specificity,
        "false_rejection": judge.false_rejection,
        "x_sel": judge.x_selection,
    }
    record: Record = {"snapshots": n}
    for name, build in metrics.items():
        record[name] = {
            "delta_replay_minus_same": ctx.est(
                outcomes.difference(build(ctx.harness, judge.Judge.REPLAY), build(ctx.harness, judge.Judge.SAME_MODEL))
            ),
            "replay": ctx.est(build(ctx.harness, judge.Judge.REPLAY)),
            "same_model": ctx.est(build(ctx.harness, judge.Judge.SAME_MODEL)),
        }
    return record


def _calibration(ctx: Context) -> Record:
    decisions = ctx.count("cal", "decisions")
    record: Record = {"decisions": decisions, "undetermined_rate": ctx.est(calibration.undetermined_rate(ctx.harness))}
    for source in calibration.ANSWERING_SOURCES:
        answered = ctx.count("cal", source.value, "n")
        if answered == 0:
            continue
        reliability = [
            {
                "bin": index,
                "mean_confidence": _bin_mean(ctx, source, index, "p"),
                "n": ctx.count("cal", source.value, "rel", str(index), "n"),
                "observed": _bin_mean(ctx, source, index, "truth"),
            }
            for index in range(calibration.RELIABILITY_BINS)
        ]
        record[source.value] = {
            "brier": ctx.est(calibration.brier(ctx.harness, source)),
            "coverage_at_5pct_error": ctx.est(calibration.coverage_at_error(ctx.harness, source)),
            "decisions": answered,
            "ece": ctx.est(calibration.ece(ctx.harness, source)),
            "reliability": reliability,
        }
    return record


def _bin_mean(ctx: Context, source: ConfidenceSource, index: int, field: str) -> float | None:
    n = ctx.count("cal", source.value, "rel", str(index), "n")
    if n == 0:
        return None
    return total(ctx.table, "|".join([ctx.harness, "cal", source.value, "rel", str(index), field])) / n


def _composition(trunks: Sequence[TrunkRecord], harness: Harness) -> Record:
    return {
        "composition": {
            s.module.value: {
                "criteria": s.criteria,
                "mismatch_rate": _proportion(s.mismatches, s.criteria),
                "mismatches": s.mismatches,
            }
            for s in composition.composition_summary(trunks, harness)
        },
        "absent_modules": {
            f"{s.arm.value}|{s.module.value}": {"counted_as_passed": s.counted_as_passed, "probes": s.probes}
            for s in composition.absent_module_summary(trunks, harness)
        },
    }


def _harness_section(
    trunks: Sequence[TrunkRecord], estimator: Estimator, harness: Harness, permutations: int
) -> Record:
    ctx = Context(estimator.table, estimator, harness.value)
    config = estimator.config
    own = [t for t in trunks if t.harness is harness]
    present = [arm for arm in ACCEPTANCE_ARMS if ctx.count(arm.value, "base_n") > 0]
    section: Record = {
        "arms": {arm.value: _arm_section(ctx, arm) for arm in present},
        "blind_spot": _blind_spot(ctx),
        "calibration": _calibration(ctx),
        "cost": _cost(ctx),
        "judge_replay": _judge(ctx),
        "model": FROZEN_MODEL[harness],
        "placebo": _placebo(ctx),
        "repositories": len({t.repository for t in own}),
        "selection_null": _selection_nulls(trunks, harness, config, permutations),
        "tasks": sorted({t.task_id for t in own}),
        "trunks": {
            "reached_s_t": ctx.count("n"),
            "s_n_defective": _proportion(ctx.count("sn_defective"), ctx.count("trunks")),
            "s_t_defective": _proportion(ctx.count("defective"), ctx.count("n")),
            "total": ctx.count("trunks"),
            "unreached": ctx.count("unreached"),
        },
    }
    section.update(_composition(trunks, harness))
    return section


def _interaction(estimator: Estimator, harnesses: Sequence[Harness]) -> Record | None:
    table = estimator.table
    if len(harnesses) < 2:
        return None
    first, second = harnesses[0], harnesses[1]
    record: Record = {}
    for arm in ACCEPTANCE_ARMS:
        if total(table, f"{first.value}|{arm.value}|base_n") and total(table, f"{second.value}|{arm.value}|base_n"):
            statistic = outcomes.difference(
                outcomes.x_loop(first.value, arm, Mode.STANDARD), outcomes.x_loop(second.value, arm, Mode.STANDARD)
            )
            record[arm.value] = estimator(statistic).to_record()
    return {"minus": second.value, "of": first.value, "x_loop_difference": record}


def analyze(
    trunks: Sequence[TrunkRecord], config: AnalysisConfig | None = None, permutations: int = DEFAULT_PERMUTATIONS
) -> Record:
    """The descriptive results of a sample: per-harness sections, the interaction, and the sample's description."""
    if not trunks:
        raise ValueError("there is no trunk record to analyse")
    config = config or AnalysisConfig()
    estimator = Estimator(build_table(trunks), config)
    harnesses = sorted({t.harness for t in trunks}, key=lambda h: h.value)
    return {
        "harnesses": {h.value: _harness_section(trunks, estimator, h, permutations) for h in harnesses},
        "interaction": _interaction(estimator, harnesses),
        "label": DESCRIPTIVE_LABEL,
        "method": {
            "interval": INTERVAL_METHOD,
            "level": CONFIDENCE_LEVEL,
            "permutations": permutations,
            "resamples": config.resamples,
            "seed": config.seed,
        },
        "sample": {
            "languages": sorted({t.language.value for t in trunks}),
            "repositories": len({t.repository for t in trunks}),
            "tasks": len({t.task_id for t in trunks}),
            "trunks": len(trunks),
        },
        "schema": RESULTS_SCHEMA,
    }
