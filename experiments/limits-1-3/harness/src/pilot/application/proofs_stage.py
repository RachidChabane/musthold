"""Discriminating-power proofs per criterion from arm T (PREREG 6.5, ADR 0001)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from typing import Any

from pilot.application.context import PipelineContext
from pilot.application.mutant_records import load_mutant
from pilot.application.validity_stage import STATUS_OK
from pilot.domain.arms import ARM_T
from pilot.domain.call_type import CallType
from pilot.domain.edits import EditSet
from pilot.domain.ids import mutant_digest
from pilot.domain.model_slate import VALIDITY_JUDGES
from pilot.domain.outcomes import KillResult
from pilot.domain.pipeline_constants import (
    DISCRIMINATING_POWER_PROOFS_DIRNAME,
    KILLS_FILENAME,
    MUTANT_CLASSES_FILENAME,
    PROOF_SUFFIX,
    PROOFS_DIRNAME,
    REFERENCE_FILENAME,
    VALIDITY_FILENAME,
)
from pilot.domain.proofs import (
    ROLE_CODE_TREE,
    ROLE_MODEL_ID,
    ROLE_MUTANT_SET,
    ROLE_PROMPT_TEMPLATE,
    ROLE_SPEC,
    ROLE_TEST_SOURCE,
    MutantProof,
    discriminating_power_statement,
    files_digest,
    text_resource,
    validity_claim,
)
from pilot.domain.spec import Criterion
from pilot.domain.validity_payload import ValidityVerdict, parse_validity

PROOF_RESULT_SURVIVED = "survived"


def write_discriminating_power_proofs(ctx: PipelineContext, bindings: Mapping[str, str]) -> None:
    classes = {c["mutant_id"]: c for c in ctx.store.read_json(MUTANT_CLASSES_FILENAME)}
    kills = {row["mutant_id"]: row["results"] for row in ctx.store.read_jsonl(KILLS_FILENAME)}
    verdicts = _verdicts(ctx)
    reference = ctx.store.read_json(REFERENCE_FILENAME)
    for criterion in ctx.criteria():
        statement = _statement_for(ctx, criterion, bindings, classes, kills, verdicts, reference)
        path = f"{PROOFS_DIRNAME}/{DISCRIMINATING_POWER_PROOFS_DIRNAME}/{criterion.ref}{PROOF_SUFFIX}"
        ctx.store.write_json(path, statement)
    ctx.log(f"[proofs] {len(ctx.criteria())} discriminating-power statements written")


def _verdicts(ctx: PipelineContext) -> dict[tuple[str, str], ValidityVerdict]:
    rows = ctx.store.read_jsonl(VALIDITY_FILENAME) if ctx.store.exists(VALIDITY_FILENAME) else []
    return {(r["mutant_id"], r["judge"]): parse_validity(r["payload"]) for r in rows if r["status"] == STATUS_OK}


def _statement_for(
    ctx: PipelineContext,
    criterion: Criterion,
    bindings: Mapping[str, str],
    classes: Mapping[str, Mapping[str, Any]],
    kills: Mapping[str, Mapping[str, str]],
    verdicts: Mapping[tuple[str, str], ValidityVerdict],
    reference: Mapping[str, Any],
) -> dict[str, Any]:
    tests = sorted(nodeid for nodeid, ref in bindings.items() if ref == criterion.ref)
    own = [mid for mid, c in sorted(classes.items()) if c["arm"] == ARM_T and c["target"] == criterion.ref]
    documents = {mid: load_mutant(ctx.store, mid) for mid in own}
    proofs = [_mutant_proof(classes[mid], documents[mid], verdicts, criterion.ref, kills[mid], tests) for mid in own]
    passes = all(run[n] == "passed" for run in reference["runs"] for n in tests)
    generators = [documents[mid]["generator"]["model"] for mid in own]
    return discriminating_power_statement(
        criterion, tests, passes, proofs, _inputs(ctx, proofs, generators), generators
    )


def _mutant_proof(
    mutant_class: Mapping[str, Any],
    document: Mapping[str, Any],
    verdicts: Mapping[tuple[str, str], ValidityVerdict],
    ref: str,
    kills: Mapping[str, str],
    tests: Sequence[str],
) -> MutantProof:
    judges = [verdicts.get((mutant_class["mutant_id"], judge.value)) for judge in VALIDITY_JUDGES]
    validity, confidence = validity_claim(mutant_class, ref, judges)
    return MutantProof(
        digest=mutant_digest(EditSet.from_mapping({"edits": document["edits"]})),
        validity=validity,
        validity_confidence=confidence,
        results={nodeid: _proof_result(kills[nodeid]) for nodeid in tests},
        generator_model=document["generator"]["model"],
    )


def _proof_result(result: str) -> str:
    return PROOF_RESULT_SURVIVED if KillResult(result) is KillResult.FLAKY else result


def _inputs(ctx: PipelineContext, proofs: Sequence[MutantProof], generators: Sequence[str]) -> list[Any]:
    resources = [
        text_resource(ROLE_SPEC, "spec", ctx.sources.spec_text()),
        text_resource(ROLE_TEST_SOURCE, "tests", files_digest(ctx.sources.test_files())),
        text_resource(ROLE_CODE_TREE, "src", files_digest(ctx.sources.source_files())),
        text_resource(ROLE_MUTANT_SET, "mutants", "\n".join(sorted(p.digest for p in proofs))),
        text_resource(ROLE_PROMPT_TEMPLATE, "generate", ctx.prompts.template(CallType.GENERATE)),
        text_resource(ROLE_PROMPT_TEMPLATE, "validity", ctx.prompts.template(CallType.VALIDITY)),
    ]
    resources += [text_resource(ROLE_MODEL_ID, model, model) for model in sorted(set(generators))]
    return resources
