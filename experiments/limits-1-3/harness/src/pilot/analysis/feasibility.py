"""Feasibility F1 to F8 (PREREG 12)."""

from __future__ import annotations

from dataclasses import dataclass

from pilot.analysis.cost import production_cost_per_criterion, wall_clock_seconds
from pilot.analysis.model import Context
from pilot.analysis.preconditions import missing_rates
from pilot.domain import analysis_constants as k

CALL_STATUS_OK = "ok"


@dataclass(frozen=True)
class FeasibilityItem:
    id: str
    description: str
    value: str
    status: str

    def to_mapping(self) -> dict[str, str]:
        return {"id": self.id, "description": self.description, "value": self.value, "status": self.status}


def grade_higher(value: float, green: float, amber: float) -> str:
    if value >= green:
        return k.FEASIBILITY_GREEN
    return k.FEASIBILITY_AMBER if value >= amber else k.FEASIBILITY_RED


def grade_lower(value: float, green: float, amber: float) -> str:
    if value <= green:
        return k.FEASIBILITY_GREEN
    return k.FEASIBILITY_AMBER if value <= amber else k.FEASIBILITY_RED


def verdict_of(items: list[FeasibilityItem]) -> str:
    statuses = {i.status for i in items}
    if k.FEASIBILITY_RED in statuses:
        return k.VERDICT_STOP
    return k.VERDICT_AMEND if k.FEASIBILITY_AMBER in statuses else k.VERDICT_PROTOCOL_READY


def evaluate_feasibility(
    ctx: Context, valid_criteria_count: int, crosscheck_exact: bool | None
) -> tuple[list[FeasibilityItem], str]:
    calls = list(ctx.run.calls.values())
    ok_share = sum(1 for c in calls if c.status == CALL_STATUS_OK) / len(calls) if calls else 0.0
    violations = sum(c.isolation_violations for c in calls)
    worst_missing = max((r.rate for r in missing_rates(ctx)), default=0.0)
    arm_t = [m for m, meta in ctx.run.mutants.items() if meta.arm == k.ARM_T]
    stillborn = sum(1 for m in arm_t if (p := ctx.run.processing.get(m)) is not None and p.stillborn)
    stillborn_share = stillborn / len(arm_t) if arm_t else 1.0
    mean_cost, max_cost = production_cost_per_criterion(ctx)
    hours = wall_clock_seconds(ctx.run.timing) / k.SECONDS_PER_HOUR
    items = [
        FeasibilityItem("F1", "valid outputs", f"{ok_share:.1%} of {len(calls)} calls",
                        grade_higher(ok_share, k.F1_GREEN_MIN, k.F1_AMBER_MIN)),
        FeasibilityItem("F2", "isolation violations", str(violations),
                        k.FEASIBILITY_GREEN if violations == 0 else k.FEASIBILITY_RED),
        FeasibilityItem("F3", "judge missing rate (worst judge)", f"{worst_missing:.1%}",
                        grade_lower(worst_missing, k.F3_GREEN_MAX, k.F3_AMBER_MAX)),
        FeasibilityItem("F4", "arm-T stillborn share", f"{stillborn_share:.1%}",
                        grade_lower(stillborn_share, k.F4_GREEN_MAX, k.F4_AMBER_MAX)),
        FeasibilityItem("F5", "criteria with >= 2 VALID arm-T mutants", str(valid_criteria_count), _f5(valid_criteria_count)),
        FeasibilityItem("F6", "runner cross-check exact",
                        "not executed" if crosscheck_exact is None else str(crosscheck_exact),
                        k.FEASIBILITY_GREEN if crosscheck_exact else k.FEASIBILITY_RED),
        FeasibilityItem("F7", "Claude cost per criterion, production path (mean; max in the cost table)",
                        f"{mean_cost:.2f} USD mean, {max_cost:.2f} USD max",
                        grade_lower(mean_cost, k.F7_GREEN_MAX_USD, k.F7_AMBER_MAX_USD)),
        FeasibilityItem("F8", "wall clock", f"{hours:.2f} h",
                        grade_lower(hours, k.F8_GREEN_MAX_HOURS, k.F8_AMBER_MAX_HOURS)),
    ]
    return items, verdict_of(items)


def _f5(count: int) -> str:
    if count >= k.F5_GREEN:
        return k.FEASIBILITY_GREEN
    return k.FEASIBILITY_AMBER if count >= k.F5_AMBER_MIN else k.FEASIBILITY_RED

