"""The outcome names of PREREG 10 and the classification of how a container command ended (9.2, 9.3)."""

from __future__ import annotations

from dataclasses import dataclass
from enum import StrEnum
from typing import Final


class SetupOutcome(StrEnum):
    CLONE = "setup-clone"
    TOOLCHAIN = "setup-toolchain"
    INSTALL = "setup-install"
    DIRTY_TREE = "setup-dirty-tree"
    SUITE_FIRST_RUN_TIMEOUT = "setup-suite-first-run-timeout"
    SUITE_TOO_SLOW = "setup-suite-too-slow"
    SUITE_DOES_NOT_RUN = "setup-suite-does-not-run"
    TOO_FEW_PASSING = "setup-too-few-passing"
    OOM = "setup-oom"
    DISK = "setup-disk"
    INTERRUPTED = "setup-interrupted"


PICK: Final = "pick"


class ScanOutcome(StrEnum):
    OK = "scan-ok"
    ERROR = "scan-error"
    KILLED = "scan-killed"
    OOM = "scan-oom"
    DISK = "scan-disk"
    INTERRUPTED = "scan-interrupted"


class ReplayResult(StrEnum):
    HOLDS = "holds on replay"
    NO_LONGER_HOLDS = "no longer holds on replay"
    FAILED = "replay failed"


REPLAY_HOLDS_EXIT: Final = 0
REPLAY_REFUTED_EXIT: Final = 1
MAX_RUNS: Final = 3
"""A command a memory guard interrupts runs again from the start, up to 3 runs in all."""


@dataclass(frozen=True)
class CommandEnd:
    """How the last run of a container command ended, as the verb observed it."""

    exit_code: int | None
    oom_killed: bool
    timed_out: bool
    killed_at_cap: bool
    interrupted_out: bool
    """True after the third run the memory guard interrupted."""


def setup_terminal(end: CommandEnd) -> SetupOutcome | None:
    """The outcomes every setup command shares: 3 guard interruptions, then a memory-limit stop."""
    if end.interrupted_out:
        return SetupOutcome.INTERRUPTED
    if end.oom_killed:
        return SetupOutcome.OOM
    return None


def setup_failed(end: CommandEnd, failure: SetupOutcome) -> SetupOutcome | None:
    """A stage whose only own outcome is a non-zero exit or its limit: clone, toolchain, install."""
    terminal = setup_terminal(end)
    if terminal is not None:
        return terminal
    if end.timed_out or end.exit_code != 0:
        return failure
    return None


def scan_outcome(end: CommandEnd) -> ScanOutcome:
    if end.interrupted_out:
        return ScanOutcome.INTERRUPTED
    if end.oom_killed:
        return ScanOutcome.OOM
    if end.killed_at_cap:
        return ScanOutcome.KILLED
    if end.exit_code == 0:
        return ScanOutcome.OK
    return ScanOutcome.ERROR


def replay_result(end: CommandEnd) -> ReplayResult:
    if end.interrupted_out or end.oom_killed or end.killed_at_cap:
        return ReplayResult.FAILED
    if end.exit_code == REPLAY_HOLDS_EXIT:
        return ReplayResult.HOLDS
    if end.exit_code == REPLAY_REFUTED_EXIT:
        return ReplayResult.NO_LONGER_HOLDS
    return ReplayResult.FAILED
