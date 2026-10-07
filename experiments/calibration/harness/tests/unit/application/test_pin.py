from __future__ import annotations

import json
from dataclasses import replace

import pytest

from calibration.application.deviation_log import DeviationLog
from calibration.application.pin import PinError
from calibration.application.pin_ports import HeadMeta, HubFile, KevEnvironmentInfo
from calibration.domain.canonical import document_json
from calibration.domain.constants import (
    BYTES_PER_GIB,
    DEVIATIONS_FILE,
    FOOTPRINT_OVERHEAD_GIB,
    KEV_COMMIT,
    MODELS_LOCK_FILE,
    OLLAMA_VERSION,
)
from calibration.domain.deviations import DeviationKind
from calibration.domain.lock import MEMORY_FIT_REASON, PinStatus
from calibration.domain.memory_rules import footprint_bytes
from calibration.domain.models import MODEL_BY_ID, MODEL_IDS
from support.build_fakes import FakeMemoryProbe
from support.pin_scenario import (
    ADAPTER_BYTES,
    BASE_REVISION_NAME,
    GIB,
    HEAD_BYTES,
    KEV_BASE_REPOSITORY,
    KEV_BASE_WEIGHT_BYTES,
    NON_WEIGHT_FILE,
    TOTAL_MEMORY,
    PinWorld,
    adapter_revision,
    base_revision_sha,
    kev_repository,
    ollama_digest,
)

OLLAMA_IDS = ("M1", "M2", "M3")
KEV_IDS = ("M4", "M5", "M6")
HALF_OF_MEMORY = TOTAL_MEMORY // 2
OVERHEAD = FOOTPRINT_OVERHEAD_GIB * BYTES_PER_GIB


def _set_ollama_manifest_size(world: PinWorld, model_id: str, size_bytes: int) -> None:
    identifier = MODEL_BY_ID[model_id].identifier
    world.registry.manifests[identifier] = replace(world.registry.manifests[identifier], size_bytes=size_bytes)


class TestStaticMemoryRuleForKev:
    def test_kev_9b_is_blocked_on_24_gib(self) -> None:
        world = PinWorld()

        report = world.use_case().run()

        entry = world.lock().entry("M6")
        assert entry.status is PinStatus.BLOCKED
        assert entry.block_reason == MEMORY_FIT_REASON
        assert entry.model_digest == ""
        assert entry.run_reference is None
        assert report.blocked == ("M6",)

    def test_the_blocked_footprint_is_the_adapter_head_and_base_weights_plus_one_gib(self) -> None:
        world = PinWorld()

        world.use_case().run()

        weights = ADAPTER_BYTES + HEAD_BYTES + KEV_BASE_WEIGHT_BYTES["M6"]
        assert world.lock().entry("M6").footprint_bytes == footprint_bytes(weights)

    def test_kev_9b_is_never_downloaded_neither_adapter_nor_base(self) -> None:
        world = PinWorld()

        world.use_case().run()

        downloaded = world.kev.downloaded_repositories()
        assert kev_repository("M6") not in downloaded
        assert KEV_BASE_REPOSITORY["M6"] not in downloaded

    def test_the_head_of_a_blocked_model_is_still_read_to_learn_its_base(self) -> None:
        world = PinWorld()

        world.use_case().run()

        assert f"head:{kev_repository('M6')}@{adapter_revision('M6')}" in world.events

    def test_the_admitted_kev_models_download_the_adapter_and_the_base_revision_the_head_names(self) -> None:
        world = PinWorld()

        world.use_case().run()

        for model_id in ("M4", "M5"):
            assert (kev_repository(model_id), adapter_revision(model_id)) in world.kev.downloads
            assert (KEV_BASE_REPOSITORY[model_id], BASE_REVISION_NAME) in world.kev.downloads

    def test_a_head_that_names_no_base_revision_downloads_the_default_branch_and_records_the_resolved_sha(self) -> None:
        world = PinWorld()
        base = KEV_BASE_REPOSITORY["M4"]
        world.kev.heads[kev_repository("M4")] = HeadMeta(base, None)
        world.hub.repositories[(base, None)] = replace(world.hub.repositories[(base, BASE_REVISION_NAME)])

        world.use_case().run()

        assert (base, None) in world.kev.downloads
        record = json.loads(world.lock_text())["kev"]["models"]["M4"]
        assert record["base_revision"] == base_revision_sha("M4")
        assert record["base_revision_named"] is None

    def test_the_admitted_kev_entries_record_both_revisions_and_the_run_reference(self) -> None:
        world = PinWorld()

        world.use_case().run()

        entry = world.lock().entry("M5")
        assert entry.status is PinStatus.PINNED
        assert entry.model_digest == adapter_revision("M5")
        assert entry.run_reference == f"{kev_repository('M5')}@{adapter_revision('M5')}"
        record = json.loads(world.lock_text())["kev"]["models"]["M5"]
        assert record["base"] == KEV_BASE_REPOSITORY["M5"]
        assert record["base_revision"] == base_revision_sha("M5")
        assert record["base_revision_named"] == "v1.0"

    def test_every_weight_file_is_recorded_with_its_size(self) -> None:
        world = PinWorld()

        world.use_case().run()

        files = json.loads(world.lock_text())["kev"]["models"]["M4"]["files"]
        sizes = {(file["repository"], file["path"]): file["size_bytes"] for file in files}
        assert sizes[(kev_repository("M4"), "adapter_model.safetensors")] == ADAPTER_BYTES
        assert sizes[(kev_repository("M4"), "head.pt")] == HEAD_BYTES
        assert (
            sum(size for (repository, _), size in sizes.items() if repository == KEV_BASE_REPOSITORY["M4"])
            == (KEV_BASE_WEIGHT_BYTES["M4"])
        )

    def test_a_file_that_is_not_a_weight_does_not_count_toward_the_footprint(self) -> None:
        world = PinWorld()
        adapter = kev_repository("M5")
        original = world.hub.repositories[(adapter, None)]
        world.hub.repositories[(adapter, None)] = replace(
            original, files=(*original.files, HubFile(NON_WEIGHT_FILE, TOTAL_MEMORY * 4))
        )

        world.use_case().run()

        assert world.lock().entry("M5").status is PinStatus.PINNED

    def test_the_blocked_model_is_logged_as_a_blocked_deviation_scoped_to_it(self) -> None:
        world = PinWorld()

        world.use_case().run()

        entries = DeviationLog(world.store).entries()
        assert [(entry.kind, entry.scope) for entry in entries] == [(DeviationKind.BLOCKED, ("M6",))]
        assert MEMORY_FIT_REASON in entries[0].text

    def test_the_lock_is_written_per_family_so_the_ollama_family_is_on_disk_before_kev_starts(self) -> None:
        world = PinWorld()

        world.use_case().run()

        assert world.store.count_writes_to(MODELS_LOCK_FILE) == 2


class TestStaticMemoryRuleForOllama:
    def test_every_ollama_model_is_admitted_and_pulled_on_24_gib(self) -> None:
        world = PinWorld()

        report = world.use_case().run()

        assert world.provisioner.provisioned == [[MODEL_BY_ID[model_id].identifier for model_id in OLLAMA_IDS]]
        assert set(OLLAMA_IDS) <= set(report.pinned)

    def test_a_blocked_ollama_model_is_absent_from_the_pull_list(self) -> None:
        world = PinWorld()
        _set_ollama_manifest_size(world, "M1", HALF_OF_MEMORY)

        report = world.use_case().run()

        assert MODEL_BY_ID["M1"].identifier not in world.provisioner.provisioned[0]
        assert world.lock().entry("M1").status is PinStatus.BLOCKED
        assert "M1" in report.blocked

    def test_a_footprint_of_exactly_half_the_memory_is_admitted(self) -> None:
        world = PinWorld()
        _set_ollama_manifest_size(world, "M1", HALF_OF_MEMORY - OVERHEAD)

        world.use_case().run()

        assert world.lock().entry("M1").status is PinStatus.PINNED

    def test_a_footprint_one_byte_over_half_the_memory_is_blocked(self) -> None:
        world = PinWorld()
        _set_ollama_manifest_size(world, "M1", HALF_OF_MEMORY - OVERHEAD + 1)

        world.use_case().run()

        entry = world.lock().entry("M1")
        assert entry.status is PinStatus.BLOCKED
        assert entry.footprint_bytes == HALF_OF_MEMORY + 1

    def test_the_rule_follows_the_total_memory_the_probe_reports(self) -> None:
        world = PinWorld(total_memory=2 * GIB)

        report = world.use_case().run()

        assert set(report.blocked) == set(MODEL_IDS)

    def test_the_pinned_entry_records_digest_footprint_and_server_description(self) -> None:
        world = PinWorld()

        world.use_case().run()

        entry = world.lock().entry("M2")
        assert entry.model_digest == ollama_digest("M2")
        assert entry.footprint_bytes == footprint_bytes(3 * GIB)
        record = json.loads(world.lock_text())["ollama"]["models"]["M2"]
        assert record["identifier"] == MODEL_BY_ID["M2"].identifier
        assert record["details"]["quantization_level"] == "Q4_K_M"
        assert record["parameters"]["num_ctx"] == 4096

    def test_the_lock_records_the_total_memory_the_versions_and_the_kev_environment(self) -> None:
        world = PinWorld()

        world.use_case().run()

        lock = world.lock()
        assert lock.total_memory_bytes == TOTAL_MEMORY
        assert lock.ollama_version == OLLAMA_VERSION
        assert lock.kev_commit == KEV_COMMIT
        kev = json.loads(world.lock_text())["kev"]
        assert kev["versions"] == {"mlx": "0.29.1", "mlx-lm": "0.28.0", "torch": "2.8.0"}
        assert kev["uv_lock_sha256"] == world.kev.info.uv_lock_sha256


class TestSizesComeFromMetadataBeforeAnyDownload:
    def test_every_manifest_is_read_before_the_server_pulls_anything(self) -> None:
        world = PinWorld()

        world.use_case().run()

        pull = world.events.index("provision")
        manifests = [index for index, event in enumerate(world.events) if event.startswith("manifest:")]
        assert len(manifests) == len(OLLAMA_IDS)
        assert max(manifests) < pull

    def test_the_sizes_of_a_kev_model_are_known_before_any_of_its_downloads(self) -> None:
        world = PinWorld()

        world.use_case().run()

        for model_id in ("M4", "M5"):
            first_download = next(
                index
                for index, event in enumerate(world.events)
                if event.startswith(f"download:{kev_repository(model_id)}")
            )
            adapter_metadata = world.events.index(f"hub:{kev_repository(model_id)}@None")
            base_metadata = world.events.index(f"hub:{KEV_BASE_REPOSITORY[model_id]}@v1.0")
            assert max(adapter_metadata, base_metadata) < first_download


class TestAnExistingEntryIsNeverRewritten:
    def test_a_complete_lock_is_left_byte_for_byte_and_nothing_is_contacted(self) -> None:
        world = PinWorld()
        world.use_case().run()
        before = world.lock_text()
        writes = world.store.count_writes_to(MODELS_LOCK_FILE)
        world.events.clear()
        world.registry.manifests.clear()

        report = world.use_case().run()

        assert world.lock_text() == before
        assert world.store.count_writes_to(MODELS_LOCK_FILE) == writes
        assert world.events == []
        assert report.pinned == ()
        assert report.already_pinned == tuple(sorted(MODEL_IDS))

    def test_a_blocked_entry_counts_as_pinned_and_is_not_decided_again(self) -> None:
        world = PinWorld()
        world.use_case().run()
        world.memory = FakeMemoryProbe(64 * GIB)

        report = world.use_case().run()

        assert world.lock().entry("M6").status is PinStatus.BLOCKED
        assert "M6" in report.already_pinned

    def test_a_pinned_family_survives_verbatim_while_the_missing_family_is_pinned(self) -> None:
        world = PinWorld()
        kept = {
            "version": OLLAMA_VERSION,
            "models": {
                model_id: {
                    "status": "pinned",
                    "block_reason": None,
                    "footprint_bytes": 1,
                    "model_digest": f"sha256:kept-{model_id}",
                    "run_reference": None,
                    "identifier": MODEL_BY_ID[model_id].identifier,
                }
                for model_id in OLLAMA_IDS
            },
        }
        world.store.write_text(MODELS_LOCK_FILE, document_json({"total_memory_bytes": TOTAL_MEMORY, "ollama": kept}))

        report = world.use_case().run()

        assert json.loads(world.lock_text())["ollama"] == kept
        assert world.provisioner.provisioned == []
        assert report.already_pinned == OLLAMA_IDS
        assert set(report.pinned) == {"M4", "M5"}

    def test_a_kev_entry_already_pinned_is_not_downloaded_again(self) -> None:
        world = PinWorld()
        world.use_case().run()
        lock = json.loads(world.lock_text())
        del lock["kev"]["models"]["M5"]
        world.store.write_text(MODELS_LOCK_FILE, document_json(lock))
        world.kev.downloads.clear()
        world.kev.heads.pop(kev_repository("M4"))

        world.use_case().run()

        assert world.kev.downloaded_repositories() == [kev_repository("M5"), KEV_BASE_REPOSITORY["M5"]]
        assert world.lock().entry("M4").status is PinStatus.PINNED


class TestVersionsAndDigestsMustAgree:
    def test_an_ollama_version_other_than_the_pin_is_refused(self) -> None:
        world = PinWorld()
        world.provisioner.version = "0.36.0"

        with pytest.raises(PinError, match=r"0\.36\.0"):
            world.use_case().run()

        assert not world.store.exists(MODELS_LOCK_FILE)

    def test_a_digest_the_server_reports_that_differs_from_the_registry_is_refused(self) -> None:
        world = PinWorld()
        identifier = MODEL_BY_ID["M3"].identifier
        world.provisioner.records[identifier] = replace(world.provisioner.records[identifier], digest="sha256:other")

        with pytest.raises(PinError, match=identifier):
            world.use_case().run()

        assert not world.store.exists(MODELS_LOCK_FILE)

    def test_a_digest_is_compared_without_its_prefix_and_without_case(self) -> None:
        world = PinWorld()
        identifier = MODEL_BY_ID["M3"].identifier
        bare_upper = ollama_digest("M3").removeprefix("sha256:").upper()
        world.provisioner.records[identifier] = replace(world.provisioner.records[identifier], digest=bare_upper)

        world.use_case().run()

        assert world.lock().entry("M3").model_digest == ollama_digest("M3")

    def test_a_kev_checkout_at_another_commit_is_refused_before_any_download(self) -> None:
        world = PinWorld()
        world.kev.info = KevEnvironmentInfo("0" * len(KEV_COMMIT), "digest", {})

        with pytest.raises(PinError, match="Kev checkout"):
            world.use_case().run()

        assert world.kev.downloads == []

    def test_a_refused_kev_pin_leaves_the_ollama_family_in_the_lock_and_no_kev_section(self) -> None:
        world = PinWorld()
        world.kev.info = KevEnvironmentInfo("0" * len(KEV_COMMIT), "digest", {})

        with pytest.raises(PinError):
            world.use_case().run()

        record = json.loads(world.lock_text())
        assert "kev" not in record
        assert set(record["ollama"]["models"]) == set(OLLAMA_IDS)


def test_the_deviation_log_of_a_pin_without_blocked_models_stays_absent() -> None:
    world = PinWorld()
    world.memory = FakeMemoryProbe(64 * GIB)

    world.use_case().run()

    assert not world.store.exists(DEVIATIONS_FILE)
