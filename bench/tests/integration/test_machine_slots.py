from __future__ import annotations

import os
import signal
import subprocess
import sys
import time
from collections.abc import Callable
from pathlib import Path

import pytest

from specbench.adapters.machine_slots import (
    HELD_SLOTS_ENV,
    SLOT_COUNT,
    SLOT_FILE_TEMPLATE,
    SLOTS_DIRECTORY_ENV,
    STATE_DIRECTORY_ENV,
    MachineSlots,
    default_slots_directory,
)

pytestmark = pytest.mark.integration

WITH_SLOT = Path(__file__).resolve().parents[3] / "script" / "with-slot"
NO_SLOT_EXIT = 75
POLL_SECONDS = 0.05
PATIENCE_SECONDS = 10.0


def wait_until(condition: Callable[[], bool], patience: float = PATIENCE_SECONDS) -> bool:
    deadline = time.monotonic() + patience
    while time.monotonic() < deadline:
        if condition():
            return True
        time.sleep(POLL_SECONDS)
    return False


def start_holder(directory: Path, count: int) -> subprocess.Popen[bytes]:
    return subprocess.Popen([str(WITH_SLOT), str(count), "--dir", str(directory), "--", "sleep", "60"])


def test_two_jobs_of_one_slot_fill_the_machine_and_a_third_finds_none(tmp_path: Path) -> None:
    slots = MachineSlots(tmp_path)
    first, second = slots.try_acquire(1), slots.try_acquire(1)
    assert first is not None and second is not None
    assert sorted((*first.indices, *second.indices)) == [0, 1]
    assert slots.try_acquire(1) is None
    first.release()
    assert slots.free() == 1


def test_a_job_that_needs_two_slots_takes_none_while_one_is_held_and_leaks_nothing(tmp_path: Path) -> None:
    slots = MachineSlots(tmp_path)
    held = slots.try_acquire(1)
    assert held is not None
    assert slots.try_acquire(2) is None
    assert slots.free() == SLOT_COUNT - 1
    held.release()
    both = slots.try_acquire(2)
    assert both is not None and both.indices == (0, 1)


def test_waiting_gives_up_after_the_deadline_and_takes_a_slot_that_comes_free_in_time(tmp_path: Path) -> None:
    slots = MachineSlots(tmp_path)
    both = slots.try_acquire(2)
    assert both is not None
    clock = {"now": 0.0}

    def sleep(seconds: float) -> None:
        clock["now"] += seconds

    assert slots.acquire(1, wait_seconds=3, poll_seconds=1, sleep=sleep, monotonic=lambda: clock["now"]) is None
    assert clock["now"] == 3

    def release_on_second_poll(seconds: float) -> None:
        clock["now"] += seconds
        if clock["now"] >= 2:
            both.release()

    clock["now"] = 0.0
    assert slots.acquire(1, 10, 1, sleep=release_on_second_poll, monotonic=lambda: clock["now"]) is not None


def test_a_holder_that_is_killed_frees_its_slot(tmp_path: Path) -> None:
    slots = MachineSlots(tmp_path)
    holder = start_holder(tmp_path, 2)
    try:
        assert wait_until(lambda: slots.free() == 0)
        assert slots.try_acquire(1) is None
        holder.send_signal(signal.SIGKILL)
        holder.wait(timeout=PATIENCE_SECONDS)
        assert slots.free() == SLOT_COUNT
    finally:
        holder.kill()
        holder.wait()


def test_the_wrapper_refuses_to_start_a_command_when_the_slots_are_taken(tmp_path: Path) -> None:
    slots = MachineSlots(tmp_path)
    held = slots.try_acquire(2)
    assert held is not None
    marker = tmp_path / "ran"
    refused = subprocess.run(
        [str(WITH_SLOT), "1", "--dir", str(tmp_path), "--", "touch", str(marker)], check=False, capture_output=True
    )
    assert refused.returncode == NO_SLOT_EXIT and not marker.exists()
    held.release()
    allowed = subprocess.run(
        [str(WITH_SLOT), "1", "--dir", str(tmp_path), "--", "touch", str(marker)], check=False, capture_output=True
    )
    assert allowed.returncode == 0 and marker.exists()


def test_the_command_sees_the_slots_it_holds_and_its_own_exit_status_comes_back(tmp_path: Path) -> None:
    done = subprocess.run(
        [str(WITH_SLOT), "2", "--dir", str(tmp_path), "--", "sh", "-c", f'echo "${HELD_SLOTS_ENV}"; exit 7'],
        check=False,
        capture_output=True,
        text=True,
    )
    assert done.returncode == 7 and done.stdout.strip() == "0,1"


def test_a_slot_stays_held_for_as_long_as_the_command_lives(tmp_path: Path) -> None:
    slots = MachineSlots(tmp_path)
    holder = start_holder(tmp_path, 1)
    try:
        assert wait_until(lambda: slots.free() == 1)
        time.sleep(0.3)
        assert slots.free() == 1
    finally:
        holder.kill()
        holder.wait()


@pytest.mark.parametrize("arguments", [["0"], ["3"], ["1"]])
def test_the_wrapper_rejects_a_bad_slot_count_or_a_missing_command(tmp_path: Path, arguments: list[str]) -> None:
    done = subprocess.run([str(WITH_SLOT), *arguments, "--dir", str(tmp_path)], check=False, capture_output=True)
    assert done.returncode == 2


def test_the_default_directory_lives_in_the_state_directory_unless_overridden(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    monkeypatch.delenv(SLOTS_DIRECTORY_ENV, raising=False)
    monkeypatch.setenv(STATE_DIRECTORY_ENV, str(tmp_path))
    assert default_slots_directory() == tmp_path / "slots"
    monkeypatch.setenv(SLOTS_DIRECTORY_ENV, str(tmp_path / "elsewhere"))
    assert default_slots_directory() == tmp_path / "elsewhere"
    assert SLOT_FILE_TEMPLATE.format(index=1) == "slot-1.lock"


def test_the_wrapper_runs_under_isolated_mode_without_the_package_installed(tmp_path: Path) -> None:
    environment = {key: value for key, value in os.environ.items() if key != "PYTHONPATH"}
    done = subprocess.run(
        [sys.executable, "-I", str(WITH_SLOT), "1", "--dir", str(tmp_path), "--", "true"],
        check=False,
        capture_output=True,
        env=environment,
    )
    assert done.returncode == 0
