from __future__ import annotations

import json

import pytest

from calibration.application.manifest import RunManifest
from calibration.domain.canonical import document_json
from calibration.domain.constants import MANIFEST_FILE
from calibration.domain.jsonread import RecordError
from support.build_fakes import InMemoryRunStore


def test_a_run_without_a_manifest_reads_as_empty() -> None:
    assert RunManifest(InMemoryRunStore()).read() == {}


def test_setting_sections_creates_the_manifest_as_a_document() -> None:
    store = InMemoryRunStore()

    RunManifest(store).set_sections({"run_id": "run-01", "counts": {"items": 2}})

    assert store.read_text(MANIFEST_FILE) == document_json({"run_id": "run-01", "counts": {"items": 2}})


def test_a_new_section_keeps_the_sections_other_verbs_wrote() -> None:
    store = InMemoryRunStore()
    manifest = RunManifest(store)
    manifest.set_sections({"run_id": "run-01", "harness_commit": "abc"})

    manifest.set_sections({"servers": {"M1": "0.35.1"}})

    assert manifest.read() == {"run_id": "run-01", "harness_commit": "abc", "servers": {"M1": "0.35.1"}}


def test_a_section_is_replaced_as_a_whole_and_not_merged() -> None:
    store = InMemoryRunStore()
    manifest = RunManifest(store)
    manifest.set_sections({"counts": {"alignment_items": 2, "screen_items": 5}})

    manifest.set_sections({"counts": {"alignment_items": 3}})

    assert json.loads(store.read_text(MANIFEST_FILE))["counts"] == {"alignment_items": 3}


def test_a_manifest_that_is_not_an_object_is_refused() -> None:
    store = InMemoryRunStore({MANIFEST_FILE: "[]"})

    with pytest.raises(RecordError):
        RunManifest(store).read()
