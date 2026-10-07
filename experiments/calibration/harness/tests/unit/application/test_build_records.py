from __future__ import annotations

import hashlib
import json

from calibration.application.alignment_items import AlignmentAssembly, MaskerRejection
from calibration.application.build_ports import BuilderBuild
from calibration.application.build_records import (
    CHECK_FILE,
    PROMPT_FILES,
    SNAPSHOT_PREREG_PREFIX,
    BuildFacts,
    BuildRecordsWriter,
    prompt_digests,
    source2_template_digest,
)
from calibration.application.builder_check import BuilderCheck
from calibration.application.deviation_log import DeviationLog
from calibration.application.layout import item_path
from calibration.application.manifest import RunManifest
from calibration.application.screen_items import ScreenAssembly, ScreenCandidate
from calibration.domain.canonical import document_json, sha256_hex
from calibration.domain.constants import (
    DEVIATIONS_FILE,
    EXCLUDED_FILE,
    PILOT_FREEZE_TAG,
    PILOT_INPUTS_FILE,
    PREREG_TAG,
    SCHEDULE_FILE,
)
from calibration.domain.deviations import DeviationKind
from calibration.domain.items import Item, KindFamily
from calibration.domain.jsonread import mapping
from calibration.domain.provenance_rules import builder_rollup
from calibration.domain.schedule import Schedule, alignment_request_id, mutant_request_id
from support.build_fakes import FakePilotRunReader, InMemoryRunStore
from support.build_scenario import BINARY_DIGESTS, SNAPSHOT_FILE_DIGESTS

NODEID = "tests/test_fee.py::test_charges"
PASSED = BuilderCheck(3, 3, 3, (), ())
PILOT_FILE = "bindings.json"


def _alignment_item() -> Item:
    return Item(
        request_id=alignment_request_id(NODEID),
        family=KindFamily.ALIGNMENT,
        unit=NODEID,
        closed={"questions": {}},
        closed_digest="c1",
        template_digest="t1",
        bodies={"nimble:9b": "body"},
        nodeid=NODEID,
        label="label",
        declared="PARK-1~1",
        token_to_ref={"T1": "PARK-1~1"},
    )


def _screen_item(mutant_id: str) -> Item:
    return Item(
        request_id=mutant_request_id(mutant_id),
        family=KindFamily.SCREEN,
        unit=mutant_id,
        closed={"questions": {}},
        closed_digest="c2",
        template_digest="t2",
        bodies={"nimble:9b": "body"},
        mutant_id=mutant_id,
        question_to_ref={"violates": "PARK-1~1"},
        witness_status="none",
    )


def _unmapped(mutant_id: str, reason: str) -> ScreenCandidate:
    return ScreenCandidate(mutant_id, "P", (), None, "unmapped", reason)


def _facts(*, clean: bool = True) -> BuildFacts:
    return BuildFacts(
        run_id="run-test",
        prereg_commit="a" * 40,
        harness_commit="d" * 40,
        harness_clean=clean,
        pilot_freeze_commit="b" * 40,
        pilot_analysis_commit="c" * 40,
        snapshot_files=SNAPSHOT_FILE_DIGESTS,
        export_digests={"go.work": "e1"},
        build=BuilderBuild({"alignment": "s1"}, BINARY_DIGESTS),
        go_procs=2,
        alignment_template_digest="alignment-digest",
        screen_template_digest="screen-digest",
    )


def _alignment() -> AlignmentAssembly:
    rejection = MaskerRejection(
        alignment_request_id("tests/x.py::masked"), "tests/x.py::masked", "l", "unmaskable", "d"
    )
    return AlignmentAssembly(
        (_alignment_item(),), (rejection,), {NODEID: ("PARK-1~1",)}, ("PARK-1~1",), "alignment-digest"
    )


def _screen() -> ScreenAssembly:
    unmapped = (_unmapped("m-b", "reason b"), _unmapped("m-a", "reason a"))
    return ScreenAssembly((_screen_item("m-a"), _screen_item("m-b")), unmapped, unmapped, False)


class _Written:
    def __init__(self, *, clean: bool = True, seal_mismatches: list[str] | None = None) -> None:
        self.store = InMemoryRunStore()
        self.reader = FakePilotRunReader({PILOT_FILE: "{}"}, seal_mismatches)
        self.reader.read_text(PILOT_FILE)
        self.alignment = _alignment()
        self.screen = _screen()
        BuildRecordsWriter(self.store, self.reader).write(self.alignment, self.screen, PASSED, _facts(clean=clean))

    def json(self, path: str) -> dict[str, object]:
        return dict(mapping(json.loads(self.store.read_text(path)), path))


class TestItemsAndSchedule:
    def test_every_item_is_written_under_its_slug(self) -> None:
        written = _Written()

        for item in (*written.alignment.items, *written.screen.items):
            assert json.loads(written.store.read_text(item_path(item.request_id))) == item.to_record()

    def test_the_schedule_is_the_one_of_the_request_ids(self) -> None:
        written = _Written()
        ids = [item.request_id for item in (*written.alignment.items, *written.screen.items)]

        schedule = Schedule.of(ids)

        assert written.json(SCHEDULE_FILE) == {"pass1": list(schedule.pass1), "pass2": list(schedule.pass2)}

    def test_every_alignment_request_is_in_pass_two(self) -> None:
        written = _Written()

        pass_two = written.json(SCHEDULE_FILE)["pass2"]
        assert isinstance(pass_two, list)
        assert alignment_request_id(NODEID) in pass_two


class TestExcluded:
    def test_masker_rejections_are_listed_with_their_record(self) -> None:
        written = _Written()

        assert written.json(EXCLUDED_FILE)["masker_rejections"] == [r.to_record() for r in written.alignment.rejections]

    def test_unmapped_witnesses_are_listed_in_mutant_id_order_with_their_reason(self) -> None:
        written = _Written()

        assert written.json(EXCLUDED_FILE)["witness_unmapped"] == [
            {"mutant_id": "m-a", "reason": "reason a"},
            {"mutant_id": "m-b", "reason": "reason b"},
        ]


class TestPilotInputsAndCheck:
    def test_the_pilot_inputs_hold_the_digest_of_each_file_read_and_the_seal_mismatches(self) -> None:
        written = _Written(seal_mismatches=[PILOT_FILE])

        assert written.json(PILOT_INPUTS_FILE) == {
            "files": {PILOT_FILE: sha256_hex("{}")},
            "seal_mismatches": [PILOT_FILE],
        }

    def test_the_builder_check_is_archived_with_the_harness_commit(self) -> None:
        written = _Written()

        assert written.json(CHECK_FILE) == PASSED.to_record("d" * 40)


class TestDeviationOnADirtyHarness:
    def test_a_dirty_harness_adds_one_protocol_entry_for_all(self) -> None:
        written = _Written(clean=False)

        entries = DeviationLog(written.store).entries()
        assert [(e.kind, e.scope) for e in entries] == [(DeviationKind.PROTOCOL, ("all",))]

    def test_a_clean_harness_adds_no_entry(self) -> None:
        written = _Written()

        assert not written.store.exists(DEVIATIONS_FILE)


class TestManifest:
    def test_the_manifest_identifies_the_preregistration_the_pilot_and_the_harness(self) -> None:
        manifest = _Written().json("manifest.json")

        assert manifest["run_id"] == "run-test"
        assert manifest["prereg_tag"] == PREREG_TAG
        assert manifest["prereg_commit"] == "a" * 40
        assert manifest["harness_commit"] == "d" * 40
        assert manifest["harness_clean"] is True
        assert manifest["pilot"] == {
            "freeze_tag": PILOT_FREEZE_TAG,
            "freeze_commit": "b" * 40,
            "analysis_commit": "c" * 40,
        }

    def test_the_builder_digest_is_the_rollup_of_the_binary_digests(self) -> None:
        manifest = _Written().json("manifest.json")

        assert manifest["builder_digest"] == builder_rollup(BINARY_DIGESTS)
        assert manifest["builders"] == {
            "source_digests": {"alignment": "s1"},
            "binary_digests": BINARY_DIGESTS,
            "go_procs": 2,
        }

    def test_the_snapshot_and_product_export_digests_are_digests_of_their_file_maps(self) -> None:
        manifest = _Written().json("manifest.json")

        assert manifest["snapshot_digest"] == sha256_hex(document_json(SNAPSHOT_FILE_DIGESTS))
        assert manifest["product_export"] == {
            "commit": "a" * 40,
            "digest": sha256_hex(document_json({"go.work": "e1"})),
            "files": {"go.work": "e1"},
        }

    def test_the_template_digests_are_recorded_per_module(self) -> None:
        manifest = _Written().json("manifest.json")

        assert manifest["template_digests"] == {"alignment": "alignment-digest", "discpower": "screen-digest"}

    def test_the_counts_describe_the_build(self) -> None:
        manifest = _Written().json("manifest.json")

        assert manifest["counts"] == {
            "alignment_items": 1,
            "masker_rejections": 1,
            "screen_items": 2,
            "screen_candidates": 2,
            "screen_blocked": False,
        }

    def test_the_sections_of_other_verbs_already_in_the_manifest_survive(self) -> None:
        store = InMemoryRunStore()
        RunManifest(store).set_sections({"servers": {"M1": "0.35.1"}})

        BuildRecordsWriter(store, FakePilotRunReader({})).write(_alignment(), _screen(), PASSED, _facts())

        assert RunManifest(store).read()["servers"] == {"M1": "0.35.1"}


class TestSourceTwoPromptDigests:
    def test_the_digests_of_the_pilot_prompt_and_schema_of_each_module_are_recorded(self) -> None:
        digests = prompt_digests(SNAPSHOT_FILE_DIGESTS)

        for name, (prompt, schema) in PROMPT_FILES.items():
            prompt_digest = SNAPSHOT_FILE_DIGESTS[SNAPSHOT_PREREG_PREFIX + prompt]
            schema_digest = SNAPSHOT_FILE_DIGESTS[SNAPSHOT_PREREG_PREFIX + schema]
            assert digests[name] == {
                "prompt": prompt_digest,
                "schema": schema_digest,
                "template": source2_template_digest(prompt_digest, schema_digest),
            }

    def test_the_template_digest_covers_the_prompt_and_the_schema_in_that_order(self) -> None:
        assert source2_template_digest("p", "s") == hashlib.sha256(b"p\x00s").hexdigest()
        assert source2_template_digest("p", "s") != source2_template_digest("s", "p")

    def test_a_file_absent_from_the_snapshot_is_recorded_as_null(self) -> None:
        digests = prompt_digests({})

        assert digests["alignment"] == {
            "prompt": None,
            "schema": None,
            "template": source2_template_digest(None, None),
        }
