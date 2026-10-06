from __future__ import annotations

import hashlib

import pytest

from pilot.domain.import_manifest import (
    EXPECTED_CALL_ROLES,
    EXPECTED_CANARY_FILES,
    IMPORTED_CANARY_DIRECTORY,
    OPUS_GENERATION_IDS,
    RELABEL_IDS,
    RETIRED_CALL_IDS,
    TERRA_GENERATION_IDS,
    ImportRefused,
    ImportRole,
    parse_sums,
    plan_import,
    render_sums,
)


def digest(text: str) -> str:
    return hashlib.sha256(text.encode()).hexdigest()


def sealed_listing() -> dict[str, str]:
    listing = {}
    for call_id in EXPECTED_CALL_ROLES:
        listing[f"raw/{call_id}/result.json"] = digest(call_id)
        listing[f"raw/{call_id}/attempt-1/request.txt"] = digest(call_id + "request")
    for call_id in RETIRED_CALL_IDS:
        listing[f"raw/{call_id}/attempt-1/request.txt"] = digest(call_id)
    for name in EXPECTED_CANARY_FILES:
        listing[f"canary/{name}"] = digest(name)
        listing[f"raw/canary/{name}/result.json"] = digest(name + "raw")
    listing["plan.json"] = digest("plan")
    listing["DEVIATIONS.md"] = digest("deviations")
    return listing


def source_plan() -> dict:
    return {"arm_t": [{"call_id": c, "nonce": f"nonce-{c}"} for c in OPUS_GENERATION_IDS + TERRA_GENERATION_IDS + RETIRED_CALL_IDS]}


def test_the_expected_set_is_ten_opus_five_terra_two_relabels_one_retired_twelve_baselines():
    assert len(OPUS_GENERATION_IDS) == 10 and len(TERRA_GENERATION_IDS) == 5
    assert len(RELABEL_IDS) == 2 and RETIRED_CALL_IDS == ("generate__PARK-3~1__5",)
    assert len(EXPECTED_CANARY_FILES) == 12
    assert "generate__PARK-4~1__1" in OPUS_GENERATION_IDS and "generate__PARK-3~1__4" in TERRA_GENERATION_IDS


def test_a_sealed_source_yields_every_call_file_and_baseline_with_its_role():
    listing = sealed_listing()
    plan = plan_import(listing, dict(listing), source_plan())
    roles = {entry.role for entry in plan.entries}
    assert roles == set(ImportRole)
    assert plan.retired == RETIRED_CALL_IDS
    assert all(entry.source_path in listing and entry.digest == listing[entry.source_path] for entry in plan.entries)
    assert not any(entry.source_path.startswith("raw/canary") or entry.source_path in {"plan.json", "DEVIATIONS.md"} for entry in plan.entries)
    canary_targets = {e.target_path for e in plan.entries if e.role is ImportRole.CANARY_BASELINE}
    assert canary_targets == {f"{IMPORTED_CANARY_DIRECTORY}/{name}" for name in EXPECTED_CANARY_FILES}
    assert set(plan.nonces) == set(OPUS_GENERATION_IDS + TERRA_GENERATION_IDS)


def test_a_modified_file_is_refused():
    listing = sealed_listing()
    sums = dict(listing)
    listing["raw/relabel__R-OAI/result.json"] = digest("tampered")
    with pytest.raises(ImportRefused, match="modified or missing: raw/relabel__R-OAI/result.json"):
        plan_import(listing, sums, source_plan())


def test_a_file_the_seal_does_not_cover_is_refused():
    listing = sealed_listing()
    sums = dict(listing)
    listing["raw/extra/note.txt"] = digest("x")
    with pytest.raises(ImportRefused, match="not covered by SHA256SUMS"):
        plan_import(listing, sums, source_plan())


def test_a_missing_or_unexpected_completed_call_is_refused():
    listing = sealed_listing()
    del listing["raw/generate__PARK-2~1__2/result.json"]
    with pytest.raises(ImportRefused, match="missing completed calls"):
        plan_import(listing, dict(listing), source_plan())
    listing = sealed_listing()
    listing["raw/generate__PARK-9~1__1/result.json"] = digest("x")
    with pytest.raises(ImportRefused, match="unexpected completed calls"):
        plan_import(listing, dict(listing), source_plan())


def test_the_retired_call_is_never_imported_and_a_completed_retired_call_is_refused():
    listing = sealed_listing()
    listing["raw/generate__PARK-3~1__5/result.json"] = digest("done")
    with pytest.raises(ImportRefused, match="unexpected completed calls"):
        plan_import(listing, dict(listing), source_plan())


def test_a_missing_nonce_is_refused():
    listing = sealed_listing()
    plan = source_plan()
    plan["arm_t"] = plan["arm_t"][1:]
    with pytest.raises(ImportRefused, match="no nonce recorded"):
        plan_import(listing, dict(listing), plan)


def test_sums_round_trip_in_sha256sum_format_and_reject_malformed_lines():
    sums = {"b": digest("b"), "a/x y": digest("a")}
    assert parse_sums(render_sums(sums)) == sums
    with pytest.raises(ImportRefused):
        parse_sums("not a sums line\n")
