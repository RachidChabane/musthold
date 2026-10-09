from __future__ import annotations

import json
from pathlib import Path

import pytest

from specbench.adapters.ci_round_archive import CiRoundArchive, read_manifest
from specbench.adapters.host_bundle_merge import merge_bundles
from specbench.domain.host_bundle import ForeignBundle, merge_bundle_manifests, parse_bundle_manifest

HEAD = "a" * 40
OTHER_HEAD = "b" * 40


def manifest_text(head: str, *responses: tuple[str, int]) -> str:
    entries = [{"requestId": request_id, "vote": vote} for request_id, vote in responses]
    return json.dumps(
        {
            "module": {"name": "spec-proof-dp", "version": "dev"},
            "headSha": head,
            "journalDigest": "d",
            "responses": entries,
        }
    )


def export(directory: Path, head: str, *responses: tuple[str, int]) -> Path:
    directory.mkdir(parents=True)
    (directory / "manifest.json").write_text(manifest_text(head, *responses), encoding="utf-8")
    (directory / "responses").mkdir()
    for request_id, vote in responses:
        (directory / "responses" / f"{request_id}.{vote}.json").write_text("{}", encoding="utf-8")
    return directory


def test_a_manifest_names_its_commit_and_the_responses_it_lists() -> None:
    manifest = parse_bundle_manifest(manifest_text(HEAD, ("r1", 0), ("r1", 1), ("r2", 0)))
    assert manifest.head_sha == HEAD
    assert manifest.responses == {("r1", 0), ("r1", 1), ("r2", 0)}


def test_a_bundle_pushed_for_another_commit_is_refused() -> None:
    manifest = parse_bundle_manifest(manifest_text(OTHER_HEAD))
    manifest.require_head(OTHER_HEAD)
    with pytest.raises(ForeignBundle, match="not for"):
        manifest.require_head(HEAD)


def test_rounds_are_counted_from_the_first_and_only_while_consecutive(tmp_path: Path) -> None:
    archive = CiRoundArchive(tmp_path / "rec")
    assert archive.rounds() == 0
    archive.archive(1, {"discpower": export(tmp_path / "x1" / "discpower", HEAD, ("r1", 0))})
    archive.archive(3, {"discpower": export(tmp_path / "x3" / "discpower", HEAD, ("r1", 0))})
    assert archive.rounds() == 1
    archive.archive(2, {"discpower": export(tmp_path / "x2" / "discpower", HEAD, ("r1", 0))})
    assert archive.rounds() == 3


def test_a_round_is_visible_only_once_all_its_modules_are_in_place(tmp_path: Path) -> None:
    archive = CiRoundArchive(tmp_path / "rec")
    archive.archive(
        1,
        {
            "discpower": export(tmp_path / "x" / "discpower", HEAD, ("r1", 0)),
            "alignment": export(tmp_path / "x" / "alignment", HEAD, ("r2", 0)),
        },
    )
    assert sorted(path.name for path in (tmp_path / "rec" / "ci" / "1").iterdir()) == ["alignment", "discpower"]
    assert not list((tmp_path / "rec" / "ci").glob("*.partial"))


def test_the_latest_bundle_of_a_module_is_the_latest_round_that_archived_it(tmp_path: Path) -> None:
    archive = CiRoundArchive(tmp_path / "rec")
    archive.archive(1, {"discpower": export(tmp_path / "a" / "discpower", HEAD, ("r1", 0))})
    archive.archive(
        2,
        {
            "alignment": export(tmp_path / "b" / "alignment", HEAD, ("r2", 0)),
            "discpower": export(tmp_path / "b" / "discpower", HEAD, ("r1", 0), ("r3", 0)),
        },
    )
    archive.archive(3, {"alignment": export(tmp_path / "c" / "alignment", HEAD, ("r2", 0), ("r4", 0))})
    assert archive.latest_bundle("discpower", 3) == tmp_path / "rec" / "ci" / "2" / "discpower"
    assert archive.latest_bundle("discpower", 1) == tmp_path / "rec" / "ci" / "1" / "discpower"
    assert archive.latest_bundle("alignment", 3) == tmp_path / "rec" / "ci" / "3" / "alignment"
    assert archive.latest_bundle("completeness", 3) is None
    assert read_manifest(tmp_path / "rec" / "ci" / "2" / "discpower").responses == {("r1", 0), ("r3", 0)}


def test_archiving_a_round_again_replaces_it(tmp_path: Path) -> None:
    archive = CiRoundArchive(tmp_path / "rec")
    archive.archive(1, {"discpower": export(tmp_path / "a" / "discpower", HEAD, ("r1", 0))})
    archive.archive(1, {"discpower": export(tmp_path / "b" / "discpower", HEAD, ("r9", 0))})
    assert read_manifest(tmp_path / "rec" / "ci" / "1" / "discpower").responses == {("r9", 0)}


def test_merged_manifests_keep_the_overlay_entry_where_both_list_a_response() -> None:
    base = json.dumps({"headSha": HEAD, "responses": [{"requestId": "r1", "vote": 0}, {"requestId": "r2", "vote": 0}]})
    overlay = json.dumps(
        {"headSha": HEAD, "journalDigest": "new", "responses": [{"requestId": "r2", "vote": 0, "marked": True}]}
    )
    merged = json.loads(merge_bundle_manifests(base, overlay))
    assert merged["journalDigest"] == "new"
    assert merged["responses"] == [{"requestId": "r1", "vote": 0}, {"requestId": "r2", "vote": 0, "marked": True}]


def test_merged_bundles_hold_the_files_of_both_and_a_manifest_that_lists_exactly_them(tmp_path: Path) -> None:
    earlier = export(tmp_path / "earlier", HEAD, ("r1", 0), ("r2", 0))
    (earlier / "responses" / "r2.0.json").write_text('{"old": true}', encoding="utf-8")
    later = export(tmp_path / "later", HEAD, ("r2", 0), ("r3", 0))
    (later / "responses" / "r2.0.json").write_text('{"new": true}', encoding="utf-8")
    merged = merge_bundles(earlier, later, tmp_path / "merged")
    assert sorted(path.name for path in (merged / "responses").iterdir()) == ["r1.0.json", "r2.0.json", "r3.0.json"]
    assert (merged / "responses" / "r2.0.json").read_text(encoding="utf-8") == '{"new": true}'
    assert read_manifest(merged).responses == {("r1", 0), ("r2", 0), ("r3", 0)}
    assert merge_bundles(earlier, later, merged) == merged
