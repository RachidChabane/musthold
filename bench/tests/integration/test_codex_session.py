"""The Codex coder-session adapter against a fake `codex`: exec, resume, fork, kill, trust cleanup. No model call."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from specbench.adapters.codex_trust import CodexConfigNotRestored
from specbench.adapters.execution_counter import execution_counter_for
from specbench.adapters.run_store import FileEventStream
from specbench.domain.arms import Arm, Harness
from specbench.domain.sessions import SessionKind, SessionOutcome
from support.coder_rigs import KILL_AFTER_SECONDS, OWNER_CONFIG, make_codex_rig
from support.fake_codex import recorded_codex_calls

SPAWN = {"spawn": "spec-proof-dp-decider"}


def test_a_fresh_session_runs_in_the_workspace_with_the_sandbox_flags(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{"events": [{"edit": "pkg/a.py"}], "final": "done"}])
    request = rig.request("t1")

    result = rig.harness.run_session(request)

    assert result.outcome is SessionOutcome.COMPLETED and result.final_message == "done"
    assert result.model_reported == "gpt-6-luna" and result.executions == 1
    call = recorded_codex_calls(rig.fake_dir)[0]
    assert call["mode"] == "exec" and call["cwd"] == str(rig.workspace) and call["stdin"] == request.prompt
    assert call["argv"][:6] == ["exec", "-m", "gpt-6-luna", "--json", "--enable", "hooks"]
    assert call["codex_home"] == str(rig.codex_home)
    assert call["overrides"]["web_search"] == "disabled"
    assert call["overrides"]["sandbox_workspace_write.network_access"] == "false"
    assert "-s" in call["options"] and "-C" in call["options"]
    assert "--ignore-user-config" not in call["options"] and "--dangerously-bypass-hook-trust" not in call["options"]


def test_the_trust_table_codex_persisted_is_removed_and_the_owner_config_is_what_it_was(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{}, {}])
    first = rig.harness.run_session(rig.request("t1"))
    rig.harness.run_session(rig.request("t2", resume=first.session_id))

    assert rig.config.read_bytes().decode() == OWNER_CONFIG
    assert str(rig.workspace) not in rig.config.read_text()
    assert json.loads((rig.codex_home / "auth.json").read_text()) == {"token": "owner-login"}


def test_the_trust_table_is_removed_even_when_the_session_is_killed(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{"events": [{"hang": True}]}], timeout_seconds=KILL_AFTER_SECONDS)

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.INTERRUPTED
    assert rig.config.read_bytes().decode() == OWNER_CONFIG


def test_a_config_that_still_names_the_workspace_after_the_cleanup_is_reported(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{}])
    rig.config.write_text(OWNER_CONFIG + f'note = "{rig.workspace}"\n')

    with pytest.raises(CodexConfigNotRestored):
        rig.harness.run_session(rig.request("t1"))


def test_resume_and_fork_carry_the_flags_that_exec_takes_as_options_and_never_s_or_c(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{}, {}, {}])
    first = rig.harness.run_session(rig.request("t1"))
    resumed = rig.harness.run_session(rig.request("t2", resume=first.session_id))
    forked = rig.harness.run_session(
        rig.request("b1", kind=SessionKind.BRANCH, arm=Arm.AL, resume=first.session_id, fork=True)
    )

    calls = recorded_codex_calls(rig.fake_dir)
    assert [call["mode"] for call in calls] == ["exec", "resume", "fork"]
    for call in calls[1:]:
        assert not {"-s", "-C"} & set(call["options"])
        assert call["overrides"]["sandbox_mode"] == '"workspace-write"'
        assert call["overrides"]["sandbox_workspace_write.network_access"] == "false"
        assert call["overrides"]["web_search"] == "disabled"
        assert call["cwd"] == str(rig.workspace)
        assert call["argv"][-2:] == [first.session_id, "-"]
    assert resumed.session_id == first.session_id
    assert forked.session_id != first.session_id


def test_spawns_are_counted_from_the_rollouts_and_only_this_sessions_own(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{"events": [SPAWN, SPAWN]}, {"events": [SPAWN]}, {"events": [SPAWN, SPAWN]}])
    first = rig.harness.run_session(rig.request("t1"))
    resumed = rig.harness.run_session(rig.request("t2", resume=first.session_id))
    forked = rig.harness.run_session(
        rig.request("b1", kind=SessionKind.BRANCH, arm=Arm.TR, resume=first.session_id, fork=True)
    )

    assert first.executions == 3
    assert resumed.executions == 2
    assert forked.executions == 3


def test_a_subagent_that_spawns_is_counted_through_its_own_rollout(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{"events": [{"spawn": "decider", "nested": 2}]}])

    assert rig.harness.run_session(rig.request("t1")).executions == 4


def test_a_killed_session_settles_from_its_events(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{"events": [SPAWN, SPAWN, {"hang": True}]}], timeout_seconds=KILL_AFTER_SECONDS)
    request = rig.request("t1")

    result = rig.harness.run_session(request)

    events = FileEventStream(rig.layout).lines(request.call_id)
    assert result.outcome is SessionOutcome.INTERRUPTED and events == result.events
    assert execution_counter_for(Harness.CODEX).count(events) == 3
    assert result.executions == 3


def test_a_fork_does_not_touch_the_parent_rollout(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{"events": [SPAWN]}, {"events": [SPAWN]}])
    first = rig.harness.run_session(rig.request("t1"))
    parent = next((rig.codex_home / "sessions").rglob(f"*{first.session_id}.jsonl"))
    before = parent.read_bytes()

    forked = rig.harness.run_session(
        rig.request("b1", kind=SessionKind.BRANCH, arm=Arm.DP, resume=first.session_id, fork=True)
    )

    assert parent.read_bytes() == before
    assert any(forked.session_id in path.name for path in (rig.codex_home / "sessions").rglob("*.jsonl"))


def test_plugin_host_arms_bypass_hook_trust_and_honest_arms_do_not(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{}, {}])
    rig.harness.run_session(rig.request("t1", arm=Arm.TR))
    rig.harness.run_session(rig.request("n1", arm=Arm.ALL_NATIVE))

    options = [call["options"] for call in recorded_codex_calls(rig.fake_dir)]
    assert "--dangerously-bypass-hook-trust" not in options[0]
    assert "--dangerously-bypass-hook-trust" in options[1]


def test_a_command_outside_the_workspace_in_a_subagent_contaminates_the_session(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{"events": [{"spawn": "decider", "child_commands": ["curl https://example.org"]}]}])

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.CONTAMINATED
    assert any(problem.startswith("network-command") for problem in result.problems)


def test_a_session_that_fails_before_a_thread_starts_is_misconfigured(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{"exit": 1, "stderr": "401 Unauthorized"}])

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED and result.exit_code == 1
    assert any("401 Unauthorized" in problem for problem in result.problems)
    assert rig.config.read_bytes().decode() == OWNER_CONFIG


def test_a_request_without_a_trunk_or_an_arm_is_a_programming_error(tmp_path: Path) -> None:
    rig = make_codex_rig(tmp_path, [{}])
    with pytest.raises(ValueError, match="names its trunk and its arm"):
        rig.harness.run_session(rig.request("t1", arm=None))
