from __future__ import annotations

import json
from dataclasses import replace

import pytest

from calibration.application.build_items import PC1, PC2, PC3
from calibration.application.build_records import CHECK_FILE
from calibration.application.builder_check import RECORDED_REPORT, RECORDED_REPORT_ENV
from calibration.application.deviation_log import DeviationLog
from calibration.application.layout import item_path
from calibration.domain.canonical import sha256_hex
from calibration.domain.constants import (
    DEVIATIONS_FILE,
    EXCLUDED_FILE,
    ITEMS_DIR,
    MANIFEST_FILE,
    PILOT_BINDINGS_FILE,
    PILOT_DIFFERENTIAL_FILE,
    PILOT_FREEZE_TAG,
    PILOT_INPUTS_FILE,
    PILOT_MUTANTS_DIR,
    PILOT_PROCESSING_FILE,
    PREREG_TAG,
    SCHEDULE_FILE,
    SNAPSHOT_FILE,
)
from calibration.domain.deviations import DeviationKind
from calibration.domain.freeze import PreconditionFailed
from calibration.domain.models import identifiers
from calibration.domain.provenance_rules import builder_rollup
from calibration.domain.schedule import alignment_request_id, mutant_request_id
from support.build_scenario import (
    ALIGNMENT_TEMPLATE_DIGEST,
    BINARY_DIGESTS,
    CACHE_DIR,
    HARNESS_COMMIT,
    PREREG_COMMIT,
    RUN_ID,
    BuildWorld,
    PilotMutant,
    differs,
)

BUILD_OUTPUT_FILES = (SCHEDULE_FILE, EXCLUDED_FILE, PILOT_INPUTS_FILE, CHECK_FILE, MANIFEST_FILE)
ALLOWED_PILOT_READS = (PILOT_BINDINGS_FILE, PILOT_PROCESSING_FILE, PILOT_DIFFERENTIAL_FILE)
MASKED_TEST_INDEX = 2


def _assert_no_build_output(world: BuildWorld) -> None:
    for path in BUILD_OUTPUT_FILES:
        assert not world.store.exists(path), path
    assert world.store.list_dir(ITEMS_DIR) == []


def _assert_precondition(world: BuildWorld, code: str) -> PreconditionFailed:
    with pytest.raises(PreconditionFailed) as failure:
        world.use_case().run()
    assert failure.value.code == code
    return failure.value


class TestPC1:
    def test_a_missing_tag_stops_the_build_before_anything_is_written(self) -> None:
        world = BuildWorld()
        del world.vcs.tags[PREREG_TAG]

        _assert_precondition(world, PC1)

        assert world.store.paths() == []

    def test_a_recorded_commit_that_is_not_the_tag_commit_stops_the_build(self) -> None:
        world = BuildWorld()
        world.vcs.blobs = dict.fromkeys(world.vcs.blobs, b"0" * 40 + b"\n")

        failure = _assert_precondition(world, PC1)

        assert "peels to" in failure.message
        assert world.store.paths() == []

    def test_a_missing_recorded_commit_stops_the_build(self) -> None:
        world = BuildWorld()
        world.vcs.blobs = {}

        _assert_precondition(world, PC1)

        assert world.store.paths() == []

    def test_an_unreadable_timestamp_stops_the_build(self) -> None:
        world = BuildWorld()
        world.timestamps.stamp = None

        _assert_precondition(world, PC1)

        assert world.store.paths() == []

    def test_a_timestamp_of_another_text_stops_the_build(self) -> None:
        world = BuildWorld()
        assert world.timestamps.stamp is not None
        world.timestamps.stamp = replace(world.timestamps.stamp, imprint=sha256_hex("another"))

        _assert_precondition(world, PC1)

        assert world.store.paths() == []

    def test_a_dirty_harness_is_refused(self) -> None:
        world = BuildWorld(clean=False)

        failure = _assert_precondition(world, PC1)

        assert "uncommitted" in failure.message
        assert world.store.paths() == []
        assert world.exporter.exports == []

    def test_the_harness_paths_are_the_ones_checked_for_cleanliness(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        assert world.vcs.cleanliness_checked == [world.settings().harness_paths]

    def test_a_dirty_harness_is_accepted_when_allowed_and_a_protocol_entry_names_all(self) -> None:
        world = BuildWorld(clean=False)

        world.use_case(allow_dirty=True).run()

        entries = DeviationLog(world.store).entries()
        assert [(entry.kind, entry.scope) for entry in entries] == [(DeviationKind.PROTOCOL, ("all",))]
        manifest = json.loads(world.store.read_text(MANIFEST_FILE))
        assert manifest["harness_clean"] is False
        assert manifest["harness_commit"] == HARNESS_COMMIT

    def test_a_clean_harness_logs_no_deviation(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        assert not world.store.exists(DEVIATIONS_FILE)


class TestPC2:
    def test_a_missing_pilot_tag_stops_the_build_before_anything_is_written(self) -> None:
        world = BuildWorld()
        del world.pilot_git.tags[PILOT_FREEZE_TAG]

        failure = _assert_precondition(world, PC2)

        assert PILOT_FREEZE_TAG in failure.message
        assert world.store.paths() == []

    def test_a_missing_analysis_commit_stops_the_build_before_anything_is_written(self) -> None:
        world = BuildWorld()
        world.pilot_git.analysis = None

        _assert_precondition(world, PC2)

        assert world.store.paths() == []
        assert world.reader.read_paths == []

    def test_a_pilot_run_without_bindings_stops_the_build_without_build_output(self) -> None:
        world = BuildWorld()
        del world.reader.files[PILOT_BINDINGS_FILE]

        failure = _assert_precondition(world, PC2)

        assert PILOT_BINDINGS_FILE in failure.message
        _assert_no_build_output(world)

    def test_no_pilot_file_is_opened_when_a_precondition_before_the_snapshot_fails(self) -> None:
        world = BuildWorld(clean=False)

        _assert_precondition(world, PC1)

        assert world.reader.read_paths == []


class TestPC3:
    def test_an_archived_run_directory_is_never_built_again(self) -> None:
        world = BuildWorld()
        world.use_case().run()
        written = list(world.store.write_log)

        failure = _assert_precondition(world, PC3)

        assert "archived" in failure.message
        assert world.store.write_log == written

    def test_a_binding_the_module_does_not_make_stops_the_build(self) -> None:
        world = BuildWorld()
        bindings = json.loads(world.reader.files[PILOT_BINDINGS_FILE])
        first = next(iter(bindings))
        bindings[first] = "PARK-9~1"
        world.reader.files[PILOT_BINDINGS_FILE] = json.dumps(bindings)

        failure = _assert_precondition(world, PC3)

        assert first in failure.message
        _assert_no_build_output(world)

    def test_a_pilot_binding_the_module_lacks_stops_the_build(self) -> None:
        world = BuildWorld()
        bindings = json.loads(world.reader.files[PILOT_BINDINGS_FILE])
        bindings["tests/test_fee.py::test_ghost"] = "PARK-4~1"
        world.reader.files[PILOT_BINDINGS_FILE] = json.dumps(bindings)

        failure = _assert_precondition(world, PC3)

        assert "test_ghost" in failure.message
        _assert_no_build_output(world)

    def test_an_item_with_another_template_digest_stops_the_build(self) -> None:
        world = BuildWorld()
        world.builders.alignment_output = {**world.builders.alignment_output, "template_digest": "changed"}

        failure = _assert_precondition(world, PC3)

        assert "template digest" in failure.message
        _assert_no_build_output(world)

    def test_a_mutant_id_the_pilot_does_not_reproduce_stops_the_build(self) -> None:
        mutants = (PilotMutant("forged", "T", differential=differs(mapped=True), recorded_id="f" * 16),)
        world = BuildWorld(mutants=mutants)

        failure = _assert_precondition(world, PC3)

        assert "is not reproduced" in failure.message
        _assert_no_build_output(world)

    def test_a_failed_builder_check_stops_the_build(self) -> None:
        world = BuildWorld()
        world.builders.compare_report = {**world.builders.compare_report, "matched": 2}

        failure = _assert_precondition(world, PC3)

        assert "builder check" in failure.message
        _assert_no_build_output(world)

    def test_a_mutant_the_screen_builder_leaves_without_request_stops_the_build(self) -> None:
        world = BuildWorld()
        world.builders.omitted_mutants = {world.mutant("mapped_target").mutant_id}

        failure = _assert_precondition(world, PC3)

        assert "has no request" in failure.message
        _assert_no_build_output(world)

    def test_a_request_for_an_unknown_mutant_stops_the_build(self) -> None:
        world = BuildWorld()
        world.builders.extra_mutants = ["0" * 16]

        failure = _assert_precondition(world, PC3)

        assert "unknown mutant" in failure.message
        _assert_no_build_output(world)

    def test_every_problem_is_reported_in_one_failure(self) -> None:
        world = BuildWorld()
        world.builders.compare_report = {**world.builders.compare_report, "matched": 0}
        world.builders.omitted_mutants = {world.mutant("mapped_target").mutant_id}

        failure = _assert_precondition(world, PC3)

        assert "builder check" in failure.message
        assert "has no request" in failure.message

    def test_the_work_directory_is_removed_even_when_the_build_fails(self) -> None:
        world = BuildWorld()
        world.builders.compare_report = {**world.builders.compare_report, "matched": 0}

        _assert_precondition(world, PC3)

        assert CACHE_DIR / f"work-{RUN_ID}" in world.digester.removed


class TestSuccessfulBuild:
    def test_every_record_of_the_build_is_written(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        for path in (*BUILD_OUTPUT_FILES, SNAPSHOT_FILE):
            assert world.store.exists(path), path

    def test_every_item_is_archived_under_its_slug_and_scheduled(self) -> None:
        world = BuildWorld()
        report = world.use_case().run()

        schedule = json.loads(world.store.read_text(SCHEDULE_FILE))
        archived = world.store.list_dir(ITEMS_DIR)
        assert len(archived) == report.alignment_items + report.screen_items
        assert sorted(schedule["pass1"]) == sorted(
            [alignment_request_id(world.tests[index].nodeid) for index in (0, 1)]
            + [
                mutant_request_id(world.mutant(name).mutant_id)
                for name in ("mapped_target", "unmapped_pocket", "no_differential")
            ]
        )
        for request_id in schedule["pass1"]:
            assert item_path(request_id).removeprefix(f"{ITEMS_DIR}/") in archived

    def test_the_report_counts_what_was_built(self) -> None:
        world = BuildWorld()

        report = world.use_case().run()

        assert (report.alignment_items, report.masker_rejections) == (2, 1)
        assert (report.screen_items, report.witness_unmapped, report.screen_blocked) == (3, 1, False)

    def test_a_masker_rejection_is_listed_in_excluded_and_is_not_an_item(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        masked = world.tests[MASKED_TEST_INDEX]
        excluded = json.loads(world.store.read_text(EXCLUDED_FILE))
        assert [entry["nodeid"] for entry in excluded["masker_rejections"]] == [masked.nodeid]
        assert excluded["masker_rejections"][0]["reason"] == "unmaskable"
        assert item_path(alignment_request_id(masked.nodeid)).removeprefix(f"{ITEMS_DIR}/") not in world.store.list_dir(
            ITEMS_DIR
        )

    def test_an_unmapped_witness_is_listed_in_excluded_with_its_reason(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        excluded = json.loads(world.store.read_text(EXCLUDED_FILE))
        assert [entry["mutant_id"] for entry in excluded["witness_unmapped"]] == [
            world.mutant("unmapped_pocket").mutant_id
        ]
        assert "literal type" in excluded["witness_unmapped"][0]["reason"]

    def test_the_manifest_records_the_builder_digest_and_the_commits(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        manifest = json.loads(world.store.read_text(MANIFEST_FILE))
        assert manifest["builder_digest"] == builder_rollup(BINARY_DIGESTS)
        assert manifest["prereg_commit"] == PREREG_COMMIT
        assert manifest["harness_commit"] == HARNESS_COMMIT
        assert manifest["template_digests"]["alignment"] == ALIGNMENT_TEMPLATE_DIGEST
        assert manifest["counts"]["screen_items"] == 3

    def test_the_builder_check_record_is_archived_with_the_harness_commit(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        record = json.loads(world.store.read_text(CHECK_FILE))
        assert record["passed"] is True
        assert record["harness_commit"] == HARNESS_COMMIT

    def test_the_pilot_inputs_list_exactly_the_files_read_with_their_digests(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        inputs = json.loads(world.store.read_text(PILOT_INPUTS_FILE))
        assert set(inputs["files"]) == set(world.reader.read_paths)
        assert inputs["files"][PILOT_BINDINGS_FILE] == sha256_hex(world.reader.files[PILOT_BINDINGS_FILE])

    def test_only_the_files_the_build_may_read_are_opened(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        for path in world.reader.read_paths:
            assert path in ALLOWED_PILOT_READS or path.startswith(f"{PILOT_MUTANTS_DIR}/")

    def test_a_mutant_that_is_stillborn_or_not_ok_or_of_another_arm_is_not_an_item(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        scheduled = json.loads(world.store.read_text(SCHEDULE_FILE))["pass1"]
        for name in ("stillborn_one", "duplicate_one", "operator_arm"):
            assert mutant_request_id(world.mutant(name).mutant_id) not in scheduled

    def test_the_product_is_exported_at_the_prereg_commit_into_a_directory_cleared_first(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        export_dir = CACHE_DIR / f"product-{PREREG_COMMIT}"
        assert world.exporter.exports == [(PREREG_COMMIT, export_dir)]
        assert export_dir in world.digester.removed
        assert f"remove:{export_dir}" in world.events

    def test_the_builders_are_compiled_into_a_directory_named_by_the_harness_commit(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        export_dir = CACHE_DIR / f"product-{PREREG_COMMIT}"
        assert world.toolchain.builds == [
            (
                export_dir,
                world.settings().builder_dir,
                CACHE_DIR / f"builders-{HARNESS_COMMIT}",
                world.settings().go_procs,
            )
        ]

    def test_the_builder_check_drives_a_second_tool_set_that_replays_the_recorded_report(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        export_dir = CACHE_DIR / f"product-{PREREG_COMMIT}"
        assert world.tools.opened[0][1] == {}
        assert world.tools.opened[1][1] == {RECORDED_REPORT_ENV: str(export_dir / RECORDED_REPORT)}

    def test_the_alignment_builder_asks_for_a_body_for_every_model_identifier(self) -> None:
        world = BuildWorld()

        world.use_case().run()

        assert world.builders.alignment_calls[0][2] == identifiers()


class TestMissingArmRecords:
    @pytest.mark.parametrize("missing", [PILOT_PROCESSING_FILE, PILOT_DIFFERENTIAL_FILE])
    def test_k4_is_blocked_and_no_screen_item_is_built(self, missing: str) -> None:
        world = BuildWorld()
        del world.reader.files[missing]

        report = world.use_case().run()

        assert report.screen_blocked is True
        assert report.screen_items == 0
        assert world.builders.screen_calls == []
        entries = DeviationLog(world.store).entries()
        assert [(entry.kind, entry.scope) for entry in entries] == [(DeviationKind.BLOCKED, ("K4",))]

    def test_the_alignment_items_are_still_archived_and_the_manifest_records_the_block(self) -> None:
        world = BuildWorld()
        del world.reader.files[PILOT_PROCESSING_FILE]

        world.use_case().run()

        schedule = json.loads(world.store.read_text(SCHEDULE_FILE))
        assert len(schedule["pass1"]) == 2
        manifest = json.loads(world.store.read_text(MANIFEST_FILE))
        assert manifest["counts"]["screen_blocked"] is True
