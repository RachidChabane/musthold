from __future__ import annotations

import sys
from pathlib import Path

import pytest

from calibration.adapters.isolated_import import import_pilot_modules, isolated_pilot_path
from calibration.adapters.pilot_snapshot import UnpackedSnapshot
from pilot_layout import unpack_snapshot

NORMALIZATION = "pilot.domain.normalization"


def pilot_modules_loaded() -> list[str]:
    return [name for name in sys.modules if name == "pilot" or name.startswith("pilot.")]


@pytest.fixture
def snapshots(tmp_path: Path) -> tuple[UnpackedSnapshot, UnpackedSnapshot]:
    first = UnpackedSnapshot(unpack_snapshot(tmp_path / "first", marker="first"))
    second = UnpackedSnapshot(unpack_snapshot(tmp_path / "second", marker="second"))
    return first, second


def test_two_snapshots_never_share_a_module(snapshots: tuple[UnpackedSnapshot, UnpackedSnapshot]) -> None:
    first, second = snapshots
    (from_first,) = import_pilot_modules(first.pilot_source_root, [NORMALIZATION])
    (from_second,) = import_pilot_modules(second.pilot_source_root, [NORMALIZATION])
    assert from_first.SNAPSHOT_MARKER == "first"
    assert from_second.SNAPSHOT_MARKER == "second"
    assert from_first is not from_second
    assert Path(str(from_first.__file__)).is_relative_to(first.pilot_source_root)
    assert Path(str(from_second.__file__)).is_relative_to(second.pilot_source_root)


def test_the_interpreter_is_left_as_it_was(snapshots: tuple[UnpackedSnapshot, UnpackedSnapshot]) -> None:
    first, _ = snapshots
    path_before = list(sys.path)
    bytecode_before = sys.dont_write_bytecode
    import_pilot_modules(first.pilot_source_root, [NORMALIZATION, "pilot.domain.matrix"])
    assert sys.path == path_before
    assert sys.dont_write_bytecode == bytecode_before
    assert pilot_modules_loaded() == []


def fail_inside_the_isolated_path(source_root: Path) -> None:
    with isolated_pilot_path(source_root):
        __import__(NORMALIZATION)
        assert sys.path[0] == str(source_root)
        raise RuntimeError


def test_the_interpreter_is_restored_when_the_body_raises(snapshots: tuple[UnpackedSnapshot, UnpackedSnapshot]) -> None:
    first, _ = snapshots
    path_before = list(sys.path)
    with pytest.raises(RuntimeError):
        fail_inside_the_isolated_path(first.pilot_source_root)
    assert sys.path == path_before
    assert pilot_modules_loaded() == []


def test_a_pilot_module_loaded_before_is_purged_so_the_snapshot_wins(
    snapshots: tuple[UnpackedSnapshot, UnpackedSnapshot],
) -> None:
    first, second = snapshots
    with isolated_pilot_path(first.pilot_source_root):
        __import__(NORMALIZATION)
        assert pilot_modules_loaded()
    with isolated_pilot_path(second.pilot_source_root):
        module = __import__(NORMALIZATION, fromlist=["SNAPSHOT_MARKER"])
        assert module.SNAPSHOT_MARKER == "second"


def test_no_bytecode_is_written_below_the_snapshot(snapshots: tuple[UnpackedSnapshot, UnpackedSnapshot]) -> None:
    first, _ = snapshots
    import_pilot_modules(first.pilot_source_root, [NORMALIZATION])
    assert not list(first.pilot_source_root.rglob("__pycache__"))


def test_a_missing_module_raises_and_still_restores(tmp_path: Path) -> None:
    path_before = list(sys.path)
    with pytest.raises(ModuleNotFoundError):
        import_pilot_modules(tmp_path, [NORMALIZATION])
    assert sys.path == path_before
    assert pilot_modules_loaded() == []
