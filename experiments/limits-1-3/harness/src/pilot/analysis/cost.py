"""Cost and wall clock (PREREG 11), from the top-level `raw/<call_id>/result.json` records."""

from __future__ import annotations

from collections import defaultdict
from collections.abc import Mapping
from dataclasses import dataclass

from pilot.analysis.model import Context
from pilot.domain import analysis_constants as k
from pilot.domain.run_records import CallRecord

CLAUDE_CLI = "claude"
OVERHEAD = "overhead"
PRODUCTION = "production"


@dataclass(frozen=True)
class CallAttribution:
    call_id: str
    path: str
    criterion: int | None
    claude: bool


@dataclass(frozen=True)
class CostLine:
    group: str
    criterion: int | None
    claude_usd: float
    claude_tokens: int
    codex_tokens: int
    calls: int


def attribute_calls(ctx: Context) -> dict[str, CallAttribution]:
    """Production-path calls by criterion: arm-T generation, both validity judges, J-SONNET NAME-MASKED."""
    found: dict[str, CallAttribution] = {}
    for meta in ctx.run.mutants.values():
        if meta.arm == k.ARM_T and meta.generator_call_id:
            found[meta.generator_call_id] = CallAttribution(
                meta.generator_call_id, PRODUCTION, meta.target, meta.generator_cli == CLAUDE_CLI
            )
    for row in ctx.run.validity:
        meta = ctx.run.mutants.get(row.mutant_id)
        if meta is not None and meta.arm == k.ARM_T and row.judge in (k.VALIDITY_JUDGE_SONNET, k.VALIDITY_JUDGE_GPT):
            found[row.call_id] = CallAttribution(row.call_id, PRODUCTION, meta.target, row.judge == k.VALIDITY_JUDGE_SONNET)
    for call in ctx.run.alignment_calls:
        if call.judge == k.JUDGE_SONNET and call.arm == k.ALIGNMENT_ARM_MASKED:
            found[call.call_id] = CallAttribution(call.call_id, PRODUCTION, ctx.declared.get(call.nodeid), True)
    return found


def cost_lines(ctx: Context) -> list[CostLine]:
    """Per-criterion production cost, then the experiment overhead (every other call)."""
    attributed = attribute_calls(ctx)
    buckets: dict[tuple[str, int | None], list[CallRecord]] = defaultdict(list)
    claude_flags: dict[str, bool] = {}
    for call_id, call in ctx.run.calls.items():
        attribution = attributed.get(call_id)
        key = (PRODUCTION, attribution.criterion) if attribution else (OVERHEAD, None)
        buckets[key].append(call)
        claude_flags[call_id] = attribution.claude if attribution else call.cost_usd is not None
    return [
        CostLine(
            group, criterion,
            sum(c.cost_usd or 0.0 for c in calls if claude_flags[c.call_id]),
            sum(c.input_tokens + c.output_tokens for c in calls if claude_flags[c.call_id]),
            sum(c.input_tokens + c.output_tokens for c in calls if not claude_flags[c.call_id]),
            len(calls),
        )
        for (group, criterion), calls in sorted(buckets.items(), key=lambda kv: (kv[0][0], kv[0][1] or 0))
    ]


def production_cost_per_criterion(ctx: Context) -> tuple[float, float]:
    """Mean and maximum Claude USD per criterion on the production path."""
    per_criterion = {c: 0.0 for c in ctx.criteria}
    for line in cost_lines(ctx):
        if line.group == PRODUCTION and line.criterion in per_criterion:
            per_criterion[line.criterion] += line.claude_usd
    values = list(per_criterion.values())
    return (sum(values) / len(values), max(values)) if values else (0.0, 0.0)


def wall_clock_seconds(timing: Mapping[str, Mapping[str, object]]) -> float:
    return sum(float(stage.get("seconds", 0.0)) for stage in timing.values())  # type: ignore[arg-type]


def cpu_seconds(timing: Mapping[str, Mapping[str, object]]) -> float | None:
    values = [float(stage["cpu_seconds"]) for stage in timing.values() if "cpu_seconds" in stage]  # type: ignore[arg-type]
    return sum(values) if values else None
