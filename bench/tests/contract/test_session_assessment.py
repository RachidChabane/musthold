"""How a coder session is judged from the event shapes the canary and the pilot recorded from the real CLIs."""

from __future__ import annotations

import json
from pathlib import Path

from specbench.adapters.claude_session import ClaudeSessionSetup, assess_claude_session
from specbench.adapters.codex_rollouts import CodexRollouts
from specbench.adapters.codex_session import CodexBaseline, CodexRolloutFacts, assess_codex_session
from specbench.adapters.codex_stream import parse_codex_rollout
from specbench.domain.arm_policy import policy_for
from specbench.domain.arms import Arm
from specbench.domain.contamination import ScanPolicy
from specbench.domain.manifest import RunLayout
from specbench.domain.sessions import SessionOutcome

FIXTURES = Path(__file__).parent.parent / "fixtures" / "streams"
CLAUDE_WORKSPACE = "/private/var/T/project"
CODEX_WORKSPACE = "/run/project"
MAIN_THREAD = "01a11334-f5df-7a33-ba1e-3dd65b19d059"
CHILD_THREAD = "01a11334-f9a0-7c11-8e02-1b3f0c2d4e55"
PLUGINS = ("spec-proof-dp", "spec-proof-alignment", "spec-proof-completeness")
SERVERS = ("plugin:spec-proof-dp:exec", "plugin:spec-proof-alignment:exec", "plugin:spec-proof-completeness:exec")


def lines(name: str) -> list[str]:
    return (FIXTURES / name).read_text().splitlines()


def claude_setup(plugins: tuple[str, ...] = PLUGINS, servers: tuple[str, ...] = SERVERS) -> ClaudeSessionSetup:
    return ClaudeSessionSetup(RunLayout(Path("/runs/r1")), (), plugins, servers)


def test_the_recorded_plugin_host_session_is_a_clean_complete_session() -> None:
    assessment = assess_claude_session(
        lines("claude_session.jsonl"),
        policy_for(Arm.ALL_NATIVE),
        claude_setup(),
        ScanPolicy(CLAUDE_WORKSPACE),
        interrupted=False,
    )

    assert assessment.outcome is SessionOutcome.COMPLETED and assessment.problems == ()
    assert assessment.session_id == "5b0c2a64-6a3e-4f4e-9d0e-0a1f6c8d2b11" and assessment.final_message == "done"
    assert assessment.model_reported == "claude-sonnet-5-5" and assessment.executions == 2


def test_the_same_session_under_an_honest_arm_shows_plugins_that_arm_must_not_have() -> None:
    assessment = assess_claude_session(
        lines("claude_session.jsonl"), policy_for(Arm.ALL), claude_setup(), ScanPolicy(CLAUDE_WORKSPACE), False
    )
    assert assessment.outcome is SessionOutcome.HARNESS_MISCONFIGURED
    assert set(assessment.problems) == {f"unexpected plugin {name}" for name in PLUGINS}


def test_a_required_server_that_did_not_connect_is_a_misconfigured_harness() -> None:
    setup = claude_setup(servers=(*SERVERS, "plugin:spec-proof-extra:exec"))
    assessment = assess_claude_session(
        lines("claude_session.jsonl"), policy_for(Arm.CHEAT), setup, ScanPolicy(CLAUDE_WORKSPACE), False
    )
    assert assessment.outcome is SessionOutcome.HARNESS_MISCONFIGURED
    assert assessment.problems == ("missing mcp:plugin:spec-proof-extra:exec",)


def test_a_recorded_result_without_init_is_a_misconfigured_harness() -> None:
    assessment = assess_claude_session(
        lines("claude_result_pilot.jsonl"), policy_for(Arm.T), claude_setup((), ()), ScanPolicy("/w"), False
    )
    assert assessment.outcome is SessionOutcome.HARNESS_MISCONFIGURED and assessment.problems == (
        "missing system/init",
    )


def test_a_stream_cut_before_init_is_interrupted_and_after_init_keeps_its_session_id() -> None:
    cut = assess_claude_session([], policy_for(Arm.T), claude_setup((), ()), ScanPolicy("/w"), interrupted=True)
    assert cut.outcome is SessionOutcome.INTERRUPTED and cut.executions == 0

    after_init = assess_claude_session(
        lines("claude_session.jsonl")[:4], policy_for(Arm.ALL_NATIVE), claude_setup(), ScanPolicy("/w"), True
    )
    assert after_init.outcome is SessionOutcome.INTERRUPTED and after_init.session_id.startswith("5b0c2a64")
    assert after_init.final_message == "" and after_init.executions == 2


def test_a_line_that_is_not_json_in_the_middle_of_the_stream_is_a_misconfigured_harness() -> None:
    stream = ["Error: something went wrong", *lines("claude_session.jsonl")]
    assessment = assess_claude_session(stream, policy_for(Arm.ALL_NATIVE), claude_setup(), ScanPolicy("/w"), False)
    assert assessment.outcome is SessionOutcome.HARNESS_MISCONFIGURED and "line 0 is not JSON" in assessment.problems[0]


def codex_facts(main: str, children: tuple[str, ...] = ()) -> CodexRolloutFacts:
    parsed = parse_codex_rollout((FIXTURES / main).read_text())
    return CodexRolloutFacts(
        parsed, tuple(parse_codex_rollout((FIXTURES / child).read_text()) for child in children), len(parsed.spawns)
    )


def test_the_recorded_codex_session_counts_its_spawn_from_the_rollout_and_scans_the_subagent() -> None:
    facts = codex_facts("codex_rollout_main.jsonl", ("codex_rollout_child.jsonl",))

    assessment = assess_codex_session(
        lines("codex_exec.jsonl"), facts, CodexBaseline(), ScanPolicy(CODEX_WORKSPACE), interrupted=False
    )

    assert assessment.outcome is SessionOutcome.COMPLETED and assessment.problems == ()
    assert assessment.session_id == MAIN_THREAD and assessment.final_message == "done"
    assert assessment.executions == 2


def test_the_rollout_is_the_authority_when_it_holds_more_spawns_than_the_stream_shows() -> None:
    facts = codex_facts("codex_rollout_spawns.jsonl")
    assessment = assess_codex_session(lines("codex_exec.jsonl"), facts, CodexBaseline(), ScanPolicy("/w"), False)
    assert assessment.executions == 5


def test_spawns_the_thread_already_held_are_not_this_sessions() -> None:
    facts = codex_facts("codex_rollout_spawns.jsonl")
    assessment = assess_codex_session(
        lines("codex_exec.jsonl"), facts, CodexBaseline(own_spawns=3), ScanPolicy("/w"), False
    )
    assert assessment.executions == 2


def test_a_stream_with_no_thread_is_a_misconfigured_harness_unless_it_was_cut() -> None:
    nothing = CodexRolloutFacts(None, (), 0)
    assert assess_codex_session([], nothing, CodexBaseline(), ScanPolicy("/w"), False).outcome is (
        SessionOutcome.HARNESS_MISCONFIGURED
    )
    assert assess_codex_session([], nothing, CodexBaseline(), ScanPolicy("/w"), True).outcome is (
        SessionOutcome.INTERRUPTED
    )


def test_a_rollout_whose_sandbox_allows_the_network_is_a_misconfigured_harness(tmp_path: Path) -> None:
    meta = {"type": "session_meta", "payload": {"id": MAIN_THREAD, "source": "exec"}}
    turn = {
        "type": "turn_context",
        "payload": {"model": "gpt-6-luna", "sandbox_policy": {"type": "workspace-write", "network_access": True}},
    }
    rollout = parse_codex_rollout("\n".join(json.dumps(line) for line in (meta, turn)))

    assessment = assess_codex_session(
        lines("codex_exec.jsonl"), CodexRolloutFacts(rollout, (), 0), CodexBaseline(), ScanPolicy("/w"), False
    )

    assert assessment.outcome is SessionOutcome.HARNESS_MISCONFIGURED
    assert assessment.problems == ("the sandbox does not deny the network",)
    assert assessment.model_reported == "gpt-6-luna"


def test_rollouts_are_found_by_thread_id_under_the_days_of_the_sessions_directory(tmp_path: Path) -> None:
    day = tmp_path / "sessions" / "2026" / "10" / "07"
    day.mkdir(parents=True)
    (day / f"rollout-2026-10-07T21-05-12-{MAIN_THREAD}.jsonl").write_text(
        (FIXTURES / "codex_rollout_main.jsonl").read_text()
    )
    (day / f"rollout-2026-10-07T21-05-15-{CHILD_THREAD}.jsonl").write_text(
        (FIXTURES / "codex_rollout_child.jsonl").read_text()
    )
    rollouts = CodexRollouts(tmp_path)

    assert rollouts.own_spawns(MAIN_THREAD) == 1
    assert rollouts.own_spawns(CHILD_THREAD) == 0
    assert rollouts.own_spawns("00000000-0000-0000-0000-000000000000") == 0
    child = rollouts.read(CHILD_THREAD)
    assert child is not None and child.source.parent_thread_id == MAIN_THREAD
