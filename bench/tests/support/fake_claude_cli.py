"""The program behind the fake `claude -p --output-format stream-json`: sessions, resume, fork, and a recorded call.

It behaves like the real CLI where the adapter's isolation depends on it: a session lives in the configuration home
(`CLAUDE_CONFIG_DIR`, else `$HOME/.claude`) and in the working directory it started in, `--resume` finds a session by
id there and refuses anything else, `--fork-session` starts a new id, and `--settings` must name a file that exists.
"""

from __future__ import annotations

import json
import os
import sys
import time
from pathlib import Path
from typing import Any

HANG_SECONDS = 3600
NO_CONVERSATION = "No conversation found with session ID: "


def _emit(event: dict[str, Any]) -> None:
    print(json.dumps(event), flush=True)


def _option(argv: list[str], name: str) -> str | None:
    return argv[argv.index(name) + 1] if name in argv else None


def _home() -> Path:
    configured = os.environ.get("CLAUDE_CONFIG_DIR")
    return Path(configured) if configured else Path(os.environ["HOME"]) / ".claude"


def _resolve_session(argv: list[str], home: Path, cwd: str, index: int) -> tuple[str, dict[str, Any]]:
    sessions = home / "sessions"
    resumed = _option(argv, "--resume")
    if resumed is None:
        return f"fake-session-{index}", {"cwd": cwd, "turns": 0, "forked_from": None}
    stored = sessions / f"{resumed}.json"
    if not stored.is_file():
        sys.stderr.write(NO_CONVERSATION + resumed)
        sys.exit(1)
    session: dict[str, Any] = json.loads(stored.read_text())
    if session["cwd"] != cwd:
        sys.stderr.write(NO_CONVERSATION + resumed)
        sys.exit(1)
    if "--fork-session" in argv:
        return f"{resumed}-fork-{index}", {**session, "forked_from": resumed}
    return resumed, session


def _step_event(step: dict[str, Any], position: int, cwd: str) -> dict[str, Any] | None:
    block: dict[str, Any]
    if "agent" in step:
        block = {"type": "tool_use", "id": f"toolu_{position}", "name": "Agent", "input": {"prompt": step["agent"]}}
    elif "bash" in step:
        block = {"type": "tool_use", "id": f"toolu_{position}", "name": "Bash", "input": {"command": step["bash"]}}
    elif "edit" in step:
        target = Path(cwd) / step["edit"]
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(step.get("content", "edited"))
        block = {"type": "tool_use", "id": f"toolu_{position}", "name": "Edit", "input": {"file_path": str(target)}}
    elif "web" in step:
        block = {"type": "tool_use", "id": f"toolu_{position}", "name": step["web"], "input": {"url": "x"}}
    elif "text" in step:
        block = {"type": "text", "text": step["text"]}
    else:
        return None
    return {"type": "assistant", "message": {"role": "assistant", "content": [block]}, "parent_tool_use_id": None}


def main(state_directory: str) -> None:
    state = Path(state_directory)
    calls = state / "calls"
    calls.mkdir(parents=True, exist_ok=True)
    index = len(list(calls.iterdir()))
    argv = sys.argv[1:]
    cwd = os.path.realpath(os.getcwd())
    home = _home()
    sources = (_option(argv, "--setting-sources") or "").split(",")
    settings = _option(argv, "--settings")
    (calls / f"{index}.json").write_text(
        json.dumps(
            {
                "argv": argv,
                "stdin": sys.stdin.read(),
                "cwd": cwd,
                "config_dir": str(home),
                "token": os.environ.get("CLAUDE_CODE_OAUTH_TOKEN"),
                "env_names": sorted(os.environ),
                "project_settings_loaded": "project" in sources and (Path(cwd) / ".claude" / "settings.json").is_file(),
                "settings": json.loads(Path(settings).read_text()) if settings and Path(settings).is_file() else None,
            }
        )
    )
    if settings is None or not Path(settings).is_file():
        sys.stderr.write("settings file not found")
        sys.exit(1)
    responses: list[dict[str, Any]] = json.loads((state / "responses.json").read_text())
    response = responses[min(index, len(responses) - 1)]
    if response.get("exit"):
        sys.stderr.write(response.get("stderr", "failure"))
        sys.exit(response["exit"])
    session_id, session = _resolve_session(argv, home, cwd, index)
    model = _option(argv, "--model") or ""
    if not response.get("omit_init"):
        _emit(
            {
                "type": "system",
                "subtype": "init",
                "cwd": cwd,
                "session_id": session_id,
                "tools": ["Agent", "Edit", "Bash"],
                "mcp_servers": [],
                "model": model,
                "plugins": [{"name": name, "path": f"/plugins/{name}"} for name in response.get("plugins", [])],
            }
        )
    for position, step in enumerate(response.get("events", [])):
        event = _step_event(step, position, cwd)
        if event is not None:
            _emit(event)
        if step.get("hang"):
            time.sleep(HANG_SECONDS)
    for name, content in response.get("leave_files", {}).items():
        Path(cwd, name).parent.mkdir(parents=True, exist_ok=True)
        Path(cwd, name).write_text(content)
    if response.get("write_project_settings"):
        (Path(cwd) / ".claude").mkdir(exist_ok=True)
        (Path(cwd) / ".claude" / "settings.json").write_text(json.dumps({"hooks": {}}))
    session["turns"] += 1
    (home / "sessions").mkdir(parents=True, exist_ok=True)
    (home / "sessions" / f"{session_id}.json").write_text(json.dumps(session))
    usage = {"input_tokens": 10, "output_tokens": 5, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}
    _emit(
        {
            "type": "result",
            "subtype": "success",
            "is_error": False,
            "result": response.get("final", f"done {session_id}"),
            "session_id": session_id,
            "num_turns": session["turns"],
            "duration_ms": 10,
            "usage": usage,
            "modelUsage": {name: {"inputTokens": 10, "outputTokens": 5} for name in response.get("models", [model])},
            "permission_denials": [],
        }
    )
