"""Scenarios (PREREG 9.1), their truth (9.3) and the gate simulation over them."""

from __future__ import annotations

from collections import defaultdict
from dataclasses import dataclass
from enum import Enum
from itertools import combinations

from pilot.adapters.shallnot import parse_report
from pilot.analysis.model import Context, gate_evidence, oracle_valid_kill_sets, oracle_valid_results
from pilot.domain.alignment_aggregate import AlignmentVerdict
from pilot.domain.analysis_constants import ALIGNMENT_ARM_MASKED, JUDGE_SONNET
from pilot.domain.gate import (
    G3,
    CriterionEvidence,
    GateDecision,
    GateInputs,
    GateOutcome,
    GatePolicy,
    evaluate_gate,
)
from pilot.domain.traceability import null_gate_failures


class TruthKind(Enum):
    ORACLE = "primary (oracle-adjudicated)"
    LABEL = "secondary (label-based)"


@dataclass(frozen=True)
class Scenario:
    criterion: int
    tests: frozenset[str]
    is_full: bool

    @property
    def key(self) -> str:
        return f"PARK-{self.criterion}:" + "+".join(sorted(self.tests))


@dataclass(frozen=True)
class GateVariant:
    """Which evidence the gate reads: judge, alignment arm, S1 rule and F_brief filter."""

    name: str
    policy: GatePolicy = G3
    judge: str = JUDGE_SONNET
    alignment_arm: str = ALIGNMENT_ARM_MASKED
    use_s1: bool = False
    f_brief: bool = False


@dataclass(frozen=True)
class ScenarioOutcome:
    scenario: Scenario
    decision: GateDecision

    @property
    def outcome(self) -> GateOutcome:
        return self.decision.outcome


def build_scenarios(ctx: Context) -> list[Scenario]:
    """Every non-empty subset of the tests declared for each criterion, the full set flagged."""
    by_criterion: dict[int, list[str]] = defaultdict(list)
    for nodeid in ctx.nodeids:
        by_criterion[ctx.declared[nodeid]].append(nodeid)
    scenarios: list[Scenario] = []
    for criterion in sorted(by_criterion):
        tests = sorted(by_criterion[criterion])
        for size in range(1, len(tests) + 1):
            scenarios += [
                Scenario(criterion, frozenset(subset), size == len(tests))
                for subset in combinations(tests, size)
            ]
    return scenarios


def is_adequate(ctx: Context, scenario: Scenario, kind: TruthKind) -> bool:
    has_gold_good = any(ctx.gold_good(test) for test in scenario.tests)
    if kind is TruthKind.LABEL or not has_gold_good:
        return has_gold_good
    return all(
        not kills.isdisjoint(scenario.tests) for kills in oracle_valid_kill_sets(ctx, scenario.criterion)
    )


def traceability_passes(ctx: Context) -> bool:
    report = parse_report(dict(ctx.run.traceability_report))
    return not null_gate_failures(report, len(ctx.nodeids), len(ctx.criteria))


def alignment_verdicts(ctx: Context, variant: GateVariant) -> dict[str, AlignmentVerdict]:
    verdicts: dict[str, AlignmentVerdict] = {}
    for nodeid in ctx.nodeids:
        view = ctx.run.alignment.get((variant.judge, variant.alignment_arm, nodeid))
        if view is None:
            verdicts[nodeid] = AlignmentVerdict.UNDETERMINED
        else:
            verdicts[nodeid] = view.verdict_s1 if variant.use_s1 else view.verdict
    return verdicts


def true_alignment(ctx: Context) -> dict[str, AlignmentVerdict]:
    """Alignment substituted by the labels (PREREG 9.4)."""
    return {
        n: AlignmentVerdict.ALIGNED if ctx.gold_good(n) else AlignmentVerdict.MISALIGNED for n in ctx.nodeids
    }


def simulate(
    ctx: Context,
    scenarios: list[Scenario],
    variant: GateVariant,
    *,
    alignment: dict[str, AlignmentVerdict] | None = None,
    oracle_evidence: bool = False,
) -> list[ScenarioOutcome]:
    """Gate decisions for every scenario; the substitutions of 9.4 replace links by their truth."""
    traceable = traceability_passes(ctx)
    references = ctx.run.reference_passes
    verdicts = alignment if alignment is not None else alignment_verdicts(ctx, variant)
    evidence = {c: _evidence(ctx, c, variant, oracle_evidence) for c in ctx.criteria}
    return [
        ScenarioOutcome(
            s,
            evaluate_gate(variant.policy, s.tests, GateInputs(traceable, references, evidence[s.criterion], verdicts)),
        )
        for s in scenarios
    ]


def _evidence(
    ctx: Context, criterion: int, variant: GateVariant, oracle_evidence: bool
) -> CriterionEvidence:
    if oracle_evidence:
        return CriterionEvidence(tuple(oracle_valid_results(ctx, criterion)), ())
    return gate_evidence(ctx, criterion, variant.f_brief)
