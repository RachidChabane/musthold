from __future__ import annotations

import dataclasses
from typing import Any

import jsonschema
import pytest

from specbench.adapters.schemas import SchemaName, load_schema, schema_problems
from specbench.domain.arms import FROZEN_MODEL, Harness
from specbench.domain.canonical import JsonValue, canonical_json
from specbench.domain.jsonread import RecordError
from specbench.domain.languages import Language
from specbench.domain.manifest import CODING_AGENT_CLIS, PRODUCT_ARTIFACTS, Manifest, Provenance, parse_manifest
from specbench.domain.sealing import CorpusLock, lock_entry_for, parse_lock
from specbench.domain.task import task_from_record
from support.builders import DIGEST, PROVENANCE, TASK_ID, make_bundle, make_task, task_json


def manifest() -> Manifest:
    return Manifest(
        run_id="sample-claude-01",
        seed=20261007,
        prereg_tag="prereg-sample-1",
        provenance=PROVENANCE,
        corpus_lock_digest=DIGEST,
        prices_lock_digest=DIGEST,
        os_name="darwin",
        toolchains={"python": "3.13.14", "go": "1.26.1"},
        harness_models={Harness.CLAUDE_CODE: FROZEN_MODEL[Harness.CLAUDE_CODE]},
        sample_tasks=(TASK_ID,),
    )


def test_every_schema_is_itself_a_valid_draft_2020_12_schema() -> None:
    for name in SchemaName:
        jsonschema.Draft202012Validator.check_schema(load_schema(name))


def test_task_records_conform_to_the_task_schema() -> None:
    for language in Language:
        record = make_task(language=language).to_record()
        assert schema_problems(SchemaName.TASK, record) == []
        task_from_record(record)


@pytest.mark.parametrize(
    "mutation",
    [
        {"extra": 1},
        {"language": "rust"},
        {"base_sha": "short"},
        {"licence": {"spdx_id": "GPL-3.0-only", "path": "LICENSE", "notice_path": None}},
        {"schema": "specbench.task/2"},
    ],
)
def test_the_schema_and_the_parser_refuse_the_same_documents(mutation: dict[str, JsonValue]) -> None:
    record = {**make_task().to_record(), **mutation}
    assert schema_problems(SchemaName.TASK, record) != []
    with pytest.raises(RecordError):
        task_from_record(record)


def test_a_selected_task_without_criteria_conforms_to_the_schema_and_the_parser() -> None:
    record = make_task(criteria=()).to_record()
    assert schema_problems(SchemaName.TASK, record) == []
    assert not task_from_record(record).is_constructed


def test_manifest_conforms_and_round_trips() -> None:
    value = manifest()
    assert schema_problems(SchemaName.MANIFEST, value.to_record()) == []
    assert parse_manifest(value.to_canonical_json()) == value


def test_the_manifest_schema_names_exactly_the_domains_product_artifacts_and_clis() -> None:
    properties = load_schema(SchemaName.MANIFEST)["properties"]
    assert tuple(properties["product_digests"]["required"]) == tuple(sorted(PRODUCT_ARTIFACTS))
    assert tuple(properties["cli_versions"]["required"]) == tuple(sorted(CODING_AGENT_CLIS))


def test_a_manifest_missing_a_product_digest_or_a_cli_version_is_refused_by_the_schema_and_the_parser() -> None:
    record = manifest().to_record()
    digests = {k: v for k, v in PROVENANCE.product_digests.items() if k != "musthold-s1"}
    versions = {k: v for k, v in PROVENANCE.cli_versions.items() if k != "codex"}
    for field, remaining in (("product_digests", digests), ("cli_versions", versions)):
        broken: dict[str, JsonValue] = {**record, field: remaining}
        assert schema_problems(SchemaName.MANIFEST, broken) != []
        with pytest.raises(RecordError):
            parse_manifest(canonical_json(broken))


def test_a_malformed_provenance_is_refused() -> None:
    with pytest.raises(RecordError):
        Provenance({**PROVENANCE.product_digests, "musthold-s1": "short"}, "c" * 40, PROVENANCE.cli_versions)
    with pytest.raises(RecordError):
        Provenance(PROVENANCE.product_digests, "not-a-sha", PROVENANCE.cli_versions)
    with pytest.raises(RecordError):
        Provenance(PROVENANCE.product_digests, "c" * 40, {"claude": "1.0.0", "codex": ""})


def test_the_manifest_states_that_the_budget_is_counted_in_model_executions() -> None:
    record = manifest().to_record()
    assert record["budget_unit"] == "model-execution"
    assert schema_problems(SchemaName.MANIFEST, record) == []
    for unit in ("call", "cli-invocation", None):
        changed = {**record, "budget_unit": unit}
        assert schema_problems(SchemaName.MANIFEST, changed) != []
        with pytest.raises(RecordError):
            parse_manifest(canonical_json(changed))
    without = {key: value for key, value in record.items() if key != "budget_unit"}
    assert schema_problems(SchemaName.MANIFEST, without) != []


def test_manifest_refuses_a_model_other_than_the_frozen_one() -> None:
    record = {**manifest().to_record(), "harness_models": {"claude-code": "claude-opus-5-5"}}
    assert schema_problems(SchemaName.MANIFEST, record) != []
    with pytest.raises(RecordError):
        dataclasses.replace(manifest(), harness_models={Harness.CODEX: "gpt-5.6-terra"})


def test_corpus_lock_conforms_and_round_trips() -> None:
    lock = CorpusLock((lock_entry_for(TASK_ID, Language.PYTHON, task_json(), make_bundle()),))
    assert schema_problems(SchemaName.CORPUS_LOCK, lock.to_record()) == []
    assert parse_lock(lock.to_canonical_json()) == lock


def test_the_trunk_schema_accepts_a_local_verdict_beside_the_verdict_and_refuses_an_unknown_one() -> None:
    body: dict[str, JsonValue] = {
        "model": FROZEN_MODEL[Harness.CLAUDE_CODE],
        "selection_pass": True,
        "verdict": "escalate",
        "local_verdict": "pass",
        "escalated_during": False,
        "final": None,
        "model_calls": 3,
        "seconds": 5,
    }
    arm_schema = {"$ref": "#/$defs/arm", "$defs": load_schema(SchemaName.TRUNK)["$defs"]}
    validator: Any = jsonschema.Draft202012Validator(arm_schema)
    assert list(validator.iter_errors(body)) == []
    assert list(validator.iter_errors({**body, "local_verdict": None})) == []
    assert list(validator.iter_errors({**body, "local_verdict": "maybe"})) != []
