from __future__ import annotations

import json
from datetime import UTC, datetime

import pytest

from pilot.adapters.run_directory import RunDirectory
from pilot.application.assemble_arm_p import DuplicateArmPItem, assemble_items
from pilot.application.arm_p import ArmPIntegrityError, planted_mutants
from pilot.application.provenance import ManifestInputs, ensure_manifest, log_deviation
from pilot.application.stage_timer import StageTimer
from pilot.domain.deviations import DeviationEntry, parse_deviations
from pilot.domain.edits import Edit, EditSet
from pilot.domain.ids import mutant_id
from pilot.domain.model_call import Cli
from pilot.domain.model_slate import OPUS, TERRA


def edit_set(body):
    return EditSet((Edit("src/p/m.py", "f", f"def f():\n    return {body}\n"),))


def test_run_directory_writes_sorted_key_json_and_sorted_jsonl(tmp_path):
    store = RunDirectory(tmp_path)
    store.write_json("a/b.json", {"z": 1, "a": {"y": 2, "b": 3}})
    assert (tmp_path / "a" / "b.json").read_text() == '{\n  "a": {\n    "b": 3,\n    "y": 2\n  },\n  "z": 1\n}\n'
    store.write_jsonl("rows.jsonl", [{"k": "b", "n": 2}, {"k": "a", "n": 3}, {"k": "a", "n": 1}, {"k": None, "n": 0}], ("k", "n"))
    assert [json.loads(line) for line in (tmp_path / "rows.jsonl").read_text().splitlines()] == [
        {"k": "a", "n": 1}, {"k": "a", "n": 3}, {"k": "b", "n": 2}, {"k": None, "n": 0},
    ]
    assert store.read_jsonl("rows.jsonl")[0] == {"k": "a", "n": 1}


def test_run_directory_writes_atomically_and_appends(tmp_path):
    store = RunDirectory(tmp_path)
    store.write_text("f.txt", "one")
    store.append_text("f.txt", "-two")
    assert store.read_text("f.txt") == "one-two" and store.list_files(".") == ["f.txt"]
    assert not store.exists("missing") and store.list_files("nowhere") == []


class FixedProvenance:
    def __init__(self, commit="c1", harness="h1"):
        self.commit, self.harness = commit, harness

    def prereg_commit(self):
        return self.commit

    def harness_digest(self):
        return self.harness

    def fixture_digest(self):
        return "f1"


class VersionBook:
    def __init__(self, **versions):
        self.versions = versions

    def installed_version(self, cli):
        return self.versions[cli.value]


INPUTS = ManifestInputs("r1", [OPUS, TERRA], {Cli.CLAUDE: "2.1.292", Cli.CODEX: "0.155.1"})


def test_manifest_records_commit_digests_versions_and_models(tmp_path):
    store = RunDirectory(tmp_path)
    manifest = ensure_manifest(store, FixedProvenance(), VersionBook(claude="2.1.292", codex="0.155.1"), INPUTS)
    assert manifest == {
        "run_id": "r1", "prereg_commit": "c1", "harness_digest": "h1", "fixture_digest": "f1",
        "cli_versions": {"claude": "2.1.292", "codex": "0.155.1"},
        "models": [
            {"model": "claude-opus-5-5", "cli": "claude", "provenance": "reported"},
            {"model": "gpt-5.6-terra", "cli": "codex", "provenance": "requested_only"},
        ],
    }
    assert store.read_text("DEVIATIONS.md").strip() == "# Deviations"


def test_a_cli_version_different_from_the_lock_is_a_provenance_record_not_fatal(tmp_path):
    store = RunDirectory(tmp_path)
    ensure_manifest(store, FixedProvenance(), VersionBook(claude="2.1.300", codex="0.155.1"), INPUTS)
    assert parse_deviations(store.read_text("DEVIATIONS.md")) == [
        DeviationEntry.provenance("CLI claude installed 2.1.300, models.lock.json records 2.1.292")
    ]


def test_a_cli_update_is_provenance_and_a_harness_change_is_a_protocol_deviation_for_all(tmp_path):
    store = RunDirectory(tmp_path)
    ensure_manifest(store, FixedProvenance(), VersionBook(claude="2.1.292", codex="0.155.1"), INPUTS)
    manifest = ensure_manifest(store, FixedProvenance(harness="h2"), VersionBook(claude="2.1.293", codex="0.155.1"), INPUTS)
    log = store.read_text("DEVIATIONS.md")
    assert parse_deviations(log) == [
        DeviationEntry.protocol_for_all("harness_digest changed from h1 to h2"),
        DeviationEntry.provenance("CLI claude changed from 2.1.292 to 2.1.293"),
    ]
    assert manifest["cli_versions"]["claude"] == "2.1.293"
    ensure_manifest(store, FixedProvenance(harness="h2"), VersionBook(claude="2.1.293", codex="0.155.1"), INPUTS)
    assert store.read_text("DEVIATIONS.md") == log


def test_deviations_are_appended_one_rendered_entry_per_line(tmp_path):
    store = RunDirectory(tmp_path)
    log_deviation(store, DeviationEntry.quota_pause("first"))
    log_deviation(store, DeviationEntry.provenance("second"))
    assert store.read_text("DEVIATIONS.md").splitlines()[-2:] == [
        "- quota-pause | endpoints: none | first",
        "- provenance | endpoints: none | second",
    ]


def test_stage_timer_keeps_the_first_start_and_accumulates_seconds(tmp_path):
    store = RunDirectory(tmp_path)
    moments = iter([datetime(2026, 1, 1, tzinfo=UTC), datetime(2026, 1, 1, 0, 0, 1, tzinfo=UTC),
                    datetime(2026, 1, 2, tzinfo=UTC), datetime(2026, 1, 2, 0, 0, 1, tzinfo=UTC)])
    timer = StageTimer(store, lambda: next(moments))
    with timer.stage("s"):
        pass
    with timer.stage("s"):
        pass
    entry = store.read_json("timing.json")["s"]
    assert entry["started"].startswith("2026-01-01") and entry["finished"].startswith("2026-01-02")
    assert entry["seconds"] >= 0


def test_a_failing_stage_still_records_its_time(tmp_path):
    store = RunDirectory(tmp_path)
    with pytest.raises(RuntimeError), StageTimer(store).stage("boom"):
        raise RuntimeError
    assert "boom" in store.read_json("timing.json")


def test_arm_p_takes_the_a_and_b_violations_and_every_n1_item_under_hash_ids():
    violations = {f"PARK-1_{s}": edit_set(n) for n, s in enumerate("abc")}
    planted = {"PARK-1_n1": edit_set(9)}
    items = assemble_items(violations, planted)
    assert sorted(i.source for i in items) == ["PARK-1_a", "PARK-1_b", "PARK-1_n1"]
    by_source = {i.source: i for i in items}
    assert by_source["PARK-1_a"].kind == "r" and by_source["PARK-1_n1"].kind == "n1"
    assert by_source["PARK-1_b"].declared_target == "PARK-1~1"
    assert all(i.mutant_id == mutant_id(i.edit_set) for i in items)
    assert [i.mutant_id for i in items] == sorted(i.mutant_id for i in items)


def test_arm_p_rejects_two_items_with_one_edit_set():
    with pytest.raises(DuplicateArmPItem):
        assemble_items({"PARK-1_a": edit_set(1), "PARK-1_b": edit_set(1)}, {})


class Items:
    def __init__(self, items):
        self._items = items

    def items(self):
        return self._items


def test_planted_mutants_must_hash_to_their_file_names():
    good = edit_set(1)
    assert [m.arm for m in planted_mutants(Items({mutant_id(good): good}))] == ["P"]
    with pytest.raises(ArmPIntegrityError):
        planted_mutants(Items({"0" * 16: good}))
