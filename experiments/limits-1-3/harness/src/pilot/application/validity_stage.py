"""Validity judges V-H1, V-H2 and the mutant audit A-OPUS, with executed witnesses (PREREG 6.5, 6.7)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from concurrent.futures import ThreadPoolExecutor
from typing import Any

from pilot.application.context import PipelineContext
from pilot.application.model_caller import CallSpec
from pilot.application.mutant_records import load_edit_set
from pilot.application.ports import WitnessExecution, WitnessExecutor
from pilot.domain.call_type import CallType
from pilot.domain.differential_text import differential_sentence
from pilot.domain.edits import EditSet
from pilot.domain.model_call import MissingResult
from pilot.domain.model_slate import JUDGE_PIN, Judge
from pilot.domain.mutants import DifferentialKind, DifferentialResult
from pilot.domain.pipeline_constants import (
    DIFFERENTIAL_FILENAME,
    MAX_WITNESS_WORKERS,
    VALIDITY_FILENAME,
    WITNESS_FILENAME,
)
from pilot.domain.prompts import validity_item
from pilot.domain.validity_input import ValidityInput, build_validity_input
from pilot.domain.validity_payload import ValidityVerdict, parse_validity, validity_problems
from pilot.domain.witness import Witness, is_confirmed

STATUS_OK = "ok"
STATUS_MISSING = "missing"
CALL_ID_TEMPLATE = "validity__{judge}__{mutant_id}"
AUDIT_CALL_ID_TEMPLATE = "audit__{judge}__{mutant_id}"
ROW_KEY = ("mutant_id", "judge")


def judge_validity(
    ctx: PipelineContext,
    executor: WitnessExecutor,
    judges: Sequence[Judge],
    mutant_ids: Sequence[str],
) -> None:
    """One call per (judge, mutant), then one witness execution per judged VIOLATED witness."""
    call_type = CallType.AUDIT if Judge.A_OPUS in judges else CallType.VALIDITY
    differentials = _differentials(ctx)
    edit_sets = {mid: load_edit_set(ctx.store, mid) for mid in mutant_ids}
    sources, spec = ctx.sources.source_files(), ctx.sources.spec_text()
    jobs = [
        (judge, mid, _spec(ctx, call_type, judge, mid, build_validity_input(sources, edit_sets[mid]), spec, differentials.get(mid)))
        for judge in judges
        for mid in mutant_ids
    ]
    outcomes = ctx.caller.call_all(f"validity:{','.join(j.value for j in judges)}", [spec for _, _, spec in jobs])
    rows = []
    verdicts: dict[tuple[str, Judge], ValidityVerdict] = {}
    for judge, mid, call in jobs:
        outcome = outcomes[call.call_id]
        if isinstance(outcome, MissingResult):
            rows.append(_row(mid, judge, call.call_id, STATUS_MISSING, None))
            continue
        rows.append(_row(mid, judge, call.call_id, STATUS_OK, outcome.payload))
        verdicts[(mid, judge)] = parse_validity(outcome.payload)
    witness_rows = _execute_witnesses(executor, edit_sets, verdicts)
    _merge_rows(ctx, VALIDITY_FILENAME, rows, judges, mutant_ids)
    _merge_rows(ctx, WITNESS_FILENAME, witness_rows, judges, mutant_ids)
    missing = sum(1 for row in rows if row["status"] == STATUS_MISSING)
    ctx.log(f"[validity] {len(rows)} verdicts ({missing} missing), {len(witness_rows)} witnesses executed")


def _differentials(ctx: PipelineContext) -> dict[str, DifferentialResult]:
    if not ctx.store.exists(DIFFERENTIAL_FILENAME):
        return {}
    return {
        row["mutant_id"]: DifferentialResult(
            DifferentialKind(row["status"]),
            row["function"],
            row["args"],
            row["original_out"],
            row["mutant_out"],
            row["reason"],
        )
        for row in ctx.store.read_jsonl(DIFFERENTIAL_FILENAME)
    }


def _spec(
    ctx: PipelineContext,
    call_type: CallType,
    judge: Judge,
    mutant_id: str,
    judged: ValidityInput,
    spec: str,
    differential: DifferentialResult | None,
) -> CallSpec:
    template = CALL_ID_TEMPLATE if call_type is CallType.VALIDITY else AUDIT_CALL_ID_TEMPLATE
    item = validity_item(
        ctx.prompts.template(call_type),
        spec,
        judged.original,
        judged.mutant,
        judged.diff,
        differential_sentence(differential),
    )
    return CallSpec(
        call_id=template.format(judge=judge.value, mutant_id=mutant_id),
        pin=JUDGE_PIN[judge],
        call_type=call_type,
        item=item,
        schema_text=ctx.prompts.schema(call_type),
        validate=validity_problems,
    )


def _row(mutant_id: str, judge: Judge, call_id: str, status: str, payload: Any) -> dict[str, Any]:
    return {"mutant_id": mutant_id, "judge": judge.value, "call_id": call_id, "status": status, "payload": payload}


def _execute_witnesses(
    executor: WitnessExecutor,
    edit_sets: Mapping[str, EditSet],
    verdicts: Mapping[tuple[str, Judge], ValidityVerdict],
) -> list[dict[str, Any]]:
    jobs = [
        (mid, judge, ref, witness)
        for (mid, judge), verdict in sorted(verdicts.items(), key=lambda kv: (kv[0][0], kv[0][1].value))
        for ref, witness in sorted(verdict.witnesses().items())
    ]
    by_pair: dict[tuple[str, Judge], list[tuple[str, Witness]]] = {}
    for mid, judge, ref, witness in jobs:
        by_pair.setdefault((mid, judge), []).append((ref, witness))

    def run(pair: tuple[str, Judge]) -> list[dict[str, Any]]:
        mid, judge = pair
        entries = by_pair[pair]
        executions = executor.execute(edit_sets[mid], [w for _, w in entries])
        return [_witness_row(mid, judge, ref, w, e) for (ref, w), e in zip(entries, executions, strict=True)]

    with ThreadPoolExecutor(max_workers=MAX_WITNESS_WORKERS) as pool:
        batches = list(pool.map(run, sorted(by_pair, key=lambda p: (p[0], p[1].value))))
    return [row for batch in batches for row in batch]


def _witness_row(mid: str, judge: Judge, ref: str, witness: Witness, execution: WitnessExecution) -> dict[str, Any]:
    confirmed = (
        execution.error is None
        and execution.reference is not None
        and execution.mutant is not None
        and is_confirmed(witness.function, witness.expected, execution.reference, execution.mutant)
    )
    return {
        "mutant_id": mid,
        "judge": judge.value,
        "criterion": ref,
        "confirmed": confirmed,
        "reference_out": execution.reference.to_mapping() if execution.reference else None,
        "mutant_out": execution.mutant.to_mapping() if execution.mutant else None,
        "error": execution.error,
    }


def _merge_rows(
    ctx: PipelineContext,
    filename: str,
    rows: Sequence[Mapping[str, Any]],
    judges: Sequence[Judge],
    mutant_ids: Sequence[str],
) -> None:
    existing = ctx.store.read_jsonl(filename) if ctx.store.exists(filename) else []
    names, ids = {j.value for j in judges}, set(mutant_ids)
    kept = [r for r in existing if not (r["judge"] in names and r["mutant_id"] in ids)]
    key = ROW_KEY if filename == VALIDITY_FILENAME else (*ROW_KEY, "criterion")
    ctx.store.write_jsonl(filename, [*kept, *rows], key)
