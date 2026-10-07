"""X per arm (in the loop), X_sel (selection at S_T), yield, false rejection and escaped defects, from additive counts.

Definitions (ADR 0004 and 0007). Population: trunks that reached S_T, per harness.
p_a = P(primary defect | accepted by a); X_a = 1 - p_a / p_T, p_T being the defect rate at S_T.
X_sel = 1 - P(defect | arm would PASS at S_T) / p_T. Acceptance counts PASS only; the
lenient mode also counts ESCALATE, the strict mode refuses any ESCALATE during the trunk.
"""

from __future__ import annotations

from enum import Enum
from typing import Final

from specbench.analysis.bootstrap import Columns, Floats, Statistic
from specbench.analysis.estimate import add, column, one_minus_ratio, rate
from specbench.domain.arms import ACCEPTANCE_ARMS, Arm
from specbench.domain.records import ArmResult, TrunkRecord, Truth


class Mode(Enum):
    STANDARD = "std"
    LENIENT = "lenient"
    STRICT = "strict"


MODES: Final = tuple(Mode)


def _accepted(result: ArmResult, mode: Mode) -> bool:
    return result.accepted(lenient=mode is Mode.LENIENT, strict=mode is Mode.STRICT)


def trunk_counts(trunk: TrunkRecord) -> dict[str, float]:
    """The additive contribution of one trunk to every outcome column of its harness."""
    h = trunk.harness.value
    counts: dict[str, float] = {}
    add(counts, column(h, "trunks"))
    add(counts, column(h, "sn_defective"), trunk.s_n.defective)
    if trunk.s_t is None:
        add(counts, column(h, "unreached"))
        return counts
    s_t: Truth = trunk.s_t
    add(counts, column(h, "n"))
    add(counts, column(h, "defective"), s_t.defective)
    add(counts, column(h, "clean"), not s_t.defective)
    for arm in ACCEPTANCE_ARMS:
        result = trunk.arms.get(arm)
        if result is None:
            continue
        _arm_counts(counts, h, arm, result, s_t)
    return counts


def _arm_counts(counts: dict[str, float], h: str, arm: Arm, result: ArmResult, s_t: Truth) -> None:
    a = arm.value
    add(counts, column(h, a, "base_n"))
    add(counts, column(h, a, "calls"), result.model_calls)
    add(counts, column(h, a, "seconds"), result.seconds)
    add(counts, column(h, a, "base_defective"), s_t.defective)
    add(counts, column(h, a, "base_clean"), not s_t.defective)
    if result.selection_pass is not None:
        add(counts, column(h, a, "sel_n"))
        add(counts, column(h, a, "sel_defective"), s_t.defective)
        add(counts, column(h, a, "sel_clean"), not s_t.defective)
        add(counts, column(h, a, "sel_pass"), result.selection_pass)
        add(counts, column(h, a, "sel_pass_defective"), result.selection_pass and s_t.defective)
    for mode in MODES:
        accepted = _accepted(result, mode)
        final = result.final
        if accepted and final is None:
            raise ValueError(f"{arm.value} accepted a trunk without recording its final truth")
        m = mode.value
        add(counts, column(h, a, m, "accepted"), accepted)
        add(counts, column(h, a, m, "rejected_clean"), not accepted and not s_t.defective)
        if accepted and final is not None:
            add(counts, column(h, a, m, "accepted_defective"), final.defective)
            add(counts, column(h, a, m, "accepted_defective_full"), final.defective_full)
            add(counts, column(h, a, m, "accepted_o_failed"), final.o_failed)


def x_loop(h: str, arm: Arm, mode: Mode, full_truth: bool = False) -> Statistic:
    """Statistic: X of an arm in the loop, `1 - p_a / p_T`, on the primary truth or on U+H+R."""
    a, m = arm.value, mode.value
    defect = "accepted_defective_full" if full_truth else "accepted_defective"

    def statistic(columns: Columns) -> Floats:
        p_arm = rate(columns[column(h, a, m, defect)], columns[column(h, a, m, "accepted")])
        p_base = rate(columns[column(h, a, "base_defective")], columns[column(h, a, "base_n")])
        return one_minus_ratio(p_arm, p_base)

    return statistic


def x_selection(h: str, arm: Arm) -> Statistic:
    a = arm.value

    def statistic(columns: Columns) -> Floats:
        p_pass = rate(columns[column(h, a, "sel_pass_defective")], columns[column(h, a, "sel_pass")])
        p_base = rate(columns[column(h, a, "sel_defective")], columns[column(h, a, "sel_n")])
        return one_minus_ratio(p_pass, p_base)

    return statistic


def sensitivity(h: str, arm: Arm) -> Statistic:
    """P(rejected at S_T | defective)."""
    a = arm.value

    def statistic(columns: Columns) -> Floats:
        caught = columns[column(h, a, "sel_defective")] - columns[column(h, a, "sel_pass_defective")]
        return rate(caught, columns[column(h, a, "sel_defective")])

    return statistic


def specificity(h: str, arm: Arm) -> Statistic:
    """P(passed at S_T | clean)."""
    a = arm.value

    def statistic(columns: Columns) -> Floats:
        passed_clean = columns[column(h, a, "sel_pass")] - columns[column(h, a, "sel_pass_defective")]
        return rate(passed_clean, columns[column(h, a, "sel_clean")])

    return statistic


def yield_rate(h: str, arm: Arm, mode: Mode) -> Statistic:
    """P(accepted | S_T)."""
    a, m = arm.value, mode.value

    def statistic(columns: Columns) -> Floats:
        return rate(columns[column(h, a, m, "accepted")], columns[column(h, a, "base_n")])

    return statistic


def false_rejection(h: str, arm: Arm, mode: Mode) -> Statistic:
    """P(not accepted | S_T clean)."""
    a, m = arm.value, mode.value

    def statistic(columns: Columns) -> Floats:
        return rate(columns[column(h, a, m, "rejected_clean")], columns[column(h, a, "base_clean")])

    return statistic


def escaped_defects(h: str, arm: Arm, mode: Mode) -> Statistic:
    """P(accepted and defective | S_T)."""
    a, m = arm.value, mode.value

    def statistic(columns: Columns) -> Floats:
        return rate(columns[column(h, a, m, "accepted_defective")], columns[column(h, a, "base_n")])

    return statistic


def accepted_o_rate(h: str, arm: Arm, mode: Mode) -> Statistic:
    """P(O assertion fails | accepted): out-of-spec defects among accepted changes, never part of X."""
    a, m = arm.value, mode.value

    def statistic(columns: Columns) -> Floats:
        return rate(columns[column(h, a, m, "accepted_o_failed")], columns[column(h, a, m, "accepted")])

    return statistic


def defect_rate_at(h: str, arm: Arm, mode: Mode) -> Statistic:
    """p_a: P(primary defect | accepted by a)."""
    a, m = arm.value, mode.value

    def statistic(columns: Columns) -> Floats:
        return rate(columns[column(h, a, m, "accepted_defective")], columns[column(h, a, m, "accepted")])

    return statistic


def difference(first: Statistic, second: Statistic) -> Statistic:
    """Statistic: the difference of two statistics; undefined where either is."""

    def statistic(columns: Columns) -> Floats:
        return first(columns) - second(columns)

    return statistic


def cost_per_accepted(h: str, arm: Arm, mode: Mode, quantity: str) -> Statistic:
    """Model calls (or seconds) spent per accepted change, repair work included."""
    a, m = arm.value, mode.value

    def statistic(columns: Columns) -> Floats:
        return rate(columns[column(h, a, quantity)], columns[column(h, a, m, "accepted")])

    return statistic
