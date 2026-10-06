"""Stage classes: mutant classes per criterion from the host votes, witnesses and kills (PREREG 6.5)."""

from __future__ import annotations

from collections import defaultdict
from collections.abc import Mapping
from typing import Any

from pilot.application.context import PipelineContext
from pilot.application.mutant_records import evaluated_mutant_ids, load_mutant
from pilot.application.validity_stage import STATUS_OK
from pilot.domain.arms import ARM_P, ARM_T
from pilot.domain.model_slate import VALIDITY_JUDGES, Judge
from pilot.domain.mutant_classes import JudgeFindings, MutantEvidence, classify_mutant
from pilot.domain.mutants import DifferentialKind
from pilot.domain.outcomes import KillResult
from pilot.domain.pipeline_constants import (
    BINDINGS_FILENAME,
    DIFFERENTIAL_FILENAME,
    KILLS_FILENAME,
    MUTANT_CLASSES_FILENAME,
    VALIDITY_FILENAME,
    WITNESS_FILENAME,
)
from pilot.domain.validity_payload import parse_validity

CLASSIFIED_ARMS = (ARM_T, ARM_P)


def classify_mutants(ctx: PipelineContext) -> list[dict[str, Any]]:
    criteria = [c.ref for c in ctx.criteria()]
    kills = {row["mutant_id"]: row["results"] for row in ctx.store.read_jsonl(KILLS_FILENAME)}
    differential = {row["mutant_id"]: row["status"] for row in ctx.store.read_jsonl(DIFFERENTIAL_FILENAME)}
    validity = _validity_rows(ctx)
    confirmed = _confirmed_witnesses(ctx)
    bindings = ctx.store.read_json(BINDINGS_FILENAME)
    classes = []
    for mid in evaluated_mutant_ids(ctx.store, CLASSIFIED_ARMS):
        document = load_mutant(ctx.store, mid)
        evidence = MutantEvidence(
            mutant_id=mid,
            arm=document["arm"],
            target=document["target"],
            kills={nodeid: KillResult(value) for nodeid, value in kills[mid].items()},
            differential=DifferentialKind(differential[mid]),
            judges=[_findings(validity, confirmed, mid, judge) for judge in VALIDITY_JUDGES],
            bindings=bindings,
        )
        classes.append(classify_mutant(evidence, criteria).to_mapping())
    ctx.store.write_json(MUTANT_CLASSES_FILENAME, classes)
    ctx.log(f"[classes] {len(classes)} mutants classified")
    return classes


def _validity_rows(ctx: PipelineContext) -> dict[tuple[str, str], Mapping[str, Any]]:
    rows = ctx.store.read_jsonl(VALIDITY_FILENAME) if ctx.store.exists(VALIDITY_FILENAME) else []
    return {(row["mutant_id"], row["judge"]): row for row in rows if row["status"] == STATUS_OK}


def _confirmed_witnesses(ctx: PipelineContext) -> dict[tuple[str, str], frozenset[str]]:
    rows = ctx.store.read_jsonl(WITNESS_FILENAME) if ctx.store.exists(WITNESS_FILENAME) else []
    grouped: dict[tuple[str, str], set[str]] = defaultdict(set)
    for row in rows:
        if row["confirmed"]:
            grouped[(row["mutant_id"], row["judge"])].add(row["criterion"])
    return {key: frozenset(refs) for key, refs in grouped.items()}


def _findings(
    validity: Mapping[tuple[str, str], Mapping[str, Any]],
    confirmed: Mapping[tuple[str, str], frozenset[str]],
    mutant_id: str,
    judge: Judge,
) -> JudgeFindings:
    row = validity.get((mutant_id, judge.value))
    if row is None:
        return JudgeFindings(None)
    return JudgeFindings(parse_validity(row["payload"]), confirmed.get((mutant_id, judge.value), frozenset()))
