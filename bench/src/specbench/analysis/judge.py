"""Cost of a same-model judge: the archived judged requests replayed through another model, paired by snapshot.

Each snapshot carries the defect truth at S_T and the PASS decision of the same-model judge and of the
replay judge. Reported: sensitivity, specificity, false rejection and X_sel for each judge, and the
deltas (replay minus same-model). Capability is confounded with independence for the Opus replay.
"""

from __future__ import annotations

from enum import Enum

from specbench.analysis.bootstrap import Columns, Floats, Statistic
from specbench.analysis.estimate import add, column, one_minus_ratio, rate
from specbench.domain.records import TrunkRecord


class Judge(Enum):
    SAME_MODEL = "same"
    REPLAY = "replay"


def trunk_counts(trunk: TrunkRecord) -> dict[str, float]:
    if trunk.judge_replay is None or trunk.s_t is None:
        return {}
    h, defective = trunk.harness.value, trunk.s_t.defective
    counts: dict[str, float] = {}
    add(counts, column(h, "judge", "n"))
    add(counts, column(h, "judge", "defective"), defective)
    add(counts, column(h, "judge", "clean"), not defective)
    for judge, passed in (
        (Judge.SAME_MODEL, trunk.judge_replay.same_pass),
        (Judge.REPLAY, trunk.judge_replay.replay_pass),
    ):
        add(counts, column(h, "judge", judge.value, "pass"), passed)
        add(counts, column(h, "judge", judge.value, "pass_defective"), passed and defective)
    return counts


def sensitivity(h: str, judge: Judge) -> Statistic:
    def statistic(columns: Columns) -> Floats:
        caught = columns[column(h, "judge", "defective")] - columns[column(h, "judge", judge.value, "pass_defective")]
        return rate(caught, columns[column(h, "judge", "defective")])

    return statistic


def specificity(h: str, judge: Judge) -> Statistic:
    def statistic(columns: Columns) -> Floats:
        passed_clean = (
            columns[column(h, "judge", judge.value, "pass")]
            - columns[column(h, "judge", judge.value, "pass_defective")]
        )
        return rate(passed_clean, columns[column(h, "judge", "clean")])

    return statistic


def false_rejection(h: str, judge: Judge) -> Statistic:
    spec = specificity(h, judge)

    def statistic(columns: Columns) -> Floats:
        return 1.0 - spec(columns)

    return statistic


def x_selection(h: str, judge: Judge) -> Statistic:
    def statistic(columns: Columns) -> Floats:
        p_pass = rate(
            columns[column(h, "judge", judge.value, "pass_defective")], columns[column(h, "judge", judge.value, "pass")]
        )
        p_base = rate(columns[column(h, "judge", "defective")], columns[column(h, "judge", "n")])
        return one_minus_ratio(p_pass, p_base)

    return statistic
