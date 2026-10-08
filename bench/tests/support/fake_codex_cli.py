"""The program behind the fake `codex exec`, `codex exec resume` and `codex exec fork`.

It follows the real CLI where the adapter depends on it:

- `resume` and `fork` accept neither `-s` nor `-C` and refuse them with a usage error;
- sessions live as rollout files under `$CODEX_HOME/sessions/<y>/<m>/<d>/rollout-<time>-<thread>.jsonl` (`CODEX_HOME`,
  else `$HOME/.codex`); a resume appends to the thread's rollout, a fork copies it into a new thread, and a subagent
  has a rollout of its own whose `session_meta` names its parent;
- `-c projects."<dir>".trust_level="trusted"` makes the CLI persist a `[projects."<dir>"]` table in `config.toml`;
- `--json` prints `thread.started`, the items of the turn and `turn.completed`.
"""

from __future__ import annotations

import json
import os
import sys
import time
from pathlib import Path
from typing import Any

HANG_SECONDS = 3600
DAY_DIRECTORY = ("2026", "10", "08")
VALUE_OPTIONS = frozenset({"-m", "-c", "-s", "-C", "--enable", "--disable", "-i", "-o", "-p"})
SANDBOX_OPTIONS = frozenset({"-s", "-C"})
PROJECTS_PREFIX = "projects."
TRUST_SUFFIX = ".trust_level"


def _home() -> Path:
    configured = os.environ.get("CODEX_HOME")
    return Path(configured) if configured else Path(os.environ["HOME"]) / ".codex"


def _emit(event: dict[str, Any]) -> None:
    print(json.dumps(event), flush=True)


def _parse(argv: list[str]) -> tuple[str, dict[str, list[str]], list[str]]:
    rest = argv[1:]
    mode = "exec"
    if rest and rest[0] in {"resume", "fork"}:
        mode, rest = rest[0], rest[1:]
    options: dict[str, list[str]] = {}
    positionals: list[str] = []
    position = 0
    while position < len(rest):
        word = rest[position]
        if word in VALUE_OPTIONS:
            options.setdefault(word, []).append(rest[position + 1])
            position += 2
        elif word.startswith("--"):
            options.setdefault(word, [])
            position += 1
        else:
            positionals.append(word)
            position += 1
    return mode, options, positionals


def _overrides(options: dict[str, list[str]]) -> dict[str, str]:
    return dict(item.split("=", 1) for item in options.get("-c", []))


def _persist_trust(home: Path, overrides: dict[str, str]) -> None:
    for key in overrides:
        if key.startswith(PROJECTS_PREFIX) and key.endswith(TRUST_SUFFIX):
            project = json.loads(key[len(PROJECTS_PREFIX) : -len(TRUST_SUFFIX)])
            config = home / "config.toml"
            existing = config.read_bytes().decode() if config.is_file() else ""
            separator = "\n" if existing else ""
            table = f'{separator}[projects."{project}"]\ntrust_level = "trusted"\n'
            home.mkdir(parents=True, exist_ok=True)
            config.write_bytes((existing + table).encode())


def _next_thread(state: Path) -> str:
    counter = state / "threads"
    number = int(counter.read_text()) + 1 if counter.is_file() else 1
    counter.write_text(str(number))
    return f"{number:08d}-0000-7000-8000-{number:012d}"


def _rollout_path(home: Path, thread: str) -> Path:
    directory = home / "sessions" / Path(*DAY_DIRECTORY)
    directory.mkdir(parents=True, exist_ok=True)
    return directory / f"rollout-2026-10-08T21-00-00-{thread}.jsonl"


def _find_rollout(home: Path, thread: str) -> Path | None:
    found = sorted((home / "sessions").glob(f"*/*/*/rollout-*-{thread}.jsonl"))
    return found[0] if found else None


def _append(path: Path, kind: str, payload: dict[str, Any]) -> None:
    with path.open("a") as handle:
        handle.write(json.dumps({"timestamp": "2026-10-08T21:00:00.000Z", "type": kind, "payload": payload}) + "\n")


def _meta(thread: str, cwd: str, source: Any) -> dict[str, Any]:
    return {
        "id": thread,
        "timestamp": "2026-10-08T21:00:00.000Z",
        "cwd": cwd,
        "originator": "codex_exec",
        "cli_version": "0.155.1",
        "source": source,
        "agent_role": None,
    }


def _function_call(name: str, arguments: dict[str, Any], call_id: str) -> dict[str, Any]:
    return {"type": "function_call", "name": name, "arguments": json.dumps(arguments), "call_id": call_id}


def _fork_rollout(parent: Path, destination: Path, thread: str) -> None:
    lines: list[str] = []
    for raw in parent.read_text().splitlines():
        event: dict[str, Any] = json.loads(raw)
        if event["type"] == "session_meta":
            event["payload"] = {**event["payload"], "id": thread, "forked_from_id": event["payload"]["id"]}
        lines.append(json.dumps(event))
    destination.write_text("".join(f"{line}\n" for line in lines))


def _spawn_child(state: Path, home: Path, parent: str, cwd: str, step: dict[str, Any]) -> str:
    child = _next_thread(state)
    path = _rollout_path(home, child)
    source = {"subagent": {"thread_spawn": {"parent_thread_id": parent, "depth": 1, "agent_path": "/root/child"}}}
    _append(path, "session_meta", _meta(child, cwd, source))
    for number in range(step.get("nested", 0)):
        _append(path, "response_item", _function_call("spawn_agent", {"agent_type": "nested"}, f"nested_{number}"))
    for command in step.get("child_commands", []):
        _append(path, "response_item", _function_call("shell_command", {"command": command}, "child_call"))
    return child


def _run_steps(
    state: Path, home: Path, rollout: Path, thread: str, cwd: str, steps: list[dict[str, Any]], final: str
) -> None:
    for position, step in enumerate(steps):
        item_id = f"item_{position}"
        if "spawn" in step:
            child = _spawn_child(state, home, thread, cwd, step)
            _append(rollout, "response_item", _function_call("spawn_agent", {"agent_type": step["spawn"]}, item_id))
            item = {
                "id": item_id,
                "type": "collab_tool_call",
                "tool": "spawn_agent",
                "receiver_thread_ids": [child],
                "arguments": {"agent_type": step["spawn"], "fork_turns": "none"},
            }
        elif "command" in step:
            _append(rollout, "response_item", _function_call("shell_command", {"command": step["command"]}, item_id))
            item = {"id": item_id, "type": "command_execution", "command": step["command"], "exit_code": 0}
        elif "edit" in step:
            target = Path(cwd) / step["edit"]
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(step.get("content", "edited"))
            item = {"id": item_id, "type": "file_change", "changes": [{"path": str(target), "kind": "update"}]}
        else:
            item = {}
        if item:
            _emit({"type": "item.completed", "item": item})
        if step.get("hang"):
            time.sleep(HANG_SECONDS)
    _emit({"type": "item.completed", "item": {"id": "item_final", "type": "agent_message", "text": final}})


def main(state_directory: str) -> None:
    state = Path(state_directory)
    calls = state / "calls"
    calls.mkdir(parents=True, exist_ok=True)
    index = len(list(calls.iterdir()))
    mode, options, positionals = _parse(sys.argv[1:])
    overrides = _overrides(options)
    home = _home()
    cwd = os.path.realpath(os.getcwd())
    prompt = sys.stdin.read() if positionals and positionals[-1] == "-" else (positionals[-1] if positionals else "")
    (calls / f"{index}.json").write_text(
        json.dumps(
            {
                "argv": sys.argv[1:],
                "mode": mode,
                "stdin": prompt,
                "cwd": cwd,
                "codex_home": str(home),
                "home": os.environ.get("HOME"),
                "env": dict(os.environ),
                "overrides": overrides,
                "options": sorted(options),
            }
        )
    )
    if mode != "exec" and SANDBOX_OPTIONS & set(options):
        offending = sorted(SANDBOX_OPTIONS & set(options))[0]
        sys.stderr.write(f"error: unexpected argument '{offending}' found")
        sys.exit(2)
    responses: list[dict[str, Any]] = json.loads((state / "responses.json").read_text())
    response = responses[min(index, len(responses) - 1)]
    if response.get("exit"):
        sys.stderr.write(response.get("stderr", "failure"))
        sys.exit(response["exit"])
    _persist_trust(home, overrides)
    thread, rollout = _open_thread(state, home, mode, positionals, cwd, overrides)
    _emit({"type": "thread.started", "thread_id": thread})
    _emit({"type": "turn.started"})
    network = overrides.get("sandbox_workspace_write.network_access") != "false"
    sandbox = options.get("-s", [overrides.get("sandbox_mode", '"read-only"').strip('"')])[0]
    _append(
        rollout,
        "turn_context",
        {
            "model": (options.get("-m") or [""])[0],
            "approval_policy": "never",
            "sandbox_policy": {"type": sandbox, "network_access": network, "writable_roots": [cwd]},
        },
    )
    _run_steps(state, home, rollout, thread, cwd, response.get("events", []), response.get("final", f"done {thread}"))
    for name, content in response.get("leave_files", {}).items():
        Path(cwd, name).parent.mkdir(parents=True, exist_ok=True)
        Path(cwd, name).write_text(content)
    if "error" in response:
        _emit({"type": "error", "message": response["error"]})
        _emit({"type": "turn.failed", "error": {"message": response["error"]}})
        sys.exit(response.get("exit_after", 1))
    _emit({"type": "turn.completed", "usage": {"input_tokens": 10, "cached_input_tokens": 0, "output_tokens": 5}})


def _open_thread(
    state: Path, home: Path, mode: str, positionals: list[str], cwd: str, overrides: dict[str, str]
) -> tuple[str, Path]:
    if mode == "exec":
        thread = _next_thread(state)
        rollout = _rollout_path(home, thread)
        _append(rollout, "session_meta", _meta(thread, cwd, "exec"))
        return thread, rollout
    parent = _find_rollout(home, positionals[0])
    if parent is None:
        sys.stderr.write(f"No saved session found with ID {positionals[0]}")
        sys.exit(1)
    if mode == "resume":
        return positionals[0], parent
    thread = _next_thread(state)
    rollout = _rollout_path(home, thread)
    _fork_rollout(parent, rollout, thread)
    return thread, rollout
