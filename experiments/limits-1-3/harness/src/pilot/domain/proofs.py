"""ADR 0001 proofs: in-toto Statement v1 with project-owned predicates (alignment, discriminating power)."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Any

from pilot.domain.ids import canonical_json, sha256_hex
from pilot.domain.pipeline_constants import (
    CLASS_EXECUTED,
    CLASS_JUDGED,
    MODULE_NAME,
    MODULE_VERSION,
    PREDICATE_ALIGNMENT,
    PREDICATE_DISCRIMINATING_POWER,
    PRODUCED_BY_LOCAL,
    STATEMENT_CANONICALIZATION,
    STATEMENT_TYPE,
)
from pilot.domain.mutant_classes import CriterionClass
from pilot.domain.spec import Criterion, statement_digest
from pilot.domain.validity_payload import ValidityVerdict

ROLE_SPEC = "spec"
ROLE_TEST_SOURCE = "test_source"
ROLE_CODE_TREE = "code_tree"
ROLE_MUTANT_SET = "mutant_set"
ROLE_MODEL_ID = "model_id"
ROLE_PROMPT_TEMPLATE = "prompt_template"


@dataclass(frozen=True)
class Resource:
    role: str
    name: str
    sha256: str

    def to_mapping(self) -> dict[str, Any]:
        return {"role": self.role, "name": self.name, "digest": {"sha256": self.sha256}}


def text_resource(role: str, name: str, content: str) -> Resource:
    return Resource(role, name, sha256_hex(content))


def files_digest(files: Mapping[str, str]) -> str:
    """Digest of a file set: SHA-256 over sorted `path NUL content NUL` records."""
    return sha256_hex("".join(f"{path}\0{files[path]}\0" for path in sorted(files)))


def judged(value: Any, confidence: float) -> dict[str, Any]:
    return {"value": value, "class": CLASS_JUDGED, "confidence": confidence}


def executed(value: Any) -> dict[str, Any]:
    return {"value": value, "class": CLASS_EXECUTED}


def _subject(criterion: Criterion) -> list[dict[str, Any]]:
    return [{"name": criterion.ref, "digest": {"sha256": statement_digest(criterion.statement)}}]


def _requirement(criterion: Criterion) -> dict[str, Any]:
    return {
        "id": criterion.id,
        "revision": criterion.revision,
        "statementCanonicalization": STATEMENT_CANONICALIZATION,
    }


def _statement(
    predicate_type: str,
    criterion: Criterion,
    inputs: Sequence[Resource],
    body: Mapping[str, Any],
    extensions: Mapping[str, Any],
) -> dict[str, Any]:
    subject = _subject(criterion)
    sorted_inputs = [r.to_mapping() for r in sorted(inputs, key=lambda r: (r.role, r.name))]
    predicate = {
        "requirement": _requirement(criterion),
        "module": {"name": MODULE_NAME, "version": MODULE_VERSION},
        "inputs": sorted_inputs,
        "cacheKey": sha256_hex(canonical_json({"subject": subject, "inputs": sorted_inputs})),
        "producedBy": PRODUCED_BY_LOCAL,
        "extensions": dict(extensions),
        **body,
    }
    return {
        "_type": STATEMENT_TYPE,
        "subject": subject,
        "predicateType": predicate_type,
        "predicate": predicate,
    }


def alignment_statement(
    criterion: Criterion,
    nodeid: str,
    verdict: str,
    modal_share: float,
    judge_model: str,
    prompt_digest: str,
    inputs: Sequence[Resource],
    extensions: Mapping[str, Any],
) -> dict[str, Any]:
    body = {
        "test": nodeid,
        "verdict": judged(verdict, modal_share),
        "judge": {"model": judge_model, "promptDigest": prompt_digest, "blind": True},
    }
    return _statement(PREDICATE_ALIGNMENT, criterion, inputs, body, extensions)


@dataclass(frozen=True)
class MutantProof:
    digest: str
    validity: str
    validity_confidence: float
    results: Mapping[str, str]
    generator_model: str

    def to_mapping(self) -> dict[str, Any]:
        return {
            "digest": self.digest,
            "validity": judged(self.validity, self.validity_confidence),
            "results": {nodeid: executed(result) for nodeid, result in sorted(self.results.items())},
            "generator": {"model": self.generator_model},
        }


def discriminating_power_statement(
    criterion: Criterion,
    tests: Sequence[str],
    correct_code_passes: bool,
    mutants: Sequence[MutantProof],
    inputs: Sequence[Resource],
    generators: Sequence[str],
) -> dict[str, Any]:
    body = {
        "tests": sorted(tests),
        "correctCodePasses": executed(correct_code_passes),
        "mutants": [m.to_mapping() for m in sorted(mutants, key=lambda m: m.digest)],
    }
    return _statement(
        PREDICATE_DISCRIMINATING_POWER,
        criterion,
        inputs,
        body,
        {"mutantGenerators": sorted(set(generators))},
    )


VALIDITY_VALID_W = "VALID_W"
VALIDITY_VALID_X = "VALID_X"
VALIDITY_EQUIVALENT = "EQUIVALENT"
VALIDITY_NOT_VIOLATING = "NOT_VIOLATING"
VALIDITY_UNDETERMINED = "UNDETERMINED"
UNKNOWN_CONFIDENCE = 0.0


def validity_claim(
    mutant_class: Mapping[str, Any], ref: str, judges: Sequence[ValidityVerdict | None]
) -> tuple[str, float]:
    """Validity of a mutant for one criterion as a judged claim: class and the minimum judge confidence."""
    per = mutant_class["per_criterion"][ref]
    if per == CriterionClass.VALID_W.value:
        claim = VALIDITY_VALID_X if ref in mutant_class["valid_x"] else VALIDITY_VALID_W
        return claim, float(mutant_class["valid_confidence"][ref])
    if mutant_class["equivalent"]:
        return VALIDITY_EQUIVALENT, _minimum(v.equivalent_confidence if v else None for v in judges)
    confidence = _minimum(v.requirements[ref].confidence if v else None for v in judges)
    return (VALIDITY_NOT_VIOLATING if per == VALIDITY_NOT_VIOLATING else VALIDITY_UNDETERMINED), confidence


def _minimum(values: Any) -> float:
    present = list(values)
    return UNKNOWN_CONFIDENCE if not present or any(v is None for v in present) else float(min(present))
