"""The Claude coder-session adapter against a fake `claude`: start, resume, fork, kill, isolation. No model call."""

from __future__ import annotations

import json
from datetime import datetime
from pathlib import Path

import pytest

from specbench.adapters.claude_homes import ClaudeConfigHomes, MissingSnapshot
from specbench.adapters.claude_session import MISSING_TOKEN_MESSAGE, OAUTH_TOKEN_VARIABLE
from specbench.adapters.execution_counter import execution_counter_for
from specbench.adapters.run_store import FileCallStore, FileEventStream, FileLedgerStore
from specbench.application.paced_calls import CallGate, PacedAgentHarness
from specbench.domain.arms import Arm, Harness
from specbench.domain.call_ledger import EMPTY_LEDGER
from specbench.domain.pacing import PacingPolicy
from specbench.domain.sessions import SessionKind, SessionOutcome
from support.coder_rigs import TEST_TOKEN, TRUNK_ID, make_claude_rig
from support.fake_claude import recorded_session_calls
from support.fakes import FixedClock

PROMPT = "implement the change"
SESSION_ONE = "fake-session-0"
_NOON = datetime(2026, 10, 9, 12, 0)


def test_a_trunk_session_runs_in_the_workspace_with_its_own_home_and_the_token(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"events": [{"text": "working"}, {"edit": "pkg/a.py"}], "final": "done"}])
    request = rig.request("t1")

    result = rig.harness.run_session(request)

    assert result.outcome is SessionOutcome.COMPLETED and result.exit_code == 0
    assert result.session_id == SESSION_ONE and result.final_message == "done"
    assert result.model_reported == "claude-sonnet-5-5" and result.executions == 1
    call = recorded_session_calls(rig.fake_dir)[0]
    assert call["cwd"] == str(rig.workspace) and call["stdin"] == request.prompt
    assert call["config_dir"] == str(rig.layout.homes(TRUNK_ID) / "claude")
    assert call["token"] == TEST_TOKEN
    assert call["argv"][:2] == ["-p", "--model"] and "stream-json" in call["argv"]
    assert (rig.workspace / "pkg" / "a.py").is_file()


def test_events_stream_to_the_call_directory_as_they_arrive(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"events": [{"text": "a"}, {"agent": "q1"}]}])
    request = rig.request("t1")

    result = rig.harness.run_session(request)

    stored = (rig.layout.call(request.call_id) / "events.jsonl").read_text().splitlines()
    assert tuple(stored) == result.events and len(stored) == 4
    assert json.loads(stored[0])["subtype"] == "init" and json.loads(stored[-1])["type"] == "result"


def test_subagents_count_as_executions(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"events": [{"agent": "one"}, {"agent": "two"}]}])
    assert rig.harness.run_session(rig.request("t1")).executions == 3


def test_resume_continues_the_session_and_a_fork_gets_a_new_id(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{}, {"final": "again"}, {"final": "forked"}])
    first = rig.harness.run_session(rig.request("t1"))
    rig.harness.run_session(rig.request("t2", resume=first.session_id))
    snapshot_taken = ClaudeConfigHomes(rig.layout).snapshot(TRUNK_ID)
    forked = rig.harness.run_session(
        rig.request("b1", kind=SessionKind.BRANCH, arm=Arm.AL, resume=first.session_id, fork=True)
    )

    calls = recorded_session_calls(rig.fake_dir)
    assert calls[1]["argv"][-2:] == ["--resume", first.session_id]
    assert "--fork-session" not in calls[1]["argv"]
    assert calls[2]["argv"][-3:] == ["--resume", first.session_id, "--fork-session"]
    assert forked.session_id != first.session_id and forked.session_id.startswith(first.session_id)
    assert calls[2]["config_dir"] == str(rig.layout.homes(TRUNK_ID) / "claude-AL")
    assert snapshot_taken == rig.layout.homes(TRUNK_ID) / "claude-snapshot"


def test_a_branch_starts_from_the_s_t_snapshot_and_not_from_the_trunk_as_it_moved_on(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{}, {}, {}, {}])
    first = rig.harness.run_session(rig.request("t1"))
    ClaudeConfigHomes(rig.layout).snapshot(TRUNK_ID)
    later = rig.harness.run_session(rig.request("t2", resume=first.session_id))
    branch = rig.request("b1", kind=SessionKind.BRANCH, arm=Arm.TR, resume=first.session_id, fork=True)
    other = rig.request("b2", kind=SessionKind.BRANCH, arm=Arm.DP, resume=first.session_id, fork=True)

    rig.harness.run_session(branch)
    rig.harness.run_session(other)

    homes = rig.layout.homes(TRUNK_ID)
    trunk_turns = json.loads((homes / "claude" / "sessions" / f"{first.session_id}.json").read_text())["turns"]
    tr_turns = json.loads((homes / "claude-TR" / "sessions" / f"{first.session_id}.json").read_text())["turns"]
    assert later.session_id == first.session_id and trunk_turns == 2 and tr_turns == 1
    forks = {
        name: sorted(path.stem for path in (homes / name / "sessions").iterdir()) for name in ("claude-TR", "claude-DP")
    }
    assert forks["claude-TR"] == [first.session_id, f"{first.session_id}-fork-2"]
    assert forks["claude-DP"] == [first.session_id, f"{first.session_id}-fork-3"]


def test_a_fork_before_the_snapshot_is_refused(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{}])
    first = rig.harness.run_session(rig.request("t1"))
    with pytest.raises(MissingSnapshot):
        rig.harness.run_session(
            rig.request("b1", kind=SessionKind.BRANCH, arm=Arm.AL, resume=first.session_id, fork=True)
        )


def test_a_missing_token_ends_as_harness_misconfigured_and_touches_nothing(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{}], environ={"PATH": "/usr/bin:/bin"})

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED and result.executions == 0
    assert MISSING_TOKEN_MESSAGE in result.problems and OAUTH_TOKEN_VARIABLE in MISSING_TOKEN_MESSAGE
    assert result.exit_code != 0 and result.events == ()
    assert not (rig.fake_dir / "state" / "calls").exists() or not list((rig.fake_dir / "state" / "calls").iterdir())
    assert not rig.layout.root.exists() or not list(rig.layout.root.rglob("*"))
    assert not (rig.owner_home / ".claude").exists()


def test_the_owner_home_is_never_written_and_variables_that_reroute_claude_are_not_inherited(tmp_path: Path) -> None:
    owner_home = tmp_path / "owner"
    environ = {
        "PATH": "/usr/bin:/bin",
        "HOME": str(owner_home),
        OAUTH_TOKEN_VARIABLE: TEST_TOKEN,
        "CLAUDE_CONFIG_DIR": str(owner_home / ".claude"),
        "ANTHROPIC_API_KEY": "sk-owner",
        "CLAUDE_CODE_USE_BEDROCK": "1",
        "CLAUDECODE": "1",
    }
    rig = make_claude_rig(tmp_path, [{}], environ=environ)

    rig.harness.run_session(rig.request("t1"))

    call = recorded_session_calls(rig.fake_dir)[0]
    assert call["config_dir"] == str(rig.layout.homes(TRUNK_ID) / "claude")
    assert not {"ANTHROPIC_API_KEY", "CLAUDE_CODE_USE_BEDROCK", "CLAUDECODE"} & set(call["env_names"])
    assert list(owner_home.iterdir()) == []


def test_a_session_killed_mid_stream_leaves_its_events_and_settles_from_them(tmp_path: Path) -> None:
    rig = make_claude_rig(
        tmp_path, [{"events": [{"agent": "one"}, {"agent": "two"}, {"hang": True}]}], timeout_seconds=2
    )
    request = rig.request("t1")

    result = rig.harness.run_session(request)

    assert result.outcome is SessionOutcome.INTERRUPTED and result.exit_code < 0
    events = FileEventStream(rig.layout).lines(request.call_id)
    assert len(events) == 3 and events == result.events
    assert execution_counter_for(Harness.CLAUDE_CODE).count(events) == 3
    assert result.executions == 3 and result.session_id == SESSION_ONE


def test_an_attempt_after_a_kill_appends_to_whole_events(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"events": [{"agent": "one"}]}])
    request = rig.request("t1")
    events_file = rig.layout.call(request.call_id) / "events.jsonl"
    events_file.parent.mkdir(parents=True)
    events_file.write_text('{"type": "system", "subtype": "init"}\n{"type": "assis')

    rig.harness.run_session(request)

    lines = events_file.read_text().splitlines()
    assert all(json.loads(line) for line in lines) and len(lines) == 4


def test_paced_wrapper_does_not_archive_an_interrupted_or_misconfigured_session(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"events": [{"hang": True}]}, {"final": "ok"}], timeout_seconds=2)
    ledgers = FileLedgerStore(rig.layout)
    calls = FileCallStore(rig.layout)
    gate = CallGate(Harness.CLAUDE_CODE, PacingPolicy(None, budget_per_night=10), ledgers, FixedClock(_NOON))
    paced = PacedAgentHarness(rig.harness, gate, calls)
    request = rig.request("t1")

    interrupted = paced.run_session(request)
    assert interrupted.outcome is SessionOutcome.INTERRUPTED and not calls.exists(request.call_id)

    finished = paced.run_session(request)
    assert finished.outcome is SessionOutcome.COMPLETED and calls.exists(request.call_id)
    replay = paced.run_session(request)
    assert replay.outcome is SessionOutcome.COMPLETED and replay.session_id == finished.session_id
    assert len(recorded_session_calls(rig.fake_dir)) == 2


def test_paced_wrapper_archives_nothing_and_charges_nothing_for_a_missing_token(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{}], environ={"PATH": "/usr/bin:/bin"})
    ledgers = FileLedgerStore(rig.layout)
    calls = FileCallStore(rig.layout)
    gate = CallGate(Harness.CLAUDE_CODE, PacingPolicy(None, budget_per_night=10), ledgers, FixedClock(_NOON))
    paced = PacedAgentHarness(rig.harness, gate, calls)
    request = rig.request("t1")

    result = paced.run_session(request)

    assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED and not calls.exists(request.call_id)
    assert ledgers.load() == EMPTY_LEDGER
