from __future__ import annotations

import json

import pytest

from calibration.domain.canonical import canonical_json, sha256_hex
from calibration.domain.constants import KEV_COMMIT, OLLAMA_VERSION
from calibration.domain.jsonread import RecordError
from calibration.domain.lock import (
    MEMORY_FIT_REASON,
    MEMORY_PRESSURE_REASON,
    LockedModel,
    ModelsLock,
    PinStatus,
)
from calibration.domain.models import model_by_id
from calibration.domain.provenance_rules import (
    DIGEST_PREFIX,
    bare_digest,
    builder_rollup,
    harness_changes,
    lock_changes,
    provenance_changes,
)

TOTAL_MEMORY = 25769803776
OLLAMA_DIGEST = "ab" * 32
KEV_REVISION = "0123456789abcdef0123456789abcdef01234567"
RUN_REFERENCE = f"jaredpalmer/kev-4b@{KEV_REVISION}"


def _lock_record() -> dict[str, object]:
    return {
        "total_memory_bytes": TOTAL_MEMORY,
        "ollama": {
            "version": OLLAMA_VERSION,
            "models": {
                "M3": {
                    "status": "pinned",
                    "block_reason": None,
                    "footprint_bytes": 2_000_000_000,
                    "model_digest": DIGEST_PREFIX + OLLAMA_DIGEST,
                    "run_reference": None,
                    "manifest_bytes": 1_000_000_000,
                }
            },
        },
        "kev": {
            "commit": KEV_COMMIT,
            "models": {
                "M5": {
                    "status": "pinned",
                    "block_reason": None,
                    "footprint_bytes": 9_000_000_000,
                    "model_digest": KEV_REVISION,
                    "run_reference": RUN_REFERENCE,
                },
                "M6": {
                    "status": "blocked",
                    "block_reason": MEMORY_FIT_REASON,
                    "footprint_bytes": 14_000_000_000,
                    "model_digest": KEV_REVISION,
                    "run_reference": None,
                },
            },
        },
    }


def _lock() -> ModelsLock:
    return ModelsLock.from_record(_lock_record())


class TestModelsLock:
    def test_the_typed_view_reads_both_servers(self) -> None:
        lock = _lock()
        assert (lock.total_memory_bytes, lock.ollama_version, lock.kev_commit) == (
            TOTAL_MEMORY,
            OLLAMA_VERSION,
            KEV_COMMIT,
        )
        assert set(lock.models) == {"M3", "M5", "M6"}

    def test_the_full_record_is_kept_whole(self) -> None:
        assert json.loads(canonical_json(_lock().record)) == _lock_record()

    def test_a_blocked_model_carries_its_reason(self) -> None:
        entry = _lock().entry("M6")
        assert (entry.status, entry.block_reason) == (PinStatus.BLOCKED, MEMORY_FIT_REASON)

    def test_the_block_reasons_are_the_documented_ones(self) -> None:
        assert (MEMORY_FIT_REASON, MEMORY_PRESSURE_REASON) == ("memory-fit", "memory-pressure")

    def test_a_model_without_an_entry_is_refused(self) -> None:
        with pytest.raises(RecordError):
            _lock().entry("M1")

    @pytest.mark.parametrize("section", ["ollama", "kev"])
    def test_a_lock_without_a_server_section_is_refused(self, section: str) -> None:
        record = _lock_record()
        del record[section]
        with pytest.raises(RecordError):
            ModelsLock.from_record(record)

    def test_an_unknown_status_is_refused(self) -> None:
        with pytest.raises(RecordError):
            LockedModel.from_record("M3", {**_model_body(), "status": "maybe"})

    def test_a_negative_footprint_is_refused(self) -> None:
        with pytest.raises(RecordError):
            LockedModel.from_record("M3", {**_model_body(), "footprint_bytes": -1})


def _model_body() -> dict[str, object]:
    return {
        "status": "pinned",
        "block_reason": None,
        "footprint_bytes": 1,
        "model_digest": "d",
        "run_reference": None,
    }


class TestLockedModelRecord:
    def test_a_pinned_model_round_trips(self) -> None:
        entry = _lock().entry("M5")
        assert LockedModel.from_record("M5", entry.to_record()) == entry

    def test_a_blocked_model_round_trips(self) -> None:
        entry = _lock().entry("M6")
        assert LockedModel.from_record("M6", entry.to_record()) == entry

    def test_the_record_names_the_five_fields(self) -> None:
        assert set(_lock().entry("M5").to_record()) == {
            "status",
            "block_reason",
            "footprint_bytes",
            "model_digest",
            "run_reference",
        }


class TestLockChanges:
    def test_a_server_that_matches_the_lock_changes_nothing(self) -> None:
        lock = _lock()
        assert lock_changes(model_by_id("M3"), lock, lock.entry("M3"), OLLAMA_VERSION, OLLAMA_DIGEST) == []

    def test_the_digest_prefix_and_case_do_not_matter(self) -> None:
        lock = _lock()
        digest = DIGEST_PREFIX + OLLAMA_DIGEST.upper()
        assert lock_changes(model_by_id("M3"), lock, lock.entry("M3"), OLLAMA_VERSION, digest) == []

    def test_an_ollama_model_is_checked_against_the_ollama_version(self) -> None:
        lock = _lock()
        (change,) = lock_changes(model_by_id("M3"), lock, lock.entry("M3"), "0.36.0", OLLAMA_DIGEST)
        assert "0.36.0" in change and OLLAMA_VERSION in change and change.startswith("M3")

    def test_a_kev_model_is_checked_against_the_kev_commit(self) -> None:
        lock = _lock()
        assert lock_changes(model_by_id("M5"), lock, lock.entry("M5"), KEV_COMMIT, KEV_REVISION) == []
        (change,) = lock_changes(model_by_id("M5"), lock, lock.entry("M5"), OLLAMA_VERSION, KEV_REVISION)
        assert KEV_COMMIT in change

    def test_a_changed_digest_is_reported(self) -> None:
        lock = _lock()
        (change,) = lock_changes(model_by_id("M3"), lock, lock.entry("M3"), OLLAMA_VERSION, "cd" * 32)
        assert "cd" * 32 in change

    def test_both_departures_are_reported(self) -> None:
        lock = _lock()
        assert len(lock_changes(model_by_id("M3"), lock, lock.entry("M3"), "0.36.0", "cd" * 32)) == 2


class TestBareDigest:
    def test_the_prefix_is_removed_and_the_case_folded(self) -> None:
        assert bare_digest(f"{DIGEST_PREFIX}ABC") == "abc"

    def test_a_bare_digest_is_unchanged(self) -> None:
        assert bare_digest("abc") == "abc"


class TestProvenanceChanges:
    def test_equal_values_change_nothing(self) -> None:
        assert provenance_changes({"os": "26.0"}, {"os": "26.0"}) == []

    def test_a_changed_value_is_reported_with_both_sides(self) -> None:
        assert provenance_changes({"os": "26.0"}, {"os": "26.1"}) == ["os changed from 26.0 to 26.1"]

    def test_a_value_present_on_one_side_only_is_not_a_change(self) -> None:
        assert provenance_changes({"os": "26.0"}, {"uv": "0.9"}) == []

    def test_changes_are_reported_in_key_order(self) -> None:
        changes = provenance_changes({"b": "1", "a": "1"}, {"b": "2", "a": "2"})
        assert [change.split()[0] for change in changes] == ["a", "b"]


class TestHarnessChanges:
    def test_equal_values_change_nothing(self) -> None:
        assert harness_changes({"commit": "abc"}, {"commit": "abc"}) == []

    def test_a_differing_value_is_reported(self) -> None:
        assert harness_changes({"commit": "abc"}, {"commit": "def"}) == ["commit is def, the manifest records abc"]

    def test_a_recorded_value_that_is_now_missing_is_a_departure(self) -> None:
        assert harness_changes({"commit": "abc"}, {}) == ["commit is None, the manifest records abc"]

    def test_a_value_the_manifest_does_not_record_is_ignored(self) -> None:
        assert harness_changes({}, {"commit": "abc"}) == []


class TestBuilderRollup:
    def test_it_is_the_digest_of_the_canonical_json(self) -> None:
        digests = {"alignment": "a" * 64, "format": "f" * 64}
        assert builder_rollup(digests) == sha256_hex(canonical_json(digests))

    def test_it_does_not_depend_on_the_order_of_the_binaries(self) -> None:
        assert builder_rollup({"a": "1", "b": "2"}) == builder_rollup({"b": "2", "a": "1"})

    def test_it_changes_when_a_binary_changes(self) -> None:
        assert builder_rollup({"a": "1"}) != builder_rollup({"a": "2"})
