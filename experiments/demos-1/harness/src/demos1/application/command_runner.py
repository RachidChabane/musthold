"""PREREG 9.3: one container command from its first run to its settled end — waits, reruns, inspect then rm."""

from __future__ import annotations

from collections.abc import Callable, Sequence
from dataclasses import dataclass
from pathlib import PurePosixPath
from typing import Final

from demos1.application.ports import Clock, ContainerEngine, Files, HostRun, OperatorStopped
from demos1.domain.container import ContainerCommand
from demos1.domain.outcomes import MAX_RUNS
from demos1.domain.records import CommandRecord, RunRecord
from demos1.domain.window import format_utc

NO_SLOT_EXIT: Final = 75
GUARD_REFUSED_EXIT: Final = 4
GUARD_INTERRUPTED_EXIT: Final = 5
ENGINE_REFUSED_EXIT: Final = 125
NO_SLOT_MARK: Final = "with-slot:"
GUARD_REFUSED_MARK: Final = "mem-guard: refused"
GUARD_INTERRUPTED_MARK: Final = "mem-guard: memory pressure left normal"
POLL_SECONDS: Final = 60.0
REFUSAL_BOUND_SECONDS: Final = 6 * 3600.0
SCAN_CAP_SECONDS: Final = 4 * 3600.0
SCAN_SLOTS: Final = 2
SETUP_SLOTS: Final = 1
STDOUT: Final = ".stdout"
STDERR: Final = ".stderr"
LOG: Final = ".log"
CLIENT_STDERR: Final = ".client.err"


class WaitBoundReached(RuntimeError):
    """Refusals added up to 6 hours within one invocation: the verb stops and resumes at the same command."""


@dataclass
class RefusalBudget:
    """The refusals of one invocation of `attempt`: neither a run nor an attempt, but bounded."""

    bound_seconds: float = REFUSAL_BOUND_SECONDS
    spent_seconds: float = 0.0

    def spend(self, seconds: float) -> None:
        self.spent_seconds += max(0.0, seconds)
        if self.spent_seconds >= self.bound_seconds:
            raise WaitBoundReached(f"refusals added up to {self.spent_seconds:.0f} s within this invocation")


@dataclass(frozen=True)
class PlannedRun:
    """One run of a command: its container, the new directories it needs, and where its files go."""

    command: ContainerCommand
    fresh: Sequence[PurePosixPath]
    prefix: PurePosixPath
    cleared: Sequence[PurePosixPath] = ()
    """What an earlier run of the command may have left half built; removed before every run after the first."""


@dataclass(frozen=True)
class CommandPlan:
    slots: int
    cap_seconds: float | None
    run: Callable[[int], PlannedRun]
    """The planned run of a given run number (1 for `-r1`)."""


def files_of(prefix: PurePosixPath) -> tuple[str, str, str, str]:
    return str(prefix) + STDOUT, str(prefix) + STDERR, str(prefix) + LOG, str(prefix) + CLIENT_STDERR


class CommandRunner:
    def __init__(
        self,
        engine: ContainerEngine,
        files: Files,
        clock: Clock,
        refusals: RefusalBudget,
        persist: Callable[[], None],
    ) -> None:
        self._engine = engine
        self._files = files
        self._clock = clock
        self._refusals = refusals
        self._persist = persist

    def settle(self, plan: CommandPlan, record: CommandRecord) -> None:
        """Run the command until it ends, or until a memory guard has interrupted it three times."""
        while not record.settled:
            planned = plan.run(record.next_run())
            name = planned.command.name
            if self._engine.exists(name):
                record.runs.append(self._recover(planned, plan.slots))
                self._persist()
                continue
            if not self._engine.available():
                self._wait(0.0)
                continue
            if record.runs:
                for path in planned.cleared:
                    self._files.remove_tree(path)
            self._files.make_fresh(planned.fresh)
            try:
                host = self._engine.run(planned.command, plan.slots, plan.cap_seconds, planned.prefix)
            except OperatorStopped as stopped:
                record.runs.append(self._finish(planned, plan.slots, stopped.host, stopped_by_operator=True))
                self._persist()
                raise
            if self._refused(host, planned):
                self._wait((host.ended_at - host.started_at).total_seconds())
                continue
            interrupted = host.exit_status == GUARD_INTERRUPTED_EXIT and GUARD_INTERRUPTED_MARK in host.client_stderr
            if interrupted and self._engine.exists(name):
                self._engine.stop(name)
            record.runs.append(self._finish(planned, plan.slots, host, interrupted=interrupted))
            if not interrupted:
                record.settled = True
            elif record.interruptions() >= MAX_RUNS:
                record.settled = True
                record.interrupted_out = True
            self._persist()

    def _wait(self, refused_for: float) -> None:
        self._refusals.spend(refused_for)
        self._clock.sleep(POLL_SECONDS)
        self._refusals.spend(POLL_SECONDS)

    def _refused(self, host: HostRun, planned: PlannedRun) -> bool:
        """with-slot exit 75, mem-guard exit 4 or `docker run` exit 125, before the command ever started."""
        status, stderr = host.exit_status, host.client_stderr
        started = self._files.read_measure(planned.prefix) is not None
        if started:
            return False
        name = planned.command.name
        if status == NO_SLOT_EXIT and NO_SLOT_MARK in stderr and not self._engine.exists(name):
            return True
        if status == GUARD_REFUSED_EXIT and GUARD_REFUSED_MARK in stderr and not self._engine.exists(name):
            return True
        if status == ENGINE_REFUSED_EXIT:
            if self._engine.exists(name):
                self._engine.remove(name)
            return True
        return False

    def _recover(self, planned: PlannedRun, slots: int) -> RunRecord:
        """A container of this name the verb left behind when it stopped: inspected, removed and recorded."""
        now = self._clock.now()
        host = HostRun(exit_status=-1, client_stderr="", killed_at_cap=False, started_at=now, ended_at=now)
        if self._engine.running(planned.command.name):
            self._engine.stop(planned.command.name)
        return self._finish(planned, slots, host, stopped_by_operator=True)

    def _finish(
        self,
        planned: PlannedRun,
        slots: int,
        host: HostRun,
        *,
        interrupted: bool = False,
        stopped_by_operator: bool = False,
    ) -> RunRecord:
        """Record `docker inspect`, then `docker rm`, then what the wrapper measured."""
        command = planned.command
        exit_code: int | None = None
        oom_killed: bool | None = None
        if self._engine.exists(command.name):
            state = self._engine.inspect(command.name)
            exit_code, oom_killed = state.exit_code, state.oom_killed
            self._engine.remove(command.name)
        measure = self._files.read_measure(planned.prefix)
        stdout, stderr, log, client_stderr = files_of(planned.prefix)
        description = dict(command.describe())
        description["host_argv"] = self._engine.host_argv(command, slots)
        return RunRecord(
            name=command.name,
            command=description,
            network=command.network,
            started_at=format_utc(host.started_at),
            ended_at=format_utc(host.ended_at),
            host_exit=host.exit_status,
            stdout=stdout,
            stderr=stderr,
            log=log,
            client_stderr=client_stderr,
            wall_seconds=measure.wall_seconds if measure is not None else None,
            memory_peak_bytes=measure.memory_peak_bytes if measure is not None else None,
            timed_out=measure.timed_out if measure is not None else None,
            exit_code=exit_code,
            oom_killed=oom_killed,
            interrupted=interrupted,
            killed_at_cap=host.killed_at_cap,
            stopped_by_operator=stopped_by_operator,
        )
