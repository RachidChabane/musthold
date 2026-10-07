"""Descriptive tables the amendment adds to PREREG 11: isolation, same-model vote correlation, J-H calibration."""

from __future__ import annotations

from collections import Counter

from pilot.analysis.model import Context
from pilot.analysis.table import Table
from pilot.domain import analysis_constants as k
from pilot.domain.alignment_aggregate import AlignmentVerdict
from pilot.domain.pipeline_constants import TAU
from pilot.domain.refs import criterion_ref

MODAL_SHARES = (0.2, 0.4, 0.6, 0.8, 1.0)


def isolation_report(ctx: Context) -> Table:
    """VALID_w against VALID_x per criterion over the primary-matrix arm-T mutants, with the predicted exposure."""
    d8 = set(ctx.truth.d8_criteria)
    rows = []
    for c in ctx.criteria:
        views = [v for v in ctx.arm_t_evaluated() if v.primary_criterion == c]
        x = sum(1 for v in views if v.primary_valid_x)
        rows.append([criterion_ref(c), len(views), x, len(views) - x, "yes" if c in d8 else "no",
                     "predicted" if c in k.PREDICTED_ISOLATION_EXPOSURE else ""])
    return Table("isolation_table", "VALID_w vs VALID_x per criterion (primary-matrix arm-T mutants); predicted exposure {3, 5, 8}",
                 ["criterion", "VALID_w", "VALID_x", "vetoed by another binding", "in C_D8", "exposure"], rows)


def _vote_valid(ctx: Context, mutant_id: str, judge: str, target: int) -> bool | None:
    row = next((r for r in ctx.run.validity if r.mutant_id == mutant_id and r.judge == judge and r.status == k.CALL_OK and r.payload), None)
    if row is None:
        return None
    found = row.payload["requirements"].get(criterion_ref(target), {})  # type: ignore[index]
    confirmed = any(w.mutant_id == mutant_id and w.judge == judge and w.criterion == target and w.confirmed for w in ctx.run.witnesses)
    return found.get("verdict") == "VIOLATED" and float(found.get("confidence", 0)) >= TAU and confirmed


def same_model_correlation(ctx: Context) -> Table:
    """P(V-H2 wrong | V-H1 wrong) against the oracle-valid-for-target truth, arm T."""
    first_wrong = both_wrong = second_wrong = first_total = 0
    for view in ctx.arm_t_evaluated():
        if view.target is None:
            continue
        v1 = _vote_valid(ctx, view.mutant_id, k.VALIDITY_JUDGE_H1, view.target)
        v2 = _vote_valid(ctx, view.mutant_id, k.VALIDITY_JUDGE_H2, view.target)
        if v1 is None or v2 is None:
            continue
        truth = ctx.oracle_valid_for(view.mutant_id, view.target)
        w1, w2 = v1 != truth, v2 != truth
        first_total += 1
        first_wrong += int(w1)
        second_wrong += int(w2)
        both_wrong += int(w1 and w2)
    rows = [
        ["mutants with both votes", first_total, None], ["V-H1 wrong", first_wrong, first_total],
        ["V-H2 wrong", second_wrong, first_total], ["both wrong", both_wrong, first_total],
        ["P(V-H2 wrong | V-H1 wrong)", both_wrong, first_wrong],
    ]
    return Table("same_model_correlation", "errors of the two host votes against the oracle",
                 ["measure", "count", "of", "rate"], [[a, b, c, (b / c if c else None)] for a, b, c in rows])


def calibration(ctx: Context) -> Table:
    """Reliability, ECE and risk-coverage of J-H `modal_share` against the labels; UNDETERMINED reported apart."""
    items = []
    undetermined = 0
    for nodeid in ctx.nodeids:
        view = ctx.run.alignment.get((k.JUDGE_H, k.ALIGNMENT_ARM_MASKED, nodeid))
        if view is None:
            continue
        if view.verdict is AlignmentVerdict.UNDETERMINED:
            undetermined += 1
            continue
        truth = AlignmentVerdict.ALIGNED if ctx.gold_good(nodeid) else AlignmentVerdict.MISALIGNED
        items.append((view.modal_share, view.verdict is truth))
    rows: list[list[object]] = []
    total = len(items)
    ece = 0.0
    bins = Counter(round(share, 1) for share, _ in items)
    for share in MODAL_SHARES:
        selected = [ok for s, ok in items if round(s, 1) == share]
        accuracy = sum(selected) / len(selected) if selected else None
        rows.append(["reliability", share, len(selected), accuracy])
        if selected and total:
            ece += len(selected) / total * abs(accuracy - share)  # type: ignore[operator]
    rows.append(["ECE", None, total, ece if total else None])
    for share in sorted(bins, reverse=True):
        covered = [ok for s, ok in items if round(s, 1) >= share]
        rows.append(["risk-coverage", share, len(covered) / total if total else None, 1 - sum(covered) / len(covered)])
    rows.append(["UNDETERMINED rate", None, undetermined, undetermined / len(ctx.nodeids) if ctx.nodeids else None])
    return Table("jh_calibration", "J-H NAME-MASKED modal_share against labels (reliability, ECE, risk-coverage) and the UNDETERMINED rate",
                 ["measure", "modal_share", "n or coverage", "accuracy, ECE or risk"], rows)
