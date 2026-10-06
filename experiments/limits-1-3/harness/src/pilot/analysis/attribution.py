"""Error attribution by substitution and error multiplication (PREREG 9.4)."""

from __future__ import annotations

from collections import Counter
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass

import numpy as np

from pilot.analysis.endpoints import judge_verdicts, matrix_signatures
from pilot.analysis.model import Context, EvidenceTruth
from pilot.analysis.scenarios import (
    GateVariant,
    Scenario,
    ScenarioOutcome,
    TruthKind,
    is_adequate,
    simulate,
    true_alignment,
)
from pilot.analysis.settings import AnalysisSettings
from pilot.analysis.stats import FloatArray, Interval, bootstrap_interval, safe_divide
from pilot.domain.alignment_aggregate import AlignmentVerdict
from pilot.domain.analysis_constants import (
    ALIGNMENT_ARM_MASKED,
    COMPONENT_GATE,
    COMPONENT_ISOLATION,
    COMPONENT_JUDGE,
    COMPONENT_VALIDITY,
    JUDGE_H,
    UNCOVERED_CRITERIA,
)
from pilot.domain.gate import GateOutcome

LINK_ALIGNMENT = "alignment"
LINK_VALIDITY = "validity"
LINK_ISOLATION = "isolation"
LINK_ALL = "all"
SUBSTITUTIONS = (LINK_ALIGNMENT, LINK_VALIDITY, LINK_ISOLATION, LINK_ALL)
LINK_COMPONENT = {
    LINK_ALIGNMENT: COMPONENT_JUDGE,
    LINK_VALIDITY: COMPONENT_VALIDITY,
    LINK_ISOLATION: COMPONENT_ISOLATION,
}

ErrorPredicate = Callable[[Scenario, GateOutcome, bool], bool]


def _is_uncovered_full(s: Scenario) -> bool:
    return s.is_full and s.criterion in UNCOVERED_CRITERIA


ERROR_PREDICATES: Mapping[str, ErrorPredicate] = {
    "D1": lambda s, o, adequate: _is_uncovered_full(s) and o is GateOutcome.PASS,
    "D2": lambda s, o, adequate: s.is_full and adequate and o is GateOutcome.BLOCK,
    "D3": lambda s, o, adequate: not adequate and o is GateOutcome.PASS,
    "D4": lambda s, o, adequate: adequate and o is GateOutcome.BLOCK,
    "D5": lambda s, o, adequate: o is GateOutcome.ESCALATE,
}


@dataclass(frozen=True)
class SubstitutedRuns:
    """Gate decisions of the base policy and under each substitution, keyed by scenario."""

    base: Mapping[str, ScenarioOutcome]
    substituted: Mapping[str, Mapping[str, ScenarioOutcome]]
    adequate: Mapping[str, bool]


def run_substitutions(
    ctx: Context, scenarios: Sequence[Scenario], variant: GateVariant, truth: TruthKind
) -> SubstitutedRuns:
    true_labels = true_alignment(ctx)
    runs = {
        LINK_ALIGNMENT: simulate(ctx, list(scenarios), variant, alignment=true_labels),
        LINK_VALIDITY: simulate(ctx, list(scenarios), variant, evidence_truth=EvidenceTruth.VALIDITY),
        LINK_ISOLATION: simulate(ctx, list(scenarios), variant, evidence_truth=EvidenceTruth.ISOLATION),
        LINK_ALL: simulate(ctx, list(scenarios), variant, alignment=true_labels, evidence_truth=EvidenceTruth.ALL),
    }
    base = simulate(ctx, list(scenarios), variant)
    return SubstitutedRuns(
        {o.scenario.key: o for o in base},
        {name: {o.scenario.key: o for o in outcomes} for name, outcomes in runs.items()},
        {o.scenario.key: is_adequate(ctx, o.scenario, truth) for o in base},
    )


def endpoint_attribution(endpoint_id: str, runs: SubstitutedRuns) -> dict[str, object]:
    """Substitutions that remove each error behind a gate endpoint, and the component they name."""
    predicate = ERROR_PREDICATES[endpoint_id]
    errors = [
        key
        for key, o in runs.base.items()
        if predicate(o.scenario, o.outcome, runs.adequate[key])
    ]
    removed: Counter[str] = Counter()
    per_error: dict[str, list[str]] = {}
    for key in errors:
        fixes = [
            link
            for link in SUBSTITUTIONS
            if not predicate(runs.base[key].scenario, runs.substituted[link][key].outcome, runs.adequate[key])
        ]
        per_error[key] = fixes
        removed.update(fixes)
    return {
        "errors": len(errors),
        "removed_by": {link: removed[link] for link in SUBSTITUTIONS},
        "per_error": per_error,
        "component": _component(removed),
    }


def _component(removed: Counter[str]) -> str:
    singles = {link: removed[link] for link in LINK_COMPONENT if removed[link] > 0}
    if not singles:
        return COMPONENT_GATE
    best = max(singles.items(), key=lambda item: (item[1], item[0]))[0]
    return LINK_COMPONENT[best]


# ------------------------------------------------------------ error multiplication


@dataclass(frozen=True)
class LinkRates:
    name: str
    false_pass: float
    false_block: float
    detail: str


@dataclass(frozen=True)
class ErrorMultiplication:
    links: Sequence[LinkRates]
    predicted_false_pass: float
    observed_false_pass: float
    ratio_false_pass: float
    ratio_false_pass_interval: Interval
    predicted_false_block: float
    observed_false_block: float
    ratio_false_block: float
    ratio_false_block_interval: Interval
    validity_false_valid: float
    validity_false_discard: float


def error_multiplication(
    ctx: Context, variant: GateVariant, settings: AnalysisSettings, traceable: bool
) -> ErrorMultiplication:
    """Per-link error rates, the independence prediction and the observed end-to-end error.

    The unit is the single-test scenario {t}: a gold bad test passed is a false pass, a gold
    good test blocked is a false block. The predictions multiply the links' own rates.
    """
    judge = judge_verdicts(ctx, JUDGE_H, ALIGNMENT_ARM_MASKED, False)
    signatures = matrix_signatures(ctx)
    singles = [Scenario(ctx.declared[n], frozenset({n}), False) for n in ctx.nodeids]
    outcomes = {next(iter(o.scenario.tests)): o.outcome for o in simulate(ctx, singles, variant)}
    cluster: dict[int, FloatArray] = {}
    for nodeid in ctx.nodeids:
        row = cluster.setdefault(ctx.labels[nodeid].cites, np.zeros(8))
        is_bad = not ctx.gold_good(nodeid)
        aligned_judge = judge[nodeid] is AlignmentVerdict.ALIGNED
        aligned_matrix = signatures[nodeid].verdict is AlignmentVerdict.ALIGNED
        if is_bad:
            row[0] += 1
            row[1] += int(outcomes[nodeid] is GateOutcome.PASS)
            row[2] += int(aligned_judge)
            row[3] += int(aligned_matrix)
        else:
            row[4] += 1
            row[5] += int(outcomes[nodeid] is GateOutcome.BLOCK)
            row[6] += int(judge[nodeid] is AlignmentVerdict.MISALIGNED)
            row[7] += int(signatures[nodeid].verdict is AlignmentVerdict.MISALIGNED)
    array = np.array(list(cluster.values()))
    totals = array.sum(axis=0)
    trace_fp = 1.0 if traceable else 0.0
    point_fp = _ratio_fp(totals[None, :], trace_fp)[0]
    point_fb = _ratio_fb(totals[None, :])[0]
    rates = _link_rates(ctx, totals, trace_fp, judge, signatures)
    predicted_fp = _predicted_fp(totals[None, :], trace_fp)[0]
    predicted_fb = _predicted_fb(totals[None, :])[0]
    observed_fp = totals[1] / totals[0] if totals[0] else float("nan")
    observed_fb = totals[5] / totals[4] if totals[4] else float("nan")
    valid_precision, discard = _validity_rates(ctx)
    return ErrorMultiplication(
        links=rates,
        predicted_false_pass=float(predicted_fp),
        observed_false_pass=float(observed_fp),
        ratio_false_pass=float(point_fp),
        ratio_false_pass_interval=bootstrap_interval(
            array, lambda s: _ratio_fp(s, trace_fp), settings.resamples, settings.seed
        ),
        predicted_false_block=float(predicted_fb),
        observed_false_block=float(observed_fb),
        ratio_false_block=float(point_fb),
        ratio_false_block_interval=bootstrap_interval(array, _ratio_fb, settings.resamples, settings.seed),
        validity_false_valid=valid_precision,
        validity_false_discard=discard,
    )


def _predicted_fp(sums: FloatArray, trace_fp: float) -> FloatArray:
    bad = sums[:, 0]
    return trace_fp * safe_divide(sums[:, 2], bad) * safe_divide(sums[:, 3], bad)


def _predicted_fb(sums: FloatArray) -> FloatArray:
    good = sums[:, 4]
    return 1 - (1 - safe_divide(sums[:, 6], good)) * (1 - safe_divide(sums[:, 7], good))


def _ratio_fp(sums: FloatArray, trace_fp: float) -> FloatArray:
    return safe_divide(safe_divide(sums[:, 1], sums[:, 0]), _predicted_fp(sums, trace_fp))


def _ratio_fb(sums: FloatArray) -> FloatArray:
    return safe_divide(safe_divide(sums[:, 5], sums[:, 4]), _predicted_fb(sums))


def _link_rates(
    ctx: Context,
    totals: FloatArray,
    trace_fp: float,
    judge: Mapping[str, AlignmentVerdict],
    signatures: Mapping[str, object],
) -> list[LinkRates]:
    bad, good = totals[0], totals[4]
    return [
        LinkRates("traceability", trace_fp, 0.0, "passes every bound test"),
        LinkRates(
            "alignment (J-H NAME-MASKED vs labels)",
            totals[2] / bad if bad else float("nan"),
            totals[6] / good if good else float("nan"),
            "false pass = gold bad judged ALIGNED; false block = gold good judged MISALIGNED",
        ),
        LinkRates(
            "matrix verdict vs labels",
            totals[3] / bad if bad else float("nan"),
            totals[7] / good if good else float("nan"),
            "false pass = gold bad ALIGNED; false block = gold good MISALIGNED",
        ),
    ]


def _validity_rates(ctx: Context) -> tuple[float, float]:
    """False VALID rate and false-discard rate of mutant validity against the oracle."""
    valid = [v for v in ctx.arm_t_evaluated() if v.primary_criterion is not None]
    wrong = sum(1 for v in valid if not ctx.oracle_valid_for(v.mutant_id, v.target))
    oracle_valid = [v for v in ctx.arm_t_evaluated() if ctx.oracle_valid_for(v.mutant_id, v.target)]
    discarded = sum(1 for v in oracle_valid if v.primary_criterion is None)
    return (
        wrong / len(valid) if valid else float("nan"),
        discarded / len(oracle_valid) if oracle_valid else float("nan"),
    )

