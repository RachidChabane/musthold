from pathlib import Path, PurePosixPath

import pytest

from demos1.adapters.local import LocalFiles
from demos1.application.command_runner import (
    CommandPlan,
    CommandRunner,
    PlannedRun,
    RefusalBudget,
    WaitBoundReached,
)
from demos1.domain.container import ContainerCommand, ContainerState
from demos1.domain.records import CommandRecord
from support.fakes import FakeClock, Played, ScriptedEngine


def plan(tmp_path: Path, stage: str = "install") -> CommandPlan:
    def planned(run: int) -> PlannedRun:
        prefix = PurePosixPath(tmp_path / "logs" / f"{stage}-r{run}")
        command = ContainerCommand(
            name=f"B-go-01-{stage}-r{run}",
            image="sha256:image",
            attempt_dir=str(tmp_path),
            binaries_dir="/product/bin",
            workdir=str(tmp_path),
            network=False,
            environment={"TMPDIR": str(tmp_path / "tmp" / f"{stage}-r{run}")},
            argv=["true"],
            record_prefix=str(prefix),
            limit_seconds=None,
            uid=502,
            gid=20,
        )
        return PlannedRun(command, (PurePosixPath(tmp_path / "tmp" / f"{stage}-r{run}"),), prefix)

    return CommandPlan(1, None, planned)


def runner(engine: ScriptedEngine, clock: FakeClock, bound: float = 6 * 3600.0) -> CommandRunner:
    return CommandRunner(engine, LocalFiles(), clock, RefusalBudget(bound), lambda: None)


def test_a_container_is_inspected_then_removed_and_its_run_recorded(tmp_path: Path):
    engine = ScriptedEngine(lambda command: Played(exit_code=1, oom_killed=True))
    record = CommandRecord()
    runner(engine, FakeClock()).settle(plan(tmp_path), record)
    assert engine.calls == [
        ("run", "B-go-01-install-r1"),
        ("inspect", "B-go-01-install-r1"),
        ("rm", "B-go-01-install-r1"),
    ]
    (run,) = record.runs
    assert (run.exit_code, run.oom_killed, run.wall_seconds, run.memory_peak_bytes) == (1, True, 1.0, 52_428_800)
    assert run.command["host_argv"] == engine.host_argv(engine.commands[0], 1)
    assert record.settled and not record.interrupted_out
    assert engine.containers == {}


def test_slot_and_guard_refusals_are_neither_a_run_nor_an_attempt(tmp_path: Path):
    answers = iter(
        [
            Played(started=False, host_exit=75, client_stderr="with-slot: 1 slot(s) not free after 3600s"),
            Played(started=False, host_exit=4, client_stderr="mem-guard: refused (below threshold)"),
            Played(exit_code=0),
        ]
    )
    engine = ScriptedEngine(lambda command: next(answers))
    clock = FakeClock()
    record = CommandRecord()
    runner(engine, clock).settle(plan(tmp_path), record)
    assert [name for kind, name in engine.calls if kind == "run"] == ["B-go-01-install-r1"] * 3
    assert len(record.runs) == 1
    assert clock.sleeps == [60.0, 60.0]


def test_a_guard_interruption_stops_the_container_and_reruns_it_up_to_three_runs(tmp_path: Path):
    interrupted = Played(
        exit_code=143,
        host_exit=5,
        client_stderr="mem-guard: memory pressure left normal; stopping the batch",
        leave_running=True,
    )
    engine = ScriptedEngine(lambda command: interrupted)
    record = CommandRecord()
    runner(engine, FakeClock()).settle(plan(tmp_path), record)
    assert [run.name for run in record.runs] == ["B-go-01-install-r1", "B-go-01-install-r2", "B-go-01-install-r3"]
    assert all(run.interrupted for run in record.runs)
    assert record.interrupted_out
    assert ("stop", "B-go-01-install-r1") in engine.calls
    assert engine.containers == {}


def test_an_exit_5_without_the_guard_message_is_the_command_own(tmp_path: Path):
    engine = ScriptedEngine(lambda command: Played(exit_code=5))
    record = CommandRecord()
    runner(engine, FakeClock()).settle(plan(tmp_path), record)
    assert len(record.runs) == 1
    assert not record.runs[0].interrupted


def test_refusals_that_add_up_to_the_bound_stop_the_verb(tmp_path: Path):
    engine = ScriptedEngine(lambda command: Played(started=False, host_exit=4, client_stderr="mem-guard: refused"))
    with pytest.raises(WaitBoundReached):
        runner(engine, FakeClock(), bound=600.0).settle(plan(tmp_path), CommandRecord())


def test_an_engine_that_refuses_is_waited_for(tmp_path: Path):
    engine = ScriptedEngine(lambda command: Played())
    engine.engine_up = False
    clock = FakeClock()

    def back_up(seconds: float) -> None:
        clock.sleeps.append(seconds)
        engine.engine_up = True

    clock.sleep = back_up  # type: ignore[method-assign]
    record = CommandRecord()
    runner(engine, clock).settle(plan(tmp_path), record)
    assert clock.sleeps == [60.0]
    assert len(record.runs) == 1


def test_a_container_left_by_a_stopped_verb_is_recorded_and_the_command_reruns_under_the_next_name(tmp_path: Path):
    engine = ScriptedEngine(lambda command: Played())
    engine.containers["B-go-01-install-r1"] = ContainerState(137, False)
    engine.running_names.add("B-go-01-install-r1")
    record = CommandRecord()
    runner(engine, FakeClock()).settle(plan(tmp_path), record)
    assert [run.name for run in record.runs] == ["B-go-01-install-r1", "B-go-01-install-r2"]
    assert record.runs[0].stopped_by_operator
    assert ("stop", "B-go-01-install-r1") in engine.calls


def test_a_rerun_after_an_interruption_starts_without_what_the_first_run_left_half_built(tmp_path: Path):
    venv = tmp_path / "clone" / ".venv"
    seen: list[bool] = []
    answers = iter(
        [
            Played(
                exit_code=143, host_exit=5, client_stderr="mem-guard: memory pressure left normal; stopping the batch"
            ),
            Played(exit_code=0),
        ]
    )

    def play(command: ContainerCommand) -> Played:
        seen.append(venv.exists())
        if not seen[1:]:
            (venv / "bin").mkdir(parents=True)
        return next(answers)

    base = plan(tmp_path)

    def planned(run: int) -> PlannedRun:
        first = base.run(run)
        return PlannedRun(first.command, first.fresh, first.prefix, (PurePosixPath(venv),))

    record = CommandRecord()
    runner(ScriptedEngine(play), FakeClock()).settle(CommandPlan(1, None, planned), record)
    assert seen == [False, False]
    assert [run.interrupted for run in record.runs] == [True, False]
