from __future__ import annotations

import dataclasses

import jsonschema
import pytest

from specbench.adapters.schemas import SchemaName, load_schema, schema_problems
from specbench.domain.arms import FROZEN_MODEL, Harness
from specbench.domain.canonical import JsonValue
from specbench.domain.jsonread import RecordError
from specbench.domain.languages import Language
from specbench.domain.manifest import Manifest, parse_manifest
from specbench.domain.sealing import CorpusLock, lock_entry_for, parse_lock
from specbench.domain.task import task_from_record
from support.builders import DIGEST, TASK_ID, make_bundle, make_task, task_json


def manifest() -> Manifest:
    return Manifest(
        run_id="sample-claude-01",
        seed=20261007,
        prereg_tag="prereg-sample-1",
        product_release_digest=DIGEST,
        shallnot_commit="c" * 40,
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
        {"criteria": []},
        {"licence": {"spdx_id": "GPL-3.0-only", "path": "LICENSE", "notice_path": None}},
        {"schema": "specbench.task/2"},
    ],
)
def test_the_schema_and_the_parser_refuse_the_same_documents(mutation: dict[str, JsonValue]) -> None:
    record = {**make_task().to_record(), **mutation}
    assert schema_problems(SchemaName.TASK, record) != []
    with pytest.raises(RecordError):
        task_from_record(record)


def test_manifest_conforms_and_round_trips() -> None:
    value = manifest()
    assert schema_problems(SchemaName.MANIFEST, value.to_record()) == []
    assert parse_manifest(value.to_canonical_json()) == value


def test_manifest_refuses_a_model_other_than_the_frozen_one() -> None:
    record = {**manifest().to_record(), "harness_models": {"claude-code": "claude-opus-5-5"}}
    assert schema_problems(SchemaName.MANIFEST, record) != []
    with pytest.raises(RecordError):
        dataclasses.replace(manifest(), harness_models={Harness.CODEX: "gpt-5.6-terra"})


def test_corpus_lock_conforms_and_round_trips() -> None:
    lock = CorpusLock((lock_entry_for(TASK_ID, Language.PYTHON, task_json(), make_bundle()),))
    assert schema_problems(SchemaName.CORPUS_LOCK, lock.to_record()) == []
    assert parse_lock(lock.to_canonical_json()) == lock
