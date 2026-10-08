"""Parsers of `codex exec --json` events and of Codex rollout files.

The exec stream carries the items the agent produced and the usage of each turn. The rollout file of a thread
carries what the stream leaves out: how a thread was started (a subagent spawned by another thread), the sandbox
policy of each turn, and the arguments of every `spawn_agent` call. Nothing here calls the CLI.
"""

from __future__ import annotations

import json
import re
import shlex
from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum
from typing import Final

from specbench.adapters.event_lines import (
    EventLine,
    count_field,
    decode_event_lines,
    optional_mapping,
    optional_sequence,
)
from specbench.domain.agent_events import ActionKind, AgentAction, EventPointer, TokenUsage
from specbench.domain.jsonread import RecordError, mapping, optional_text, text

EXEC_STREAM_NAME: Final = "codex stream"
ROLLOUT_NAME: Final = "codex rollout"
EVENT_THREAD_STARTED: Final = "thread.started"
EVENT_TURN_COMPLETED: Final = "turn.completed"
EVENT_TURN_FAILED: Final = "turn.failed"
EVENT_ERROR: Final = "error"
EVENT_ITEM_STARTED: Final = "item.started"
EVENT_ITEM_COMPLETED: Final = "item.completed"
ITEM_COMMAND: Final = "command_execution"
ITEM_FILE_CHANGE: Final = "file_change"
ITEM_MCP_CALL: Final = "mcp_tool_call"
ITEM_COLLAB_CALL: Final = "collab_tool_call"
ITEM_WEB_SEARCH: Final = "web_search"
ITEM_AGENT_MESSAGE: Final = "agent_message"
TOOL_SPAWN_AGENT: Final = "spawn_agent"
ROLLOUT_SESSION_META: Final = "session_meta"
ROLLOUT_TURN_CONTEXT: Final = "turn_context"
ROLLOUT_RESPONSE_ITEM: Final = "response_item"
PAYLOAD_FUNCTION_CALL: Final = "function_call"
SOURCE_EXEC: Final = "exec"
SHELL_FUNCTIONS: Final = frozenset({"shell", "shell_command", "exec_command", "local_shell"})
APPLY_PATCH_FUNCTION: Final = "apply_patch"
PATCH_FILE_LINE: Final = re.compile(r"^\*\*\* (?:Add|Update|Delete) File: (.+)$", re.MULTILINE)


@dataclass(frozen=True)
class SubagentSpawn:
    """A `spawn_agent` call: the agent type, the fork depth the parent asked for, and the threads it created."""

    pointer: EventPointer
    agent_type: str | None
    fork_turns: str | None
    task_name: str | None
    receiver_thread_ids: tuple[str, ...]


@dataclass(frozen=True)
class CodexExec:
    thread_id: str | None
    actions: tuple[AgentAction, ...]
    spawns: tuple[SubagentSpawn, ...]
    usage: TokenUsage
    turns_completed: int
    final_message: str
    errors: tuple[str, ...]


class SourceKind(Enum):
    EXEC = "exec"
    SUBAGENT = "subagent"
    OTHER = "other"


@dataclass(frozen=True)
class RolloutSource:
    kind: SourceKind
    parent_thread_id: str | None = None
    depth: int | None = None
    agent_path: str | None = None
    agent_nickname: str | None = None


@dataclass(frozen=True)
class SandboxPolicy:
    kind: str
    network_access: bool | None
    writable_roots: tuple[str, ...]


@dataclass(frozen=True)
class TurnContext:
    model: str | None
    approval_policy: str | None
    sandbox: SandboxPolicy | None


@dataclass(frozen=True)
class RolloutFile:
    thread_id: str
    cli_version: str | None
    originator: str | None
    agent_role: str | None
    source: RolloutSource
    turns: tuple[TurnContext, ...]
    spawns: tuple[SubagentSpawn, ...]
    actions: tuple[AgentAction, ...]


def _strings(items: list[object], where: str) -> tuple[str, ...]:
    return tuple(text(item, where) for item in items)


def _item_pointer(event: EventLine, item: Mapping[str, object]) -> EventPointer:
    return EventPointer(event.line, optional_text(item.get("id"), f"{EXEC_STREAM_NAME} item id"))


def _item_actions(event: EventLine, item: Mapping[str, object]) -> list[AgentAction]:
    where = f"{EXEC_STREAM_NAME} line {event.line}"
    pointer = _item_pointer(event, item)
    match item.get("type"):
        case "command_execution":
            return [
                AgentAction(
                    pointer, ActionKind.COMMAND, ITEM_COMMAND, command=text(item.get("command"), f"{where} command")
                )
            ]
        case "file_change":
            changes = optional_sequence(item, "changes", where)
            paths = tuple(text(mapping(change, where).get("path"), f"{where} path") for change in changes)
            return [AgentAction(pointer, ActionKind.FILE_ACCESS, ITEM_FILE_CHANGE, paths=paths)]
        case "mcp_tool_call":
            server, tool = optional_text(item.get("server"), where) or "", optional_text(item.get("tool"), where) or ""
            return [AgentAction(pointer, ActionKind.TOOL_CALL, f"{server}.{tool}")]
        case "collab_tool_call":
            return [
                AgentAction(pointer, ActionKind.TOOL_CALL, optional_text(item.get("tool"), where) or ITEM_COLLAB_CALL)
            ]
        case "web_search":
            return [AgentAction(pointer, ActionKind.TOOL_CALL, ITEM_WEB_SEARCH)]
        case _:
            return []


def _spawn_of_item(event: EventLine, item: Mapping[str, object]) -> SubagentSpawn | None:
    where = f"{EXEC_STREAM_NAME} line {event.line}"
    if item.get("type") != ITEM_COLLAB_CALL or item.get("tool") != TOOL_SPAWN_AGENT:
        return None
    arguments = optional_mapping(item, "arguments", where)
    return SubagentSpawn(
        _item_pointer(event, item),
        optional_text(arguments.get("agent_type"), where),
        _fork_turns(arguments.get("fork_turns")),
        optional_text(arguments.get("task_name"), where),
        _strings(optional_sequence(item, "receiver_thread_ids", where), where),
    )


def _fork_turns(value: object) -> str | None:
    """`fork_turns` is a keyword (`none`, `all`) or a number of turns; both read as text."""
    if isinstance(value, bool) or value is None:
        return None
    return str(value) if isinstance(value, str | int) else None


def _codex_usage(body: Mapping[str, object]) -> TokenUsage:
    return TokenUsage(
        count_field(body, "input_tokens"),
        count_field(body, "output_tokens"),
        count_field(body, "cached_input_tokens"),
        count_field(body, "cache_write_input_tokens"),
        count_field(body, "reasoning_output_tokens"),
    )


def _error_message(event: EventLine) -> str:
    body = event.body
    nested = body.get("error")
    message = mapping(nested, "codex error").get("message") if nested is not None else body.get("message")
    return message if isinstance(message, str) else json.dumps(body, sort_keys=True)


def parse_codex_exec(stream: str) -> CodexExec:
    """The structured content of one `codex exec --json` run. An item seen twice (started, completed) counts once."""
    thread_id: str | None = None
    usage = TokenUsage()
    turns = 0
    final_message = ""
    actions: dict[str, AgentAction] = {}
    anonymous: list[AgentAction] = []
    spawns: list[SubagentSpawn] = []
    errors: list[str] = []
    for event in decode_event_lines(stream, EXEC_STREAM_NAME):
        kind = event.body.get("type")
        if kind == EVENT_THREAD_STARTED:
            thread_id = text(event.body.get("thread_id"), f"{EXEC_STREAM_NAME} thread_id")
        elif kind == EVENT_TURN_COMPLETED:
            turns += 1
            usage = usage + _codex_usage(optional_mapping(event.body, "usage", EXEC_STREAM_NAME))
        elif kind in {EVENT_TURN_FAILED, EVENT_ERROR}:
            errors.append(_error_message(event))
        elif kind in {EVENT_ITEM_STARTED, EVENT_ITEM_COMPLETED}:
            item = mapping(event.body.get("item"), f"{EXEC_STREAM_NAME} line {event.line} item")
            for action in _item_actions(event, item):
                if action.pointer.call_id is None:
                    anonymous.append(action)
                else:
                    actions.setdefault(action.pointer.call_id, action)
            spawn = _spawn_of_item(event, item) if kind == EVENT_ITEM_COMPLETED else None
            if spawn is not None:
                spawns.append(spawn)
            if kind == EVENT_ITEM_COMPLETED and item.get("type") == ITEM_AGENT_MESSAGE:
                final_message = text(item.get("text"), f"{EXEC_STREAM_NAME} agent_message text")
    ordered = sorted([*actions.values(), *anonymous], key=lambda action: action.pointer.line)
    return CodexExec(thread_id, tuple(ordered), tuple(spawns), usage, turns, final_message, tuple(errors))


def _parse_source(value: object, where: str) -> RolloutSource:
    if value == SOURCE_EXEC:
        return RolloutSource(SourceKind.EXEC)
    if value is None or isinstance(value, str):
        return RolloutSource(SourceKind.OTHER)
    subagent = optional_mapping(mapping(value, where), "subagent", where)
    spawn = optional_mapping(subagent, "thread_spawn", where)
    if not spawn:
        return RolloutSource(SourceKind.OTHER)
    depth = spawn.get("depth")
    return RolloutSource(
        SourceKind.SUBAGENT,
        text(spawn.get("parent_thread_id"), f"{where} parent_thread_id"),
        depth if isinstance(depth, int) and not isinstance(depth, bool) else None,
        optional_text(spawn.get("agent_path"), where),
        optional_text(spawn.get("agent_nickname"), where),
    )


def _parse_turn_context(payload: Mapping[str, object], where: str) -> TurnContext:
    raw = payload.get("sandbox_policy")
    sandbox: SandboxPolicy | None = None
    if raw is not None:
        policy = mapping(raw, f"{where} sandbox_policy")
        network = policy.get("network_access")
        sandbox = SandboxPolicy(
            text(policy.get("type"), f"{where} sandbox type"),
            network if isinstance(network, bool) else None,
            _strings(optional_sequence(policy, "writable_roots", where), where),
        )
    return TurnContext(
        optional_text(payload.get("model"), where), optional_text(payload.get("approval_policy"), where), sandbox
    )


def _function_arguments(payload: Mapping[str, object], where: str) -> Mapping[str, object]:
    raw = payload.get("arguments")
    if not isinstance(raw, str):
        return {}
    try:
        decoded: object = json.loads(raw)
    except json.JSONDecodeError as error:
        raise RecordError(f"{where} arguments are not JSON") from error
    return decoded if isinstance(decoded, Mapping) else {}  # pyright: ignore[reportUnknownVariableType]


def _shell_command(arguments: Mapping[str, object]) -> str:
    command = arguments.get("command") or arguments.get("cmd")
    if isinstance(command, list):
        return shlex.join(str(word) for word in command)  # pyright: ignore[reportUnknownVariableType, reportUnknownArgumentType]
    return command if isinstance(command, str) else ""


def _rollout_function_call(
    event: EventLine, payload: Mapping[str, object]
) -> tuple[AgentAction | None, SubagentSpawn | None]:
    where = f"{ROLLOUT_NAME} line {event.line}"
    name = text(payload.get("name"), f"{where} function name")
    pointer = EventPointer(event.line, optional_text(payload.get("call_id"), where))
    arguments = _function_arguments(payload, where)
    if name == TOOL_SPAWN_AGENT:
        spawn = SubagentSpawn(
            pointer,
            optional_text(arguments.get("agent_type"), where),
            _fork_turns(arguments.get("fork_turns")),
            optional_text(arguments.get("task_name"), where),
            (),
        )
        return AgentAction(pointer, ActionKind.TOOL_CALL, name), spawn
    if name in SHELL_FUNCTIONS:
        return AgentAction(pointer, ActionKind.COMMAND, name, command=_shell_command(arguments)), None
    if name == APPLY_PATCH_FUNCTION:
        patch = arguments.get("input") or payload.get("arguments")
        paths = tuple(PATCH_FILE_LINE.findall(patch)) if isinstance(patch, str) else ()
        return AgentAction(pointer, ActionKind.FILE_ACCESS, name, paths=paths), None
    return AgentAction(pointer, ActionKind.TOOL_CALL, name), None


def parse_codex_rollout(rollout: str) -> RolloutFile:
    """The structured content of a rollout file. It must open with a `session_meta` record."""
    meta: Mapping[str, object] | None = None
    turns: list[TurnContext] = []
    spawns: list[SubagentSpawn] = []
    actions: list[AgentAction] = []
    for event in decode_event_lines(rollout, ROLLOUT_NAME):
        kind = event.body.get("type")
        payload = optional_mapping(event.body, "payload", f"{ROLLOUT_NAME} line {event.line}")
        if kind == ROLLOUT_SESSION_META and meta is None:
            meta = payload
        elif kind == ROLLOUT_TURN_CONTEXT:
            turns.append(_parse_turn_context(payload, f"{ROLLOUT_NAME} line {event.line}"))
        elif kind == ROLLOUT_RESPONSE_ITEM and payload.get("type") == PAYLOAD_FUNCTION_CALL:
            action, spawn = _rollout_function_call(event, payload)
            actions.extend([action] if action is not None else [])
            spawns.extend([spawn] if spawn is not None else [])
    if meta is None:
        raise RecordError(f"{ROLLOUT_NAME} has no {ROLLOUT_SESSION_META} record")
    return RolloutFile(
        text(meta.get("id"), f"{ROLLOUT_NAME} id"),
        optional_text(meta.get("cli_version"), ROLLOUT_NAME),
        optional_text(meta.get("originator"), ROLLOUT_NAME),
        optional_text(meta.get("agent_role"), ROLLOUT_NAME),
        _parse_source(meta.get("source"), ROLLOUT_NAME),
        tuple(turns),
        tuple(spawns),
        tuple(actions),
    )
