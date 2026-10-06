"""Blind relabels by two model raters, one call each over all tests (PREREG 3.4)."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any

from pilot.application.context import PipelineContext
from pilot.application.model_caller import CallSpec
from pilot.domain.call_type import CallType
from pilot.domain.model_call import MissingResult
from pilot.domain.model_slate import JUDGE_PIN, RELABEL_RATERS, Judge
from pilot.domain.pipeline_constants import RELABEL_FILENAME
from pilot.domain.prompts import relabel_item
from pilot.domain.relabel_input import relabel_problems, render_tests

CALL_ID_TEMPLATE = "relabel__{rater}"
RELABEL_KEY = ("rater", "nodeid")


def run_relabel(ctx: PipelineContext, bindings: Mapping[str, str]) -> list[Judge]:
    """One call per rater with codebook, spec, full source and every test; returns raters with no answer."""
    item = relabel_item(
        ctx.prompts.template(CallType.RELABEL),
        ctx.prompts.codebook(),
        ctx.sources.spec_text(),
        ctx.sources.source_files(),
        render_tests(ctx.sources.test_files(), bindings),
    )
    check = relabel_problems(sorted(bindings))
    specs = {
        rater: CallSpec(
            call_id=CALL_ID_TEMPLATE.format(rater=rater.value),
            pin=JUDGE_PIN[rater],
            call_type=CallType.RELABEL,
            item=item,
            schema_text=ctx.prompts.schema(CallType.RELABEL),
            validate=check,
        )
        for rater in RELABEL_RATERS
    }
    outcomes = ctx.caller.call_all("relabel", list(specs.values()))
    rows: list[dict[str, Any]] = []
    unanswered = []
    for rater, spec in specs.items():
        outcome = outcomes[spec.call_id]
        if isinstance(outcome, MissingResult):
            unanswered.append(rater)
            continue
        rows += [
            {"rater": rater.value, "nodeid": t["nodeid"], "label": t["label"], "checks": t["checks"]}
            for t in outcome.payload["tests"]
        ]
    ctx.store.write_jsonl(RELABEL_FILENAME, rows, RELABEL_KEY)
    ctx.log(f"[relabel] {len(rows)} labels from {len(RELABEL_RATERS) - len(unanswered)}/{len(RELABEL_RATERS)} raters")
    return unanswered
