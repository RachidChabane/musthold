"""The blind-spot estimand over a run: cells from the sources, the suites, the kill vectors and the oracle."""

from __future__ import annotations

from collections.abc import Sequence

from pilot.analysis.blind_spot import (
    CELL_ORDER,
    BlindSpotResult,
    CellCount,
    Escape,
    LocationSplit,
    count_outcomes,
    escape_outcome,
    estimate,
    location_split,
)
from pilot.analysis.model import Context
from pilot.analysis.settings import AnalysisSettings
from pilot.analysis.table import Table
from pilot.analysis.stats import wilson
from pilot.domain import analysis_constants as k
from pilot.domain.exploratory import STREAM_LUNA
from pilot.domain.model_slate import MutantSource, Writer

EXPLORATORY_WRITERS = (Writer.LUNA, Writer.FIXTURE)
EXPLORATORY_SOURCES = (MutantSource.TERRA, MutantSource.LUNA)
CROSS_FAMILY = (Writer.LUNA, MutantSource.TERRA, MutantSource.LUNA)
ALL_WRITERS = (Writer.SONNET, Writer.OPUS, *EXPLORATORY_WRITERS)
ALL_SOURCES = (MutantSource.SONNET, MutantSource.OPUS, MutantSource.OPERATOR, *EXPLORATORY_SOURCES)


def evaluated_members(ctx: Context, source: MutantSource) -> list[str]:
    return sorted(m for m in ctx.run.sources.get(source.value, ()) if ctx.run.is_evaluated(m))


def universe(ctx: Context, source: MutantSource, criterion: int) -> list[str]:
    """U(M, c): evaluated, deduplicated mutants of the source that are oracle-valid for c, whatever their target."""
    return [m for m in evaluated_members(ctx, source) if ctx.oracle_valid_for(m, criterion)]


def suite_nodeids(ctx: Context, writer: Writer, criterion: int) -> tuple[str, ...]:
    if writer is Writer.FIXTURE:
        return tuple(n for n in ctx.nodeids if ctx.labels[n].cites == criterion and ctx.gold_good(n))
    suite = ctx.run.suites.get(writer.value, {}).get(criterion)
    return suite.kept if suite is not None else ()


def pair_results(ctx: Context, writer: Writer, mutant_id: str, nodeids: Sequence[str]) -> list[str | None]:
    found = ctx.run.kills.get(mutant_id) if writer is Writer.FIXTURE else ctx.run.writer_kills.get((writer.value, mutant_id))
    return [found.get(n) if found is not None else None for n in nodeids]


def pair_outcome(ctx: Context, writer: Writer, mutant_id: str, criterion: int) -> Escape:
    return escape_outcome(pair_results(ctx, writer, mutant_id, suite_nodeids(ctx, writer, criterion)))


def cell_count(ctx: Context, writer: Writer, source: MutantSource, criterion: int) -> CellCount | None:
    if not suite_nodeids(ctx, writer, criterion):
        return None
    return count_outcomes([pair_outcome(ctx, writer, m, criterion) for m in universe(ctx, source, criterion)])


def primary_cells(ctx: Context) -> dict[tuple[Writer, MutantSource, int], CellCount | None]:
    return {(w, s, c): cell_count(ctx, w, s, c) for w, s in CELL_ORDER for c in ctx.criteria}


def blind_spot(ctx: Context, settings: AnalysisSettings) -> BlindSpotResult:
    return estimate(primary_cells(ctx), ctx.criteria, settings)


def _escaping(ctx: Context, writer: Writer, source: MutantSource, criteria: Sequence[int]) -> dict[int, list[str]]:
    return {c: [m for m in universe(ctx, source, c) if suite_nodeids(ctx, writer, c) and pair_outcome(ctx, writer, m, c) is Escape.ESCAPED] for c in criteria}


def location_splits(ctx: Context, result: BlindSpotResult) -> list[LocationSplit]:
    def hosts(mutant_id: str) -> tuple[str, ...]:
        return ctx.run.mutants[mutant_id].hosts

    return [
        location_split(w, s, _escaping(ctx, w, s, result.c_star), _escaping(ctx, w, MutantSource.OPERATOR, result.c_star), hosts, result.c_star)
        for w in (Writer.SONNET, Writer.OPUS)
        for s in (MutantSource.SONNET, MutantSource.OPUS)
    ]


def blind_spot_tables(ctx: Context, result: BlindSpotResult) -> list[Table]:
    return [
        _estimand_table(result), _cells_table(result), _exclusion_table(result), _location_table(ctx, result),
        _exploratory_table(ctx), _yield_table(ctx),
    ]


def _fmt(value: float | None) -> float | None:
    return None if value is None or value != value else value


def _estimand_table(result: BlindSpotResult) -> Table:
    c = result.contrasts
    rows = [[name, _fmt(getattr(c, attr)) if c else None] for name, attr in
            (("Delta_S", "delta_s"), ("Delta_O", "delta_o"), ("SB_S", "sb_s"), ("SB_O", "sb_o"), ("SB", "sb"))]
    rows += [["SB interval lo", _fmt(result.interval.lo)], ["SB interval hi", _fmt(result.interval.hi)],
             ["reading", result.reading], ["upper bound below materiality", str(result.below_materiality)],
             ["criteria in C*", len(result.c_star)]]
    return Table("blind_spot_estimand", "shared blind spot SB (descriptive): contrasts, bootstrap interval and fixed reading",
                 ["measure", "value"], rows)


def _cells_table(result: BlindSpotResult) -> Table:
    rows = [[w.value, s.value, result.pooled[(w, s)].escaped, result.pooled[(w, s)].caught,
             result.pooled[(w, s)].excluded, _fmt(result.means[(w, s)])] for w, s in CELL_ORDER]
    return Table("blind_spot_cells", "e(W, M) over C*: writers (rows) by mutant sources, with pooled counts",
                 ["writer", "source", "escaped", "caught", "excluded", "e(W,M) criterion-weighted"], rows)


def _exclusion_table(result: BlindSpotResult) -> Table:
    return Table("blind_spot_exclusions", "criteria outside C* with their reasons", ["criterion", "reasons"],
                 [[f"PARK-{e.criterion}~1", "; ".join(e.reasons)] for e in result.exclusions])


def _location_table(ctx: Context, result: BlindSpotResult) -> Table:
    rows = [[s.writer.value, s.source.value, s.escaping, s.coverage_hole, s.model_specific,
             s.coverage_hole / s.escaping if s.escaping else None] for s in location_splits(ctx, result)]
    return Table("blind_spot_location", "escaping model mutants: coverage hole (an escaping operator mutant edits the same definition) vs model-specific",
                 ["writer", "source", "escaping model mutants", "coverage hole", "model-specific", "coverage-hole share"], rows)


def _blocked_calls(ctx: Context) -> int:
    return sum(1 for status in ctx.run.exploratory_status.get(STREAM_LUNA, {}).values() if status != "complete")


def _exploratory_table(ctx: Context) -> Table:
    rows = []
    primary = {(w, s) for w, s in CELL_ORDER}
    for writer in ALL_WRITERS:
        for source in ALL_SOURCES:
            if (writer, source) in primary:
                continue
            counts = [c for crit in ctx.criteria if (c := cell_count(ctx, writer, source, crit)) is not None and c.evaluable]
            total = CellCount()
            for c in counts:
                total = total.plus(c)
            cross = writer in CROSS_FAMILY or source in CROSS_FAMILY
            luna = Writer.LUNA is writer or MutantSource.LUNA is source
            blocked = _blocked_calls(ctx) if luna else 0
            cell = "BLOCKED" if luna and not counts else ("" if counts else "no evaluable criterion")
            mean = sum(c.rate for c in counts) / len(counts) if counts else None
            rows.append([writer.value, source.value, "cross-family" if cross else "same-family", len(counts), total.escaped,
                         total.caught, total.excluded, mean, blocked, cell])
    return Table("blind_spot_exploratory", "exploratory cells (labelled cross-family where luna or terra is involved)",
                 ["writer", "source", "family", "criteria", "escaped", "caught", "excluded", "e(W,M)", "luna calls not complete", "status"], rows)


def _yield_table(ctx: Context) -> Table:
    rows = []
    for source in ALL_SOURCES:
        members = ctx.run.sources.get(source.value, ())
        evaluated = [m for m in members if ctx.run.is_evaluated(m)]
        valid = [m for m in evaluated if (o := ctx.oracle_class.get(m)) is not None and o.kind.value == "oracle-valid"]
        interval = wilson(len(valid), len(evaluated))
        rows.append([source.value, len(members), len(evaluated), len(valid),
                     len(valid) / len(evaluated) if evaluated else None, interval.lo, interval.hi])
    return Table("source_yield", "per-source oracle-valid yield: Sonnet (S), Opus (O), operators (op), terra, luna",
                 ["source", "candidates", "non-stillborn", "oracle-valid", "share", "wilson lo", "wilson hi"], rows)
