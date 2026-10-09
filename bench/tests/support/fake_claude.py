"""Stand-ins for the `claude` executable that answer from scripted responses and record how they were called.

`install_fake_claude` writes an executable that reads `claude -p` arguments and a prompt on stdin, saves them under
`<state>/calls/<n>.json`, and prints the n-th recorded response of `<state>/responses.json` in the shape of a real
`--output-format json` result (see the pilot's canary records).

`install_fake_claude_sessions` writes the stream-json counterpart for coder sessions: it keeps sessions in the
configuration home, resumes and forks them by id, and streams `system/init`, tool uses and a `result` event.

Nothing in the test suite may run the real CLI.
"""

from __future__ import annotations

import json
import stat
import sys
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import TypedDict, cast

from specbench.domain.canonical import JsonValue
from specbench.domain.construction import BASELINE_PROMPT

FAKE_BASELINE_INPUT_TOKENS = 1100
"""What an empty isolated call reported when measured on the real CLI: input 2 plus cache creation 1098."""

FAKE_SOURCE = """#!{python}
import json, os, sys
from pathlib import Path

state = Path({state!r})
argv = sys.argv[1:]
stdin = sys.stdin.read()
is_baseline = stdin == {baseline_prompt!r}
calls = state / ("baseline_calls" if is_baseline else "calls")
calls.mkdir(parents=True, exist_ok=True)
index = len(list(calls.iterdir()))
if is_baseline:
    settings = state / "baseline.json"
    responses = [json.loads(settings.read_text()) if settings.is_file() else {{}}]
    responses[0].setdefault("payload", {{"ok": True}})
    responses[0].setdefault("input_tokens", {baseline_tokens})
else:
    responses = json.loads((state / "responses.json").read_text())
(calls / f"{{index}}.json").write_text(json.dumps({{
    "argv": argv,
    "stdin": stdin,
    "cwd_entries": sorted(os.listdir(".")),
    "env_has_nesting": any(k in os.environ for k in ("CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT")),
}}))
response = responses[min(index, len(responses) - 1)]
if response.get("exit"):
    sys.stderr.write(response.get("stderr", "failure"))
    sys.exit(response["exit"])
if response.get("leave_file"):
    Path(response["leave_file"]).write_text("left behind")
model = argv[argv.index("--model") + 1]
usage = {{"input_tokens": response.get("input_tokens", 400), "cache_creation_input_tokens": 0,
          "cache_read_input_tokens": 0, "output_tokens": 50}}
result = {{
    "type": "result", "subtype": "success", "is_error": response.get("is_error", False),
    "num_turns": 1, "result": "ok", "usage": usage,
    "modelUsage": {{name: {{"inputTokens": 1}} for name in response.get("models", [model])}},
}}
if "payload" in response:
    result["structured_output"] = response["payload"]
print(json.dumps(result))
"""


def install_fake_claude(
    directory: Path,
    responses: Sequence[Mapping[str, JsonValue]],
    baseline: Mapping[str, JsonValue] | None = None,
) -> Path:
    """The executable's path; `directory/state` holds the scripted responses and the recorded calls.

    The baseline call (the trivial call that measures an empty isolated call) is answered apart from `responses`, by
    default with `FAKE_BASELINE_INPUT_TOKENS` input tokens, and is recorded apart from the other calls; `baseline`
    scripts it like a response.
    """
    state = directory / "state"
    state.mkdir(parents=True, exist_ok=True)
    (state / "responses.json").write_text(json.dumps(list(responses)))
    if baseline is not None:
        (state / "baseline.json").write_text(json.dumps(dict(baseline)))
    executable = directory / "claude"
    executable.write_text(
        FAKE_SOURCE.format(
            python=sys.executable,
            state=str(state),
            baseline_prompt=BASELINE_PROMPT,
            baseline_tokens=FAKE_BASELINE_INPUT_TOKENS,
        )
    )
    executable.chmod(executable.stat().st_mode | stat.S_IXUSR)
    return executable


class RecordedCall(TypedDict):
    argv: list[str]
    stdin: str
    cwd_entries: list[str]
    env_has_nesting: bool


def recorded_calls(directory: Path) -> list[RecordedCall]:
    calls = sorted((directory / "state" / "calls").glob("*.json"), key=lambda p: int(p.stem))
    return [cast(RecordedCall, json.loads(path.read_text())) for path in calls]


def recorded_baseline_calls(directory: Path) -> list[RecordedCall]:
    calls = sorted((directory / "state" / "baseline_calls").glob("*.json"), key=lambda p: int(p.stem))
    return [cast(RecordedCall, json.loads(path.read_text())) for path in calls]


SESSION_LAUNCHER = """#!{python}
import sys
sys.path.insert(0, {support!r})
from fake_claude_cli import main
main({state!r})
"""


class RecordedSessionCall(TypedDict):
    argv: list[str]
    stdin: str
    cwd: str
    config_dir: str
    token: str | None
    home: str | None
    env: dict[str, str]
    env_names: list[str]
    project_settings_loaded: bool
    settings: dict[str, JsonValue] | None


def install_fake_claude_sessions(directory: Path, responses: Sequence[Mapping[str, JsonValue]]) -> Path:
    """The stream-json executable's path. `responses` script the n-th call: `events` (steps `agent`, `bash`, `edit`,
    `web`, `text`, `hang`), `plugins`, `omit_init`, `final`, `models`, `leave_files`, `write_project_settings`,
    and `exit` with `stderr`."""
    state = directory / "state"
    state.mkdir(parents=True, exist_ok=True)
    (state / "responses.json").write_text(json.dumps(list(responses)))
    executable = directory / "claude"
    executable.write_text(
        SESSION_LAUNCHER.format(python=sys.executable, support=str(Path(__file__).parent), state=str(state))
    )
    executable.chmod(executable.stat().st_mode | stat.S_IXUSR)
    return executable


def recorded_session_calls(directory: Path) -> list[RecordedSessionCall]:
    calls = sorted((directory / "state" / "calls").glob("*.json"), key=lambda p: int(p.stem))
    return [cast(RecordedSessionCall, json.loads(path.read_text())) for path in calls]
