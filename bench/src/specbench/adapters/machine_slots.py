"""Machine-wide execution slots: at most two heavy jobs of the benchmark and the night jobs run at once.

A slot is an exclusive `flock` on `slot-<n>.lock` in the state directory. The lock belongs to the open file, so a
holder that exits or is killed frees its slot at once and no lock file is ever stale. `script/with-slot` loads this
module by path with `python3 -I`, so it imports the standard library only.
"""

from __future__ import annotations

import fcntl
import os
import time
from collections.abc import Callable
from pathlib import Path
from typing import IO, Final

SLOT_COUNT: Final = 2
SLOT_FILE_TEMPLATE: Final = "slot-{index}.lock"
STATE_DIRECTORY_ENV: Final = "MUSTHOLD_STATE_DIR"
SLOTS_DIRECTORY_ENV: Final = "MUSTHOLD_SLOTS_DIR"
SLOTS_DIRECTORY_NAME: Final = "slots"
HELD_SLOTS_ENV: Final = "MUSTHOLD_SLOTS_HELD"
DEFAULT_POLL_SECONDS: Final = 5.0
_STATE_RELATIVE_PATH: Final = Path("Library") / "Application Support" / "musthold"


def default_state_directory() -> Path:
    """`$MUSTHOLD_STATE_DIR`, else `~/Library/Application Support/musthold`."""
    configured = os.environ.get(STATE_DIRECTORY_ENV)
    return Path(configured) if configured else Path.home() / _STATE_RELATIVE_PATH


def default_slots_directory() -> Path:
    configured = os.environ.get(SLOTS_DIRECTORY_ENV)
    return Path(configured) if configured else default_state_directory() / SLOTS_DIRECTORY_NAME


class SlotHold:
    """The slots one holder owns. Release them, use the hold as a context manager, or exit the process."""

    def __init__(self, files: dict[int, IO[bytes]]) -> None:
        self._files = files

    @property
    def indices(self) -> tuple[int, ...]:
        return tuple(sorted(self._files))

    def make_inheritable(self) -> None:
        """Let the descriptors survive `exec`, so the slots last as long as the command that replaces this process."""
        for file in self._files.values():
            os.set_inheritable(file.fileno(), True)

    def release(self) -> None:
        for file in self._files.values():
            file.close()
        self._files = {}

    def __enter__(self) -> SlotHold:
        return self

    def __exit__(self, *_: object) -> None:
        self.release()


class MachineSlots:
    """The `SLOT_COUNT` slots of one directory."""

    def __init__(self, directory: Path, capacity: int = SLOT_COUNT) -> None:
        self._directory = directory
        self._capacity = capacity

    def try_acquire(self, count: int) -> SlotHold | None:
        """Take `count` slots or none: a holder that gets only some of them gives those back."""
        if not 1 <= count <= self._capacity:
            raise ValueError(f"a job takes between 1 and {self._capacity} slots, not {count}")
        self._directory.mkdir(parents=True, exist_ok=True)
        taken: dict[int, IO[bytes]] = {}
        for index in range(self._capacity):
            if len(taken) == count:
                break
            file = self._try_lock(index)
            if file is not None:
                taken[index] = file
        if len(taken) < count:
            SlotHold(taken).release()
            return None
        return SlotHold(taken)

    def acquire(
        self,
        count: int,
        wait_seconds: float,
        poll_seconds: float = DEFAULT_POLL_SECONDS,
        sleep: Callable[[float], None] = time.sleep,
        monotonic: Callable[[], float] = time.monotonic,
    ) -> SlotHold | None:
        """Take `count` slots, polling for up to `wait_seconds`; None when they did not come free in time."""
        deadline = monotonic() + wait_seconds
        while True:
            hold = self.try_acquire(count)
            if hold is not None or monotonic() >= deadline:
                return hold
            sleep(min(poll_seconds, max(deadline - monotonic(), 0.0)))

    def free(self) -> int:
        """How many slots are free right now."""
        self._directory.mkdir(parents=True, exist_ok=True)
        return sum(self._is_free(index) for index in range(self._capacity))

    def _is_free(self, index: int) -> bool:
        file = self._try_lock(index)
        if file is None:
            return False
        file.close()
        return True

    def _try_lock(self, index: int) -> IO[bytes] | None:
        path = self._directory / SLOT_FILE_TEMPLATE.format(index=index)
        file = path.open("ab")
        try:
            fcntl.flock(file.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            file.close()
            return None
        return file
