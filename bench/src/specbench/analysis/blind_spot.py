"""Blind spots on agent-written tests: SB_coder with task-averaged escape rates (ADR 0007).

For a task, e(W, M) is the share of its evaluable valid mutants of source M that escape the tests
of writer W, pooled over the task's criteria. SB_coder = [e(coder, G) - e(coder, op)] - [e(U, G) - e(U, op)],
averaged over the tasks for which all four cells are evaluable; G is the generator subagent's source,
and the same contrast with Opus mutants as M gives the same-family cell. The truth is never the
comparison suite: a mutant is valid for a criterion by the H tests, which belong to neither writer.
"""

from __future__ import annotations

from collections import defaultdict
from typing import Final

from specbench.analysis.bootstrap import Columns, Floats, Statistic
from specbench.analysis.estimate import add, column, rate
from specbench.domain.records import MutantSource, TrunkRecord, Writer

VARIANTS: Final = (MutantSource.GENERATOR, MutantSource.OPUS)
CELL_WRITERS: Final = (Writer.CODER, Writer.UPSTREAM)


def task_rates(trunk: TrunkRecord) -> dict[tuple[Writer, MutantSource], float]:
    """e_t(W, M) for every cell with at least one evaluable mutant, pooled over the task's criteria."""
    escaped: defaultdict[tuple[Writer, MutantSource], int] = defaultdict(int)
    evaluable: defaultdict[tuple[Writer, MutantSource], int] = defaultdict(int)
    for cell in trunk.blind_spot:
        escaped[(cell.writer, cell.source)] += cell.escaped
        evaluable[(cell.writer, cell.source)] += cell.evaluable
    return {key: escaped[key] / count for key, count in evaluable.items() if count > 0}


def trunk_counts(trunk: TrunkRecord) -> dict[str, float]:
    h = trunk.harness.value
    rates = task_rates(trunk)
    counts: dict[str, float] = {}
    for variant in VARIANTS:
        cells = [(w, s) for w in CELL_WRITERS for s in (variant, MutantSource.OPERATOR)]
        if not all(cell in rates for cell in cells):
            continue
        contrast = (rates[(Writer.CODER, variant)] - rates[(Writer.CODER, MutantSource.OPERATOR)]) - (
            rates[(Writer.UPSTREAM, variant)] - rates[(Writer.UPSTREAM, MutantSource.OPERATOR)]
        )
        add(counts, column(h, "sb", variant.value, "tasks"))
        add(counts, column(h, "sb", variant.value, "contrast"), contrast)
        for writer, source in cells:
            add(counts, column(h, "sb", variant.value, writer.value, source.value), rates[(writer, source)])
    return counts


def sb_coder(h: str, variant: MutantSource) -> Statistic:
    def statistic(columns: Columns) -> Floats:
        return rate(
            columns[column(h, "sb", variant.value, "contrast")], columns[column(h, "sb", variant.value, "tasks")]
        )

    return statistic


def cell_rate(h: str, variant: MutantSource, writer: Writer, source: MutantSource) -> Statistic:
    def statistic(columns: Columns) -> Floats:
        return rate(
            columns[column(h, "sb", variant.value, writer.value, source.value)],
            columns[column(h, "sb", variant.value, "tasks")],
        )

    return statistic
