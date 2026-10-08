"""`SubprocessGateSessionRunner` against a stand-in executable that prints stream lines: no `claude` is started."""

from __future__ import annotations

import sys
from pathlib import Path

import pytest

from specbench.adapters.claude_gate_session import SubprocessGateSessionRunner
from specbench.adapters.process import MachineUnsafe
from specbench.application.ports import GateSessionSpec

pytestmark = pytest.mark.integration

PRINTS_LINES = (
    "import sys\nfor n in range(5):\n    print(f'line-{n}', flush=True)\nsys.stderr.write('warning')\nsys.exit(3)\n"
)
SLEEPS_AFTER_FIRST_LINE = "import time\nprint('first', flush=True)\ntime.sleep(60)\n"
PLENTY_OF_MEMORY = 100


def spec_of(script: Path, tmp_path: Path, env: dict[str, str] | None = None) -> GateSessionSpec:
    return GateSessionSpec((sys.executable, str(script)), env or {}, tmp_path)


def write(tmp_path: Path, name: str, source: str) -> Path:
    path = tmp_path / name
    path.write_text(source)
    return path


def test_every_line_is_streamed_to_the_sink_and_the_exit_and_stderr_are_returned(tmp_path: Path) -> None:
    seen: list[str] = []
    runner = SubprocessGateSessionRunner(lambda: PLENTY_OF_MEMORY)
    end = runner.run(spec_of(write(tmp_path, "s.py", PRINTS_LINES), tmp_path), lambda line: seen.append(line) or True)
    assert seen == [f"line-{n}" for n in range(5)]
    assert (end.exit_code, end.stderr) == (3, "warning")


def test_a_sink_that_refuses_a_line_stops_the_session(tmp_path: Path) -> None:
    runner = SubprocessGateSessionRunner(lambda: PLENTY_OF_MEMORY)
    end = runner.run(spec_of(write(tmp_path, "s.py", SLEEPS_AFTER_FIRST_LINE), tmp_path), lambda _line: False)
    assert end.exit_code != 0


def test_the_session_gets_exactly_the_environment_it_was_given(tmp_path: Path) -> None:
    source = "import os\nprint(os.environ.get('ONLY_THIS'), os.environ.get('HOME'))\n"
    seen: list[str] = []
    runner = SubprocessGateSessionRunner(lambda: PLENTY_OF_MEMORY)
    runner.run(
        spec_of(write(tmp_path, "s.py", source), tmp_path, {"ONLY_THIS": "yes"}), lambda line: seen.append(line) or True
    )
    assert seen == ["yes None"]


def test_a_session_is_not_started_below_the_free_memory_floor(tmp_path: Path) -> None:
    runner = SubprocessGateSessionRunner(lambda: 10, minimum_percent=40)
    with pytest.raises(MachineUnsafe):
        runner.run(spec_of(write(tmp_path, "s.py", PRINTS_LINES), tmp_path), lambda _line: True)
