"""A stand-in for the `codex` executable: `exec`, `exec resume` and `exec fork` over rollout files in `CODEX_HOME`.

`install_fake_codex` writes an executable whose n-th call follows the n-th scripted response (`events` with the steps
`spawn`, `command`, `edit`, `hang`; `final`, `leave_files`, and `exit` with `stderr`) and records how it was called under
`<state>/calls/<n>.json`. The program itself is `fake_codex_cli`. Nothing in the test suite may run the real CLI.
"""

from __future__ import annotations

import json
import stat
import sys
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import TypedDict, cast

from specbench.domain.canonical import JsonValue

LAUNCHER = """#!{python}
import sys
sys.path.insert(0, {support!r})
from fake_codex_cli import main
main({state!r})
"""


class RecordedCodexCall(TypedDict):
    argv: list[str]
    mode: str
    stdin: str
    cwd: str
    codex_home: str
    overrides: dict[str, str]
    options: list[str]


def install_fake_codex(directory: Path, responses: Sequence[Mapping[str, JsonValue]]) -> Path:
    state = directory / "state"
    state.mkdir(parents=True, exist_ok=True)
    (state / "responses.json").write_text(json.dumps(list(responses)))
    executable = directory / "codex"
    executable.write_text(LAUNCHER.format(python=sys.executable, support=str(Path(__file__).parent), state=str(state)))
    executable.chmod(executable.stat().st_mode | stat.S_IXUSR)
    return executable


def recorded_codex_calls(directory: Path) -> list[RecordedCodexCall]:
    calls = sorted((directory / "state" / "calls").glob("*.json"), key=lambda p: int(p.stem))
    return [cast(RecordedCodexCall, json.loads(path.read_text())) for path in calls]
