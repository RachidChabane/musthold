from __future__ import annotations

import os
import socket
import sys
import time
from pathlib import Path

import pytest
from hypothesis import given
from hypothesis import strategies as st

from calibration.adapters.process_tree import (
    PS_ARGV,
    ProcessEntry,
    ProcessTable,
    ProcessTableError,
    ProcessTreeTerminator,
    ServerCommand,
    ancestors_of,
    descendants_of,
    is_alive,
    parse_process_table,
    port_is_free,
    spawn_process,
)
from calibration.adapters.subprocess_runner import SubprocessRunner
from calibration.adapters.system_time import SystemClock, SystemSleeper
from calibration.domain.ports import ProcessResult
from support.fakes import ScriptedRunner

PS_TEXT = """\
    1     0     1 /sbin/launchd
  100     1   100 -zsh -l
  200   100   200 python -m thing --flag "quoted arg"
  201   200   200
garbage line
"""

ENTRIES = parse_process_table(PS_TEXT)
GRACE_SECONDS = 1.0
SLEEP_CODE = "import time; time.sleep(60)"
STUBBORN_CODE = "import signal, time; signal.signal(signal.SIGTERM, signal.SIG_IGN); time.sleep(60)"


def test_ps_lines_are_parsed_with_their_whole_command() -> None:
    assert ProcessEntry(200, 100, 200, 'python -m thing --flag "quoted arg"') in ENTRIES
    assert ProcessEntry(201, 200, 200, "") in ENTRIES
    assert len(ENTRIES) == 4


@given(st.text())
def test_parsing_never_raises(text: str) -> None:
    assert isinstance(parse_process_table(text), tuple)


@given(
    st.lists(
        st.tuples(st.integers(1, 30), st.integers(0, 30), st.integers(0, 30)),
        max_size=40,
    ),
    st.integers(0, 30),
)
def test_tree_walks_terminate_on_any_table(rows: list[tuple[int, int, int]], pid: int) -> None:
    entries = [ProcessEntry(row[0], row[1], row[2], "x") for row in rows]
    assert pid not in descendants_of(entries, pid)
    assert pid not in ancestors_of(entries, pid)


def test_descendants_and_ancestors() -> None:
    assert descendants_of(ENTRIES, 100) == {200, 201}
    assert ancestors_of(ENTRIES, 201) == (200, 100, 1, 0)


def test_ps_failure_is_an_error() -> None:
    with pytest.raises(ProcessTableError):
        ProcessTable(ScriptedRunner({PS_ARGV: ProcessResult(1, b"", b"")})).snapshot()


def test_the_real_ps_lists_this_process_below_its_parent() -> None:
    entries = ProcessTable(SubprocessRunner()).snapshot()
    me = next(entry for entry in entries if entry.pid == os.getpid())
    assert me.ppid in {entry.pid for entry in entries}
    assert os.getpid() in descendants_of(entries, me.ppid)


def test_a_port_is_free_until_something_listens() -> None:
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = int(listener.getsockname()[1])
    assert port_is_free("127.0.0.1", port)
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", port))
        listener.listen(1)
        assert not port_is_free("127.0.0.1", port)


def spawn_with_grandchild(tmp_path: Path, child_code: str):  # type: ignore[no-untyped-def]
    pid_file = tmp_path / "grandchild.pid"
    code = (
        "import subprocess, sys, time\n"
        f"g = subprocess.Popen([sys.executable, '-c', {child_code!r}])\n"
        f"open({str(pid_file)!r}, 'w').write(str(g.pid))\n"
        "time.sleep(60)\n"
    )
    process = spawn_process(ServerCommand((sys.executable, "-c", code)))
    deadline = time.monotonic() + 10
    while not pid_file.exists() or not pid_file.read_text():
        assert time.monotonic() < deadline
        time.sleep(0.05)
    return process, int(pid_file.read_text())


def terminator(grace: float = GRACE_SECONDS) -> ProcessTreeTerminator:
    return ProcessTreeTerminator(ProcessTable(SubprocessRunner()), SystemClock(), SystemSleeper(), grace)


@pytest.mark.integration
def test_the_whole_tree_is_terminated(tmp_path: Path) -> None:
    process, grandchild = spawn_with_grandchild(tmp_path, SLEEP_CODE)
    assert terminator().terminate(process)
    assert process.poll() is not None
    assert not is_alive(grandchild)


@pytest.mark.integration
def test_a_process_that_ignores_sigterm_is_killed_after_the_grace(tmp_path: Path) -> None:
    process, grandchild = spawn_with_grandchild(tmp_path, STUBBORN_CODE)
    assert terminator(0.3).terminate(process)
    assert not is_alive(grandchild)


@pytest.mark.integration
def test_an_already_exited_process_is_reported_gone() -> None:
    process = spawn_process(ServerCommand((sys.executable, "-c", "pass")))
    process.wait(10)
    assert terminator().terminate(process)


@pytest.mark.integration
def test_spawn_lays_env_over_the_environment_and_unsets(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("CALIBRATION_GONE", "x")
    out = tmp_path / "env.txt"
    code = f"import os; open({str(out)!r}, 'w').write(os.environ.get('CALIBRATION_GONE', '-') + os.environ['CALIBRATION_NEW'])"
    process = spawn_process(
        ServerCommand((sys.executable, "-c", code), env={"CALIBRATION_NEW": "n"}, unset=("CALIBRATION_GONE",))
    )
    process.wait(10)
    assert out.read_text() == "-n"


@pytest.mark.integration
def test_a_spawned_child_stays_in_the_callers_process_group() -> None:
    process = spawn_process(ServerCommand((sys.executable, "-c", SLEEP_CODE)))
    try:
        assert os.getpgid(process.pid) == os.getpgrp()
    finally:
        process.kill()
        process.wait(10)
