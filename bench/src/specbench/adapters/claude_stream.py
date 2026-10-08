"""Parser of `claude -p --output-format stream-json --verbose` output.

Reads the events a session emits: `system/init` (the plugins and MCP servers that loaded), assistant tool calls
(as `AgentAction`s, attributed to a subagent through `parent_tool_use_id`), hook events and the closing `result` with
its usage per model. Nothing here calls the CLI.
"""

from __future__ import annotations

from collections.abc import Collection, Mapping
from dataclasses import dataclass
from typing import Final

from specbench.adapters.event_lines import (
    EventLine,
    count_field,
    decode_event_lines,
    optional_cost,
    optional_mapping,
    optional_sequence,
)
from specbench.domain.agent_events import (
    ActionKind,
    AgentAction,
    EventPointer,
    HarnessCheck,
    HarnessOutcome,
    ModelUsage,
    TokenUsage,
)
from specbench.domain.jsonread import boolean, integer, mapping, optional_text, text

STREAM_NAME: Final = "claude stream"
TYPE_SYSTEM: Final = "system"
TYPE_ASSISTANT: Final = "assistant"
TYPE_RESULT: Final = "result"
SUBTYPE_INIT: Final = "init"
HOOK_SUBTYPES: Final = frozenset({"hook_started", "hook_response", "hook_progress"})
BLOCK_TOOL_USE: Final = "tool_use"
SHELL_TOOL: Final = "Bash"
FILE_TOOLS: Final = frozenset({"Read", "Edit", "Write", "MultiEdit", "NotebookEdit", "Glob", "Grep", "LS"})
PATH_INPUT_KEYS: Final = ("file_path", "notebook_path", "path")
MCP_CONNECTED: Final = "connected"
PLUGIN_PREFIX: Final = "plugin"
MCP_PREFIX: Final = "mcp"
MISSING_INIT: Final = "system/init"


@dataclass(frozen=True)
class McpServer:
    name: str
    status: str


@dataclass(frozen=True)
class ClaudeInit:
    session_id: str
    model: str | None
    plugins: tuple[str, ...]
    mcp_servers: tuple[McpServer, ...]
    tools: tuple[str, ...]


@dataclass(frozen=True)
class HookEvent:
    pointer: EventPointer
    subtype: str
    hook_event: str
    hook_name: str
    outcome: str | None
    exit_code: int | None


@dataclass(frozen=True)
class ClaudeResult:
    session_id: str
    subtype: str
    is_error: bool
    final_message: str
    num_turns: int
    duration_ms: int
    total_cost_usd: float | None
    usage: TokenUsage
    model_usage: tuple[ModelUsage, ...]
    permission_denials: int
    terminal_reason: str | None


@dataclass(frozen=True)
class ClaudeStream:
    init: ClaudeInit | None
    actions: tuple[AgentAction, ...]
    hooks: tuple[HookEvent, ...]
    subagent_messages: Mapping[str, int]
    result: ClaudeResult | None


def _names(items: list[object], where: str) -> tuple[str, ...]:
    return tuple(text(mapping(item, where).get("name"), f"{where}.name") for item in items)


def _parse_init(event: EventLine) -> ClaudeInit:
    where = f"{STREAM_NAME} init"
    body = event.body
    servers = tuple(
        McpServer(
            text(mapping(item, where).get("name"), f"{where} server name"),
            text(mapping(item, where).get("status"), f"{where} server status"),
        )
        for item in optional_sequence(body, "mcp_servers", where)
    )
    tools = tuple(text(tool, f"{where} tool") for tool in optional_sequence(body, "tools", where))
    return ClaudeInit(
        text(body.get("session_id"), f"{where} session_id"),
        optional_text(body.get("model"), f"{where} model"),
        _names(optional_sequence(body, "plugins", where), f"{where} plugin"),
        servers,
        tools,
    )


def _parse_hook(event: EventLine, subtype: str) -> HookEvent:
    body = event.body
    where = f"{STREAM_NAME} line {event.line}"
    exit_code = body.get("exit_code")
    return HookEvent(
        EventPointer(event.line),
        subtype,
        text(body.get("hook_event"), f"{where} hook_event"),
        text(body.get("hook_name"), f"{where} hook_name"),
        optional_text(body.get("outcome"), f"{where} outcome"),
        integer(exit_code, f"{where} exit_code", minimum=-255) if exit_code is not None else None,
    )


def _tool_paths(tool_input: Mapping[str, object]) -> tuple[str, ...]:
    return tuple(value for key in PATH_INPUT_KEYS if isinstance(value := tool_input.get(key), str) and value)


def _action_of(block: Mapping[str, object], event: EventLine, scope: str | None) -> AgentAction:
    where = f"{STREAM_NAME} line {event.line}"
    tool = text(block.get("name"), f"{where} tool name")
    call_id = optional_text(block.get("id"), f"{where} tool id")
    tool_input = optional_mapping(block, "input", where)
    pointer = EventPointer(event.line, call_id)
    if tool == SHELL_TOOL:
        return AgentAction(
            pointer,
            ActionKind.COMMAND,
            tool,
            command=optional_text(tool_input.get("command"), where) or "",
            scope=scope,
        )
    if tool in FILE_TOOLS:
        return AgentAction(pointer, ActionKind.FILE_ACCESS, tool, paths=_tool_paths(tool_input), scope=scope)
    return AgentAction(pointer, ActionKind.TOOL_CALL, tool, paths=_tool_paths(tool_input), scope=scope)


def _assistant_actions(event: EventLine) -> list[AgentAction]:
    where = f"{STREAM_NAME} line {event.line}"
    scope = optional_text(event.body.get("parent_tool_use_id"), f"{where} parent_tool_use_id")
    message = optional_mapping(event.body, "message", where)
    blocks = optional_sequence(message, "content", f"{where} message")
    return [
        _action_of(block, event, scope)
        for item in blocks
        if (block := mapping(item, f"{where} content block")).get("type") == BLOCK_TOOL_USE
    ]


def _token_usage(body: Mapping[str, object], keys: tuple[str, str, str, str]) -> TokenUsage:
    input_key, output_key, cache_read_key, cache_creation_key = keys
    return TokenUsage(
        count_field(body, input_key),
        count_field(body, output_key),
        count_field(body, cache_read_key),
        count_field(body, cache_creation_key),
    )


def _model_usage(body: Mapping[str, object], where: str) -> tuple[ModelUsage, ...]:
    per_model = optional_mapping(body, "modelUsage", where)
    keys = ("inputTokens", "outputTokens", "cacheReadInputTokens", "cacheCreationInputTokens")
    return tuple(
        ModelUsage(
            model,
            _token_usage(mapping(usage, f"{where} modelUsage {model}"), keys),
            optional_cost(mapping(usage, where), "costUSD"),
        )
        for model, usage in sorted(per_model.items())
    )


def _parse_result(event: EventLine) -> ClaudeResult:
    where = f"{STREAM_NAME} result"
    body = event.body
    keys = ("input_tokens", "output_tokens", "cache_read_input_tokens", "cache_creation_input_tokens")
    denials = optional_sequence(body, "permission_denials", where)
    return ClaudeResult(
        text(body.get("session_id"), f"{where} session_id"),
        text(body.get("subtype"), f"{where} subtype"),
        boolean(body.get("is_error"), f"{where} is_error"),
        optional_text(body.get("result"), f"{where} result") or "",
        count_field(body, "num_turns"),
        count_field(body, "duration_ms"),
        optional_cost(body, "total_cost_usd"),
        _token_usage(optional_mapping(body, "usage", where), keys),
        _model_usage(body, where),
        len(denials),
        optional_text(body.get("terminal_reason"), f"{where} terminal_reason"),
    )


def parse_claude_stream(stream: str) -> ClaudeStream:
    """The structured content of one session's stream. A stream without `system/init` or `result` has `None` there."""
    init: ClaudeInit | None = None
    result: ClaudeResult | None = None
    actions: list[AgentAction] = []
    hooks: list[HookEvent] = []
    subagent_messages: dict[str, int] = {}
    for event in decode_event_lines(stream, STREAM_NAME):
        kind = event.body.get("type")
        subtype = event.body.get("subtype")
        parent = event.body.get("parent_tool_use_id")
        if isinstance(parent, str):
            subagent_messages[parent] = subagent_messages.get(parent, 0) + 1
        if kind == TYPE_SYSTEM and subtype == SUBTYPE_INIT:
            init = _parse_init(event)
        elif kind == TYPE_SYSTEM and isinstance(subtype, str) and subtype in HOOK_SUBTYPES:
            hooks.append(_parse_hook(event, subtype))
        elif kind == TYPE_ASSISTANT:
            actions.extend(_assistant_actions(event))
        elif kind == TYPE_RESULT:
            result = _parse_result(event)
    return ClaudeStream(init, tuple(actions), tuple(hooks), subagent_messages, result)


def check_claude_harness(
    init: ClaudeInit | None, required_plugins: Collection[str], required_servers: Collection[str]
) -> HarnessCheck:
    """`harness_misconfigured` unless `system/init` lists every required plugin and a connected MCP server of each name."""
    if init is None:
        return HarnessCheck(HarnessOutcome.HARNESS_MISCONFIGURED, (MISSING_INIT,))
    connected = {server.name for server in init.mcp_servers if server.status == MCP_CONNECTED}
    missing = tuple(
        [f"{PLUGIN_PREFIX}:{name}" for name in sorted(required_plugins) if name not in init.plugins]
        + [f"{MCP_PREFIX}:{name}" for name in sorted(required_servers) if name not in connected]
    )
    outcome = HarnessOutcome.HARNESS_MISCONFIGURED if missing else HarnessOutcome.OK
    return HarnessCheck(outcome, missing)
