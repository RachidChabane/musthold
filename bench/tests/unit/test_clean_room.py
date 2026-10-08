from __future__ import annotations

from pathlib import Path

from specbench.domain.arms import Arm
from specbench.domain.clean_room import PORTABLE_ROOT, CleanRoom, portable

DIGEST = "ab" * 32


def test_the_room_of_a_gate_is_named_by_its_arm_and_the_digest_of_its_tree() -> None:
    room = CleanRoom.locate(Path("/runs/r1/cleanroom/task-1"), Arm.ALL, DIGEST)
    assert room.root == Path(f"/runs/r1/cleanroom/task-1/ALL/{DIGEST}")
    assert room.session.tree == room.root / "session" / "tree"
    assert room.settle.state == room.root / "settle" / "state"
    assert room.settle.cache == room.root / "settle" / "cache"


def test_the_two_areas_of_a_room_are_apart() -> None:
    room = CleanRoom.locate(Path("/rooms"), Arm.AL, DIGEST)
    assert room.session.root != room.settle.root
    assert room.session.ready_marker.parent == room.session.root


def test_an_area_is_ready_once_its_marker_exists(tmp_path: Path) -> None:
    area = CleanRoom.locate(tmp_path, Arm.DP, DIGEST).session
    assert not area.is_ready
    area.root.mkdir(parents=True)
    area.ready_marker.write_text("ready")
    assert area.is_ready


def test_the_tree_path_reads_as_the_portable_root_in_every_string() -> None:
    report = {"run": {"spec_files": ["/tmp/room/tree/specs/a.md"]}, "n": 3, "ok": True, "x": None}
    assert portable(report, ["/tmp/room/tree"]) == {
        "run": {"spec_files": [f"{PORTABLE_ROOT}/specs/a.md"]},
        "n": 3,
        "ok": True,
        "x": None,
    }


def test_the_longest_path_wins_when_one_contains_the_other() -> None:
    assert portable("/a/b/c/file", ["/a/b", "/a/b/c"]) == f"{PORTABLE_ROOT}/file"
