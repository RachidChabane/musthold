"""Alignment judges and the leakage probe over the fixture's tests (PREREG 7)."""

from __future__ import annotations

from collections import defaultdict
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Any

from pilot.application.context import PipelineContext
from pilot.application.model_caller import CallSpec
from pilot.domain.alignment_aggregate import Repetition, aggregate_alignment
from pilot.domain.alignment_input import (
    Arm,
    TestUnit,
    alignment_schema,
    build_test_unit,
    public_signatures,
    tokenize_criteria,
)
from pilot.domain.call_type import CallType
from pilot.domain.ids import slugify
from pilot.domain.model_call import MissingResult, PayloadCheck
from pilot.domain.model_slate import JUDGE_PIN, Judge
from pilot.domain.pipeline_constants import (
    ALIGNMENT_AGGREGATE_FILENAME,
    ALIGNMENT_CONFIDENCE_MAX,
    ALIGNMENT_FILENAME,
    ALIGNMENT_PROOFS_DIRNAME,
    ALIGNMENT_REPS,
    NONE_ANSWER,
    PROBE_FILENAME,
    PROBE_REPS,
    PROOF_SUFFIX,
    PROOFS_DIRNAME,
    TOKEN_MAPS_DIRNAME,
)
from pilot.domain.prompts import alignment_item, probe_item
from pilot.domain.proofs import (
    ROLE_CODE_TREE,
    ROLE_MODEL_ID,
    ROLE_PROMPT_TEMPLATE,
    ROLE_SPEC,
    ROLE_TEST_SOURCE,
    alignment_statement,
    files_digest,
    text_resource,
)
from pilot.domain.spec import Criterion

STATUS_OK = "ok"
STATUS_MISSING = "missing"
TEST_FILE_SEPARATOR = "::"
ALIGNMENT_CALL_ID = "align__{judge}__{arm}__{slug}__r{rep}"
PROBE_CALL_ID = "probe__{judge}__{slug}__r{rep}"
ALIGNMENT_KEY = ("judge", "arm", "nodeid", "rep")
PROBE_KEY = ("judge", "nodeid", "rep")
COVERAGE_NONE = "NONE"


@dataclass(frozen=True)
class AlignmentCall:
    judge: Judge
    arm: Arm
    nodeid: str
    rep: int
    spec: CallSpec
    token_map: Mapping[str, str]


class UnitBook:
    """Test units and signatures built once per (nodeid, arm) from the fixture."""

    def __init__(self, ctx: PipelineContext) -> None:
        self._tests = ctx.sources.test_files()
        self._package = {
            path: text for path, text in ctx.sources.source_files().items() if path.endswith(".py")
        }
        self._units: dict[tuple[str, Arm], TestUnit] = {}

    def unit(self, nodeid: str, arm: Arm) -> TestUnit:
        key = (nodeid, arm)
        if key not in self._units:
            file, name = nodeid.split(TEST_FILE_SEPARATOR, 1)
            self._units[key] = build_test_unit(self._tests[file], name, arm)
        return self._units[key]

    def signatures(self, nodeid: str, arm: Arm, package: str) -> str:
        return public_signatures(self.unit(nodeid, arm), package, self._package)


def alignment_problems(token_map: Mapping[str, str]) -> PayloadCheck:
    allowed = {*token_map, NONE_ANSWER}

    def check(payload: Any) -> list[str]:
        problems = []
        if payload["verifies"] not in allowed:
            problems.append(f"verifies {payload['verifies']!r} is not one of the call's tokens")
        if (payload["verifies"] == NONE_ANSWER) != (payload["coverage"] == COVERAGE_NONE):
            problems.append("verifies is NONE exactly when coverage is NONE")
        if not 0 <= payload["confidence"] <= ALIGNMENT_CONFIDENCE_MAX:
            problems.append("confidence outside 0..100")
        return problems

    return check


def build_alignment_calls(
    ctx: PipelineContext,
    judge: Judge,
    arm: Arm,
    nodeids: Sequence[str],
    units: UnitBook,
    package: str,
    reps: int = ALIGNMENT_REPS,
) -> list[AlignmentCall]:
    criteria = ctx.criteria()
    template = ctx.prompts.template(CallType.ALIGNMENT)
    schema_template = ctx.prompts.schema(CallType.ALIGNMENT)
    calls = []
    for nodeid in nodeids:
        unit = units.unit(nodeid, arm)
        signatures = units.signatures(nodeid, arm, package)
        for rep in range(1, reps + 1):
            tokens = tokenize_criteria(criteria, ctx.run_id, nodeid, arm, rep)
            spec = CallSpec(
                call_id=ALIGNMENT_CALL_ID.format(judge=judge.value, arm=arm.value, slug=slugify(nodeid), rep=rep),
                pin=JUDGE_PIN[judge],
                call_type=CallType.ALIGNMENT,
                item=alignment_item(template, tokens.text, unit.source, signatures),
                schema_text=alignment_schema(schema_template, tokens.tokens),
                validate=alignment_problems(tokens.token_to_ref),
            )
            calls.append(AlignmentCall(judge, arm, nodeid, rep, spec, tokens.token_to_ref))
    return calls


def token_map_path(judge: Judge, arm: Arm, nodeid: str, rep: int) -> str:
    return f"{TOKEN_MAPS_DIRNAME}/{judge.value}/{arm.value}/{slugify(nodeid)}/rep-{rep}.json"


def run_alignment(
    ctx: PipelineContext, combos: Sequence[tuple[Judge, Arm]], nodeids: Sequence[str], package: str
) -> None:
    units = UnitBook(ctx)
    calls = [c for judge, arm in combos for c in build_alignment_calls(ctx, judge, arm, nodeids, units, package)]
    for call in calls:
        ctx.store.write_json(token_map_path(call.judge, call.arm, call.nodeid, call.rep), dict(call.token_map))
    label = ",".join(f"{judge.value}/{arm.value}" for judge, arm in combos)
    outcomes = ctx.caller.call_all(f"alignment:{label}", [c.spec for c in calls])
    rows = [_alignment_row(call, outcomes[call.spec.call_id]) for call in calls]
    _merge(ctx, ALIGNMENT_FILENAME, rows, ALIGNMENT_KEY)
    missing = sum(1 for row in rows if row["status"] == STATUS_MISSING)
    ctx.log(f"[alignment] {len(rows)} judgments ({missing} missing)")


def _alignment_row(call: AlignmentCall, outcome: Any) -> dict[str, Any]:
    base = {
        "judge": call.judge.value,
        "arm": call.arm.value,
        "nodeid": call.nodeid,
        "rep": call.rep,
        "call_id": call.spec.call_id,
    }
    if isinstance(outcome, MissingResult):
        return {**base, "status": STATUS_MISSING, "verifies_ref": None, "coverage": None,
                "confidence": None, "rationale": None}
    payload = outcome.payload
    return {
        **base,
        "status": STATUS_OK,
        "verifies_ref": call.token_map.get(payload["verifies"], NONE_ANSWER),
        "coverage": payload["coverage"],
        "confidence": payload["confidence"],
        "rationale": payload["rationale"],
    }


def run_probe(ctx: PipelineContext, judges: Sequence[Judge], nodeids: Sequence[str], reps: int = PROBE_REPS) -> None:
    units = UnitBook(ctx)
    template = ctx.prompts.template(CallType.PROBE)
    schema = ctx.prompts.schema(CallType.PROBE)
    jobs = [
        (judge, nodeid, rep, CallSpec(
            call_id=PROBE_CALL_ID.format(judge=judge.value, slug=slugify(nodeid), rep=rep),
            pin=JUDGE_PIN[judge],
            call_type=CallType.PROBE,
            item=probe_item(template, units.unit(nodeid, Arm.NAME_MASKED).source),
            schema_text=schema,
        ))
        for judge in judges
        for nodeid in nodeids
        for rep in range(1, reps + 1)
    ]
    outcomes = ctx.caller.call_all("probe", [spec for *_, spec in jobs])
    rows = []
    for judge, nodeid, rep, spec in jobs:
        outcome = outcomes[spec.call_id]
        base = {"judge": judge.value, "nodeid": nodeid, "rep": rep, "call_id": spec.call_id}
        if isinstance(outcome, MissingResult):
            rows.append({**base, "status": STATUS_MISSING, "guess": None, "confidence": None})
        else:
            rows.append({**base, "status": STATUS_OK, "guess": outcome.payload["guess"],
                         "confidence": outcome.payload["confidence"]})
    _merge(ctx, PROBE_FILENAME, rows, PROBE_KEY)
    ctx.log(f"[probe] {len(rows)} probes")


def _merge(ctx: PipelineContext, filename: str, rows: Sequence[Mapping[str, Any]], key: Sequence[str]) -> None:
    existing = ctx.store.read_jsonl(filename) if ctx.store.exists(filename) else []
    replaced = {tuple(row[k] for k in key) for row in rows}
    kept = [row for row in existing if tuple(row[k] for k in key) not in replaced]
    ctx.store.write_jsonl(filename, [*kept, *rows], key)


def aggregate_and_prove(
    ctx: PipelineContext, bindings: Mapping[str, str], criteria: Sequence[Criterion]
) -> None:
    """Aggregates every alignment row into verdicts (PREREG 7.4) and writes one Statement per verdict."""
    rows = ctx.store.read_jsonl(ALIGNMENT_FILENAME) if ctx.store.exists(ALIGNMENT_FILENAME) else []
    grouped: dict[tuple[str, str, str], list[Mapping[str, Any]]] = defaultdict(list)
    for row in rows:
        grouped[(row["judge"], row["arm"], row["nodeid"])].append(row)
    by_ref = {c.ref: c for c in criteria}
    aggregate: dict[str, dict[str, dict[str, Any]]] = {}
    for (judge, arm, nodeid), group in sorted(grouped.items()):
        result = aggregate_alignment(_repetitions(group), bindings[nodeid])
        aggregate.setdefault(judge, {}).setdefault(arm, {})[nodeid] = result.to_mapping()
        _write_proof(ctx, Judge(judge), Arm(arm), nodeid, bindings[nodeid], by_ref, result.verdict.value, result.modal_share)
    ctx.store.write_json(ALIGNMENT_AGGREGATE_FILENAME, aggregate)
    ctx.log(f"[alignment] {len(grouped)} aggregates and proofs written")


def _repetitions(group: Sequence[Mapping[str, Any]]) -> list[Repetition]:
    return [
        Repetition(row["verifies_ref"], row["coverage"], float(row["confidence"]))
        for row in group
        if row["status"] == STATUS_OK
    ]


def _write_proof(
    ctx: PipelineContext,
    judge: Judge,
    arm: Arm,
    nodeid: str,
    declared: str,
    criteria: Mapping[str, Criterion],
    verdict: str,
    modal_share: float,
) -> None:
    file = nodeid.split(TEST_FILE_SEPARATOR, 1)[0]
    template = ctx.prompts.template(CallType.ALIGNMENT)
    pin = JUDGE_PIN[judge]
    inputs = [
        text_resource(ROLE_SPEC, "spec", ctx.sources.spec_text()),
        text_resource(ROLE_TEST_SOURCE, file, ctx.sources.test_files()[file]),
        text_resource(ROLE_CODE_TREE, "src", files_digest(ctx.sources.source_files())),
        text_resource(ROLE_PROMPT_TEMPLATE, "alignment", template),
        text_resource(ROLE_MODEL_ID, pin.key, pin.model),
    ]
    statement = alignment_statement(
        criteria[declared], nodeid, verdict, modal_share, pin.model,
        text_resource(ROLE_PROMPT_TEMPLATE, "alignment", template).sha256, inputs,
        {"arm": arm.value, "judge": judge.value, "repetitions": ALIGNMENT_REPS},
    )
    ctx.store.write_json(
        f"{PROOFS_DIRNAME}/{ALIGNMENT_PROOFS_DIRNAME}/{judge.value}/{arm.value}/{slugify(nodeid)}{PROOF_SUFFIX}",
        statement,
    )
