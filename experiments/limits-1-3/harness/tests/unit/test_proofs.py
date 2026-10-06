from __future__ import annotations

import hashlib
import json

import rfc8785

from pilot.domain.ids import canonical_json
from pilot.domain.proofs import (
    MutantProof,
    alignment_statement,
    discriminating_power_statement,
    files_digest,
    text_resource,
    validity_claim,
)
from pilot.domain.spec import Criterion, statement_digest
from pilot.domain.validity_payload import CriterionVerdict, ValidityVerdict, Verdict

CRITERION = Criterion("PARK-3", 1, "WHEN a stay lasts more than 15 minutes THE SYSTEM SHALL charge 2.50.")
INPUTS = [text_resource("spec", "spec", "S"), text_resource("code_tree", "src", "C")]


def alignment():
    return alignment_statement(CRITERION, "tests/t.py::test_x", "ALIGNED", 0.8, "claude-sonnet-5-5", "d" * 64, INPUTS, {"arm": "NAME-MASKED"})


def power(mutants=None):
    mutants = mutants if mutants is not None else [
        MutantProof("b" * 64, "VALID_X", 0.85, {"t::b": "killed", "t::a": "survived"}, "claude-opus-5-5"),
        MutantProof("a" * 64, "NOT_VIOLATING", 0.9, {"t::a": "error"}, "gpt-5.6-terra"),
    ]
    return discriminating_power_statement(CRITERION, ["t::b", "t::a"], True, mutants, INPUTS, ["gpt-5.6-terra", "claude-opus-5-5", "gpt-5.6-terra"])


def test_alignment_statement_is_an_in_toto_v1_statement_with_the_criterion_as_subject():
    statement = alignment()
    assert statement["_type"] == "https://in-toto.io/Statement/v1"
    assert statement["predicateType"] == "https://rachid-chabane.com/attestations/alignment/v1"
    assert statement["subject"] == [{"name": "PARK-3~1", "digest": {"sha256": statement_digest(CRITERION.statement)}}]
    predicate = statement["predicate"]
    assert predicate["requirement"] == {"id": "PARK-3", "revision": 1, "statementCanonicalization": "c1"}
    assert predicate["verdict"] == {"value": "ALIGNED", "class": "judged", "confidence": 0.8}
    assert predicate["judge"] == {"model": "claude-sonnet-5-5", "promptDigest": "d" * 64, "blind": True}
    assert predicate["producedBy"] == "local" and predicate["test"] == "tests/t.py::test_x"


def test_discriminating_power_statement_classes_every_claim():
    predicate = power()["predicate"]
    assert power()["predicateType"] == "https://rachid-chabane.com/attestations/discriminating-power/v1"
    assert predicate["correctCodePasses"] == {"value": True, "class": "executed"}
    assert predicate["extensions"] == {"mutantGenerators": ["claude-opus-5-5", "gpt-5.6-terra"]}
    first = predicate["mutants"][0]
    assert first["digest"] == "a" * 64 and first["validity"]["class"] == "judged" and "confidence" in first["validity"]
    assert all(r["class"] == "executed" and "confidence" not in r for m in predicate["mutants"] for r in m["results"].values())
    assert first["generator"] == {"model": "gpt-5.6-terra"} and "edits" not in first


def test_arrays_are_sorted_and_inputs_by_role_then_name():
    predicate = power()["predicate"]
    assert predicate["tests"] == ["t::a", "t::b"]
    assert [m["digest"] for m in predicate["mutants"]] == ["a" * 64, "b" * 64]
    assert [i["role"] for i in predicate["inputs"]] == ["code_tree", "spec"]


def test_serialization_is_deterministic_jcs_without_timestamps_or_host_names():
    assert canonical_json(power()) == canonical_json(power()) == rfc8785.dumps(power())
    text = canonical_json(power()).decode()
    assert json.loads(text)["_type"] == "https://in-toto.io/Statement/v1"
    assert "/Users" not in text and "timestamp" not in text.lower()


def test_cache_key_is_the_sha256_of_the_jcs_of_subject_and_inputs_and_follows_the_inputs():
    statement = power()
    predicate = statement["predicate"]
    expected = hashlib.sha256(canonical_json({"subject": statement["subject"], "inputs": predicate["inputs"]})).hexdigest()
    assert predicate["cacheKey"] == expected
    other = discriminating_power_statement(CRITERION, ["t::a"], True, [], [text_resource("spec", "spec", "changed")], [])
    assert other["predicate"]["cacheKey"] != expected


def test_a_statement_edit_changes_the_subject_digest_and_a_reformat_does_not():
    edited = Criterion("PARK-3", 1, CRITERION.statement.replace("2.50", "2.60"))
    reformatted = Criterion("PARK-3", 1, "  " + CRITERION.statement.replace(" ", "  "))
    assert alignment_statement(edited, "n", "ALIGNED", 1.0, "m", "d", INPUTS, {})["subject"] != alignment()["subject"]
    assert alignment_statement(reformatted, "n", "ALIGNED", 1.0, "m", "d", INPUTS, {})["subject"] == alignment()["subject"]


def test_files_digest_depends_on_paths_and_contents_not_on_insertion_order():
    assert files_digest({"b": "2", "a": "1"}) == files_digest({"a": "1", "b": "2"})
    assert files_digest({"a": "1"}) != files_digest({"a": "2"}) != files_digest({"c": "1"})


def mutant_class(per, equivalent=False, confidence=None, valid_x=()):
    return {"per_criterion": {"PARK-3~1": per}, "equivalent": equivalent, "valid_confidence": confidence or {}, "valid_x": list(valid_x)}


def verdicts(violated_confidence=0.8, equivalent_confidence=0.75):
    one = ValidityVerdict(True, equivalent_confidence, {"PARK-3~1": CriterionVerdict(Verdict.NOT_VIOLATED, violated_confidence, None)})
    two = ValidityVerdict(True, 0.95, {"PARK-3~1": CriterionVerdict(Verdict.NOT_VIOLATED, 0.99, None)})
    return [one, two]


def test_validity_claims_carry_the_minimum_judge_confidence():
    assert validity_claim(mutant_class("VALID_W", confidence={"PARK-3~1": 0.8}, valid_x=["PARK-3~1"]), "PARK-3~1", verdicts()) == ("VALID_X", 0.8)
    assert validity_claim(mutant_class("VALID_W", confidence={"PARK-3~1": 0.8}), "PARK-3~1", verdicts()) == ("VALID_W", 0.8)
    assert validity_claim(mutant_class("NOT_VIOLATING"), "PARK-3~1", verdicts()) == ("NOT_VIOLATING", 0.8)
    assert validity_claim(mutant_class("UNDETERMINED", equivalent=True), "PARK-3~1", verdicts()) == ("EQUIVALENT", 0.75)
    assert validity_claim(mutant_class("UNDETERMINED"), "PARK-3~1", [verdicts()[0], None]) == ("UNDETERMINED", 0.0)
