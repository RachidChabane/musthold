"""Failure-mode catalogue (PREREG 11): every misclassified test, gate-error scenario and discordant mutant."""

from __future__ import annotations

from collections.abc import Mapping, Sequence

from pilot.analysis.attribution import SubstitutedRuns
from pilot.analysis.endpoints import judge_verdicts
from pilot.analysis.model import Context
from pilot.analysis.scenarios import Scenario
from pilot.analysis.table import Table
from pilot.analysis.tables import label_of, pipeline_class, truth_verdict
from pilot.domain import analysis_constants as k
from pilot.domain.alignment_aggregate import AlignmentVerdict
from pilot.domain.gate import GateOutcome
from pilot.domain.matrix import TestSignature
from pilot.domain.refs import criterion_ref

HEADER = ("kind", "id", "summary", "causal chain")
KIND_JUDGE = "judge error"
KIND_MATRIX = "matrix error"
KIND_GATE = "gate error"
KIND_MUTANT = "pipeline/oracle discordance"


def _short(nodeid: str) -> str:
    return nodeid.split("::")[-1]


def test_failures(ctx: Context, signatures: Mapping[str, TestSignature]) -> list[list[str]]:
    rows: list[list[str]] = []
    verdicts = judge_verdicts(ctx, k.JUDGE_SONNET, k.ALIGNMENT_ARM_MASKED, False)
    for nodeid in ctx.nodeids:
        truth = truth_verdict(ctx, nodeid)
        label = label_of(ctx, nodeid)
        view = ctx.run.alignment.get((k.JUDGE_SONNET, k.ALIGNMENT_ARM_MASKED, nodeid))
        if verdicts[nodeid] is not truth:
            chain = (
                f"label {label}, declared {criterion_ref(ctx.declared[nodeid])}; modal answer "
                f"{view.modal_ref if view else 'missing'} with coverage {view.modal_coverage if view else 'n/a'} "
                f"and share {view.modal_share if view else 0:.2f}; verdict {verdicts[nodeid].value}, truth {truth.value}"
            )
            rows.append([KIND_JUDGE, nodeid, f"J-SONNET NAME-MASKED {verdicts[nodeid].value} on a {label} test", chain])
        signature = signatures[nodeid]
        if signature.verdict is not truth and not (
            signature.verdict is AlignmentVerdict.UNDETERMINED and truth is AlignmentVerdict.MISALIGNED
        ):
            chain = (
                f"label {label}, declared {criterion_ref(ctx.declared[nodeid])}; signature {signature.signature.value} "
                f"(own {signature.own}, other {signature.other:.2f}, none {signature.none:.2f}); "
                f"kills VALID mutants of criteria {list(signature.criteria_killed)}; verdict {signature.verdict.value}"
            )
            rows.append([KIND_MATRIX, nodeid, f"matrix {signature.signature.value} on a {label} test", chain])
    return rows


def gate_failures(runs: SubstitutedRuns) -> list[list[str]]:
    rows: list[list[str]] = []
    for key, outcome in sorted(runs.base.items()):
        adequate = runs.adequate[key]
        wrong = (outcome.outcome is GateOutcome.PASS and not adequate) or (
            outcome.outcome is GateOutcome.BLOCK and adequate
        )
        if not wrong:
            continue
        removed = [link for link, found in runs.substituted.items() if _fixes(found[key].outcome, adequate)]
        chain = (
            f"{'ADEQUATE' if adequate else 'INADEQUATE'} scenario, gate {outcome.outcome.name}; "
            f"rules fired {[r.name for r in outcome.decision.fired] or 'none'}; "
            f"error removed by substituting {removed or 'no single link'}"
        )
        rows.append([KIND_GATE, key, _scenario_summary(outcome.scenario, outcome.outcome, adequate), chain])
    return rows


def _fixes(outcome: GateOutcome, adequate: bool) -> bool:
    return not ((outcome is GateOutcome.PASS and not adequate) or (outcome is GateOutcome.BLOCK and adequate))


def _scenario_summary(scenario: Scenario, outcome: GateOutcome, adequate: bool) -> str:
    kind = "false pass" if outcome is GateOutcome.PASS else "false block"
    return f"{kind} on {len(scenario.tests)} test(s) of {criterion_ref(scenario.criterion)}"


def mutant_discordances(ctx: Context) -> list[list[str]]:
    rows: list[list[str]] = []
    for view in sorted(ctx.run.classes.values(), key=lambda v: v.mutant_id):
        oracle = ctx.oracle_class.get(view.mutant_id)
        if oracle is None:
            continue
        pipeline_valid = view.target_valid and view.primary_included
        oracle_valid = oracle.valid_for(view.target)
        if pipeline_valid == oracle_valid:
            continue
        chain = (
            f"arm {view.arm}, target {criterion_ref(view.target) if view.target else 'none'}; pipeline class "
            f"{pipeline_class(view)}; oracle {oracle.kind.value} {list(oracle.criteria)}; "
            f"differential {ctx.run.differential.get(view.mutant_id, 'n/a')}; "
            f"killed by {len(ctx.killers.get(view.mutant_id, ()))} test(s)"
        )
        direction = "VALID but not oracle-valid" if pipeline_valid else "oracle-valid but not VALID"
        rows.append([KIND_MUTANT, view.mutant_id, direction, chain])
    return rows


def failure_table(
    ctx: Context, signatures: Mapping[str, TestSignature], runs: SubstitutedRuns
) -> Table:
    rows: Sequence[Sequence[str]] = [*test_failures(ctx, signatures), *gate_failures(runs), *mutant_discordances(ctx)]
    return Table("failure_modes", "failure-mode catalogue", list(HEADER), rows)
