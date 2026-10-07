from __future__ import annotations

import os
import sys
import time
from pathlib import Path

import pytest

from calibration.adapters.process_tree import is_alive
from calibration.adapters.subprocess_runner import SubprocessRunner
from calibration.domain.ports import ProcessTimeout

PYTHON = sys.executable
TIMEOUT_SECONDS = 0.5


def run_code(code: str, **kwargs: object):  # type: ignore[no-untyped-def]
    return SubprocessRunner().run([PYTHON, "-c", code], **kwargs)  # type: ignore[arg-type]


def test_output_and_status_are_returned_as_bytes() -> None:
    result = run_code("import sys; sys.stdout.write('out'); sys.stderr.write('err'); sys.exit(3)")
    assert (result.returncode, result.stdout, result.stderr) == (3, b"out", b"err")


def test_env_is_laid_over_the_inherited_environment(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("CALIBRATION_INHERITED", "kept")
    code = "import os; print(os.environ['CALIBRATION_INHERITED'], os.environ['CALIBRATION_ADDED'])"
    result = run_code(code, env={"CALIBRATION_ADDED": "set"})
    assert result.stdout.split() == [b"kept", b"set"]


def test_unset_removes_an_inherited_variable(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("CALIBRATION_REMOVED", "x")
    result = run_code("import os; print('CALIBRATION_REMOVED' in os.environ)", unset=["CALIBRATION_REMOVED"])
    assert result.stdout.strip() == b"False"


def test_stdin_bytes_reach_the_command_and_none_means_end_of_input() -> None:
    echoed = run_code("import sys; sys.stdout.buffer.write(sys.stdin.buffer.read())", input_bytes=b"\x00\xffabc")
    empty = run_code("import sys; print(len(sys.stdin.buffer.read()))")
    assert echoed.stdout == b"\x00\xffabc"
    assert empty.stdout.strip() == b"0"


def test_the_working_directory_is_applied(tmp_path: Path) -> None:
    result = run_code("import os; print(os.getcwd())", cwd=tmp_path)
    assert Path(result.stdout.decode().strip()).resolve() == tmp_path.resolve()


def test_a_timeout_kills_the_command_and_its_children(tmp_path: Path) -> None:
    pid_file = tmp_path / "child.pid"
    code = (
        "import subprocess, sys, time\n"
        f"child = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(60)'])\n"
        f"open({str(pid_file)!r}, 'w').write(str(child.pid))\n"
        "time.sleep(60)\n"
    )
    started = time.monotonic()
    with pytest.raises(ProcessTimeout):
        run_code(code, timeout_seconds=TIMEOUT_SECONDS * 2)
    assert time.monotonic() - started < 30
    child_pid = int(pid_file.read_text())
    deadline = time.monotonic() + 5
    while is_alive(child_pid) and time.monotonic() < deadline:
        time.sleep(0.05)
    assert not is_alive(child_pid)


def test_a_missing_executable_raises_os_error() -> None:
    with pytest.raises(OSError):
        SubprocessRunner().run([os.path.join("no", "such", "binary")])
