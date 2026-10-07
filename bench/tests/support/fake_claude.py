"""A stand-in for the `claude` executable that answers from recorded results and records how it was called.

`install_fake_claude` writes an executable that reads `claude -p` arguments and a prompt on stdin, saves them under
`<state>/calls/<n>.json`, and prints the n-th recorded response of `<state>/responses.json` in the shape of a real
`--output-format json` result (see the pilot's canary records). Nothing in the test suite may run the real CLI.
"""

from __future__ import annotations

import json
import stat
import sys
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import TypedDict, cast

from specbench.domain.canonical import JsonValue

FAKE_SOURCE = """#!{python}
import json, os, sys
from pathlib import Path

state = Path({state!r})
calls = state / "calls"
calls.mkdir(parents=True, exist_ok=True)
index = len(list(calls.iterdir()))
responses = json.loads((state / "responses.json").read_text())
argv = sys.argv[1:]
(calls / f"{{index}}.json").write_text(json.dumps({{
    "argv": argv,
    "stdin": sys.stdin.read(),
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


def install_fake_claude(directory: Path, responses: Sequence[Mapping[str, JsonValue]]) -> Path:
    """The executable's path; `directory/state` holds the scripted responses and the recorded calls."""
    state = directory / "state"
    state.mkdir(parents=True, exist_ok=True)
    (state / "responses.json").write_text(json.dumps(list(responses)))
    executable = directory / "claude"
    executable.write_text(FAKE_SOURCE.format(python=sys.executable, state=str(state)))
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
