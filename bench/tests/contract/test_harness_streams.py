"""The stream parsers against streams the canary and the pilot recorded from the real CLIs."""

from __future__ import annotations

from pathlib import Path

import pytest

from specbench.adapters.claude_stream import check_claude_harness, parse_claude_stream
from specbench.adapters.codex_stream import SourceKind, parse_codex_exec, parse_codex_rollout
from specbench.domain.agent_events import ActionKind, HarnessOutcome
from specbench.domain.contamination import FindingKind, ScanPolicy, scan_actions
from specbench.domain.jsonread import RecordError
from support.pinned_claude import OTHER_CLAUDE_VERSION, TEST_CLAUDE_VERSION, with_claude_version

FIXTURES = Path(__file__).parent.parent / "fixtures" / "streams"
PLUGINS = {"spec-proof-dp", "spec-proof-alignment", "spec-proof-completeness"}
SERVERS = {"plugin:spec-proof-dp:exec", "plugin:spec-proof-alignment:exec", "plugin:spec-proof-completeness:exec"}
WORKSPACE = "/private/var/T/project"
SPAWN_COUNT_IN_CANARY_ROLLOUT = 4
SUBAGENT_EVENTS_IN_CANARY_SESSION = 5


def read(name: str) -> str:
    return (FIXTURES / name).read_text()


def versioned(name: str, version: str | None = TEST_CLAUDE_VERSION) -> str:
    return "\n".join(with_claude_version(read(name).splitlines(), version))


def test_claude_init_lists_the_plugins_and_connected_servers() -> None:
    stream = parse_claude_stream(versioned("claude_session.jsonl"))
    assert stream.init is not None
    assert set(stream.init.plugins) == PLUGINS
    assert {server.name for server in stream.init.mcp_servers} == SERVERS
    assert stream.init.model == "claude-sonnet-5-5"
    assert stream.init.claude_code_version == TEST_CLAUDE_VERSION
    assert check_claude_harness(stream.init, PLUGINS, SERVERS, TEST_CLAUDE_VERSION).outcome is HarnessOutcome.OK


def test_a_missing_plugin_or_disconnected_server_is_a_misconfigured_harness() -> None:
    init = parse_claude_stream(versioned("claude_session.jsonl")).init
    check = check_claude_harness(
        init, PLUGINS | {"spec-proof-extra"}, SERVERS | {"plugin:spec-proof-extra:exec"}, TEST_CLAUDE_VERSION
    )
    assert check.outcome is HarnessOutcome.HARNESS_MISCONFIGURED
    assert check.missing == ("plugin:spec-proof-extra", "mcp:plugin:spec-proof-extra:exec")


def test_a_stream_without_init_is_a_misconfigured_harness() -> None:
    stream = parse_claude_stream(read("claude_result_pilot.jsonl"))
    assert stream.init is None
    check = check_claude_harness(stream.init, PLUGINS, SERVERS, TEST_CLAUDE_VERSION)
    assert check.outcome is HarnessOutcome.HARNESS_MISCONFIGURED


def test_an_init_that_names_another_version_than_the_pin_is_a_misconfigured_harness() -> None:
    init = parse_claude_stream(versioned("claude_session.jsonl", OTHER_CLAUDE_VERSION)).init
    check = check_claude_harness(init, PLUGINS, SERVERS, TEST_CLAUDE_VERSION)
    assert check.outcome is HarnessOutcome.HARNESS_MISCONFIGURED and check.missing == ()
    assert (
        check.version_fault == f"the session ran Claude Code {OTHER_CLAUDE_VERSION}, the pin is {TEST_CLAUDE_VERSION}"
    )


def test_an_init_without_a_version_is_a_misconfigured_harness() -> None:
    recorded = parse_claude_stream(read("claude_session.jsonl")).init
    assert recorded is not None and recorded.claude_code_version is None
    check = check_claude_harness(recorded, PLUGINS, SERVERS, TEST_CLAUDE_VERSION)
    assert check.outcome is HarnessOutcome.HARNESS_MISCONFIGURED and check.version_fault is not None


def test_claude_result_reports_usage_per_model() -> None:
    result = parse_claude_stream(read("claude_session.jsonl")).result
    assert result is not None
    assert not result.is_error and result.final_message == "done" and result.num_turns == 6
    assert [usage.model for usage in result.model_usage] == ["claude-sonnet-5-5"]
    assert result.model_usage[0].usage.output_tokens == 640 and result.model_usage[0].cost_usd == pytest.approx(0.0831)


def test_a_pilot_result_carries_the_cache_counters_of_its_model() -> None:
    result = parse_claude_stream(read("claude_result_pilot.jsonl")).result
    assert result is not None
    usage = result.model_usage[0]
    assert usage.model == "claude-opus-5-5"
    assert usage.usage.cache_creation_tokens == 3646 and usage.usage.cache_read_tokens == 717
    assert result.usage.output_tokens == 2426 and result.terminal_reason == "completed"


def test_subagent_messages_are_attributed_by_parent_tool_use_id() -> None:
    stream = parse_claude_stream(read("claude_session.jsonl"))
    assert stream.subagent_messages == {"toolu_01Agent": SUBAGENT_EVENTS_IN_CANARY_SESSION}
    scoped = [action for action in stream.actions if action.scope == "toolu_01Agent"]
    assert [action.tool for action in scoped] == [
        "mcp__plugin_spec-proof-dp_exec__fetch_request",
        "mcp__plugin_spec-proof-dp_exec__submit_response",
    ]
    assert all(action.scope is None for action in stream.actions if action not in scoped)


def test_claude_tool_calls_become_actions_with_a_pointer() -> None:
    stream = parse_claude_stream(read("claude_session.jsonl"))
    edit = next(action for action in stream.actions if action.tool == "Edit")
    assert edit.kind is ActionKind.FILE_ACCESS and edit.paths == ("/private/var/T/project/parcel/rates.py",)
    assert edit.pointer.call_id == "toolu_01Edit"


def test_claude_hook_events_are_listed_in_order() -> None:
    hooks = parse_claude_stream(read("claude_session.jsonl")).hooks
    assert [hook.hook_event for hook in hooks] == ["SubagentStart", "SubagentStop", "Stop"]
    assert all(hook.outcome == "success" and hook.exit_code == 0 for hook in hooks)


def test_the_recorded_claude_session_is_clean_inside_its_workspace() -> None:
    stream = parse_claude_stream(read("claude_session.jsonl"))
    assert scan_actions(stream.actions, ScanPolicy(WORKSPACE)) == ()


def test_the_same_session_scanned_from_another_root_reports_the_edit_as_outside() -> None:
    stream = parse_claude_stream(read("claude_session.jsonl"))
    findings = scan_actions(stream.actions, ScanPolicy("/elsewhere"))
    assert [f.kind for f in findings] == [FindingKind.OUTSIDE_PATH]
    assert findings[0].pointer.call_id == "toolu_01Edit"


def test_a_line_that_is_not_json_is_named() -> None:
    with pytest.raises(RecordError, match="line 1 is not JSON"):
        parse_claude_stream('{"type":"system"}\nnot json\n')


def test_codex_exec_events_give_actions_usage_and_spawns() -> None:
    run = parse_codex_exec(read("codex_exec.jsonl"))
    assert run.thread_id == "01a11334-f5df-7a33-ba1e-3dd65b19d059"
    assert run.final_message == "done" and run.turns_completed == 1 and not run.errors
    assert run.usage.input_tokens == 21840 and run.usage.cache_read_tokens == 9216 and run.usage.reasoning_tokens == 120
    assert [spawn.agent_type for spawn in run.spawns] == ["spec-proof-dp-decider"]
    assert run.spawns[0].receiver_thread_ids == ("01a11334-f9a0-7c11-8e02-1b3f0c2d4e55",)
    command = next(action for action in run.actions if action.kind is ActionKind.COMMAND)
    assert command.command == "/bin/zsh -lc 'touch sandbox-probe.txt'"
    assert [action.tool for action in run.actions if action.kind is ActionKind.TOOL_CALL] == [
        "spawn_agent",
        "spec-proof-dp-exec.fetch_request",
        "spec-proof-dp-exec.submit_response",
    ]


def test_the_recorded_codex_run_is_clean_inside_its_workspace() -> None:
    run = parse_codex_exec(read("codex_exec.jsonl"))
    assert scan_actions(run.actions, ScanPolicy("/run/project")) == ()


def test_the_main_rollout_names_its_spawns_and_their_fork_depth() -> None:
    rollout = parse_codex_rollout(read("codex_rollout_spawns.jsonl"))
    assert rollout.source.kind is SourceKind.EXEC and rollout.originator == "codex_exec"
    assert [(s.agent_type, s.fork_turns) for s in rollout.spawns] == [
        ("spec-proof-dp-decider", "none"),
        ("spec-proof-dp-decider", None),
        ("spec-proof-dp-decider", "all"),
        ("spec-proof-dp-judge", "3"),
    ]
    assert len(rollout.spawns) == SPAWN_COUNT_IN_CANARY_ROLLOUT


def test_a_child_rollout_names_its_parent_and_its_sandbox() -> None:
    rollout = parse_codex_rollout(read("codex_rollout_child.jsonl"))
    assert rollout.source.kind is SourceKind.SUBAGENT
    assert rollout.source.parent_thread_id == "01a11334-f5df-7a33-ba1e-3dd65b19d059"
    assert rollout.source.depth == 1 and rollout.source.agent_path == "/root/decide_ticket"
    assert rollout.agent_role == "spec-proof-dp-decider"
    sandbox = rollout.turns[0].sandbox
    assert sandbox is not None and sandbox.kind == "read-only" and sandbox.network_access is False
    assert rollout.turns[0].model == "gpt-5.5"
    shell = rollout.actions[0]
    assert shell.kind is ActionKind.COMMAND and shell.command == "touch sandbox-probe.txt"


def test_a_rollout_without_session_meta_is_refused() -> None:
    with pytest.raises(RecordError, match="session_meta"):
        parse_codex_rollout(read("codex_exec.jsonl"))
