"""A Claude coder session runs the pinned binary with the updater off in its own environment, records the version its
stream reports, and is a fault when that version is not the pin."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

from specbench.adapters.claude_credentials import OAUTH_TOKEN_VARIABLE
from specbench.adapters.run_store import FileCallStore, FileLedgerStore
from specbench.application.paced_calls import CallGate, PacedAgentHarness
from specbench.application.ports import PinnedClaudeUnavailable
from specbench.domain.arms import Harness
from specbench.domain.pacing import PacingPolicy
from specbench.domain.sessions import SessionOutcome
from support.coder_rigs import TEST_TOKEN, make_claude_rig
from support.fake_claude import recorded_session_calls
from support.fakes import FixedClock
from support.pinned_claude import OTHER_CLAUDE_VERSION, TEST_CLAUDE_VERSION

NOON = datetime(2026, 10, 9, 12, 0)
NOT_STARTED_EXIT_CODE = 78
INSTALL_COMMAND = "script/claude-code-install"


@dataclass(frozen=True)
class UninstalledClaude:
    version: str = TEST_CLAUDE_VERSION

    def executable(self) -> str:
        raise PinnedClaudeUnavailable(f"the pinned Claude Code is not installed: run {INSTALL_COMMAND}")


def test_a_session_records_the_version_its_stream_reports(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{}])

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.COMPLETED and result.cli_version == TEST_CLAUDE_VERSION


def test_the_session_environment_alone_switches_the_updater_off(tmp_path: Path) -> None:
    owner_environment = {"PATH": "/usr/bin:/bin", "HOME": str(tmp_path / "owner"), OAUTH_TOKEN_VARIABLE: TEST_TOKEN}
    rig = make_claude_rig(tmp_path, [{}], environ=owner_environment)

    rig.harness.run_session(rig.request("t1"))

    [call] = recorded_session_calls(rig.fake_dir)
    assert call["env"]["DISABLE_AUTOUPDATER"] == "1"
    assert "DISABLE_AUTOUPDATER" not in owner_environment


def test_a_session_that_ran_another_version_than_the_pin_is_a_fault_and_keeps_that_version(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"claude_code_version": OTHER_CLAUDE_VERSION}])

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED
    assert result.cli_version == OTHER_CLAUDE_VERSION
    assert f"the session ran Claude Code {OTHER_CLAUDE_VERSION}, the pin is {TEST_CLAUDE_VERSION}" in result.problems


def test_a_stream_whose_init_names_no_version_is_a_fault(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"claude_code_version": None}])

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED and result.cli_version is None
    assert f"the stream reports no Claude Code version, the pin is {TEST_CLAUDE_VERSION}" in result.problems


def test_the_pinned_version_decides_not_the_version_the_fake_happens_to_report(tmp_path: Path) -> None:
    rig = make_claude_rig(
        tmp_path, [{"claude_code_version": OTHER_CLAUDE_VERSION}], claude_version=OTHER_CLAUDE_VERSION
    )

    assert rig.harness.run_session(rig.request("t1")).outcome is SessionOutcome.COMPLETED


def test_a_pinned_binary_that_is_not_installed_starts_nothing_and_names_the_install_script(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{}], pinned=UninstalledClaude())

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED and result.executions == 0
    assert result.exit_code == NOT_STARTED_EXIT_CODE and INSTALL_COMMAND in result.problems[0]
    assert recorded_session_calls(rig.fake_dir) == []


def test_the_version_survives_the_archive_and_a_replay(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{}])
    calls = FileCallStore(rig.layout)
    gate = CallGate(
        Harness.CLAUDE_CODE, PacingPolicy(None, budget_per_night=10), FileLedgerStore(rig.layout), FixedClock(NOON)
    )
    paced = PacedAgentHarness(rig.harness, gate, calls)
    request = rig.request("t1")

    live = paced.run_session(request)
    replay = paced.run_session(request)

    assert live.cli_version == TEST_CLAUDE_VERSION and replay.cli_version == TEST_CLAUDE_VERSION
    assert calls.read(request.call_id).cli_version == TEST_CLAUDE_VERSION
    assert len(recorded_session_calls(rig.fake_dir)) == 1


def test_a_faulty_session_is_not_archived_so_the_run_stops_and_resumes_once_the_binary_is_right(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"claude_code_version": OTHER_CLAUDE_VERSION}, {}])
    calls = FileCallStore(rig.layout)
    gate = CallGate(
        Harness.CLAUDE_CODE, PacingPolicy(None, budget_per_night=10), FileLedgerStore(rig.layout), FixedClock(NOON)
    )
    paced = PacedAgentHarness(rig.harness, gate, calls)
    request = rig.request("t1")

    faulty = paced.run_session(request)
    assert faulty.outcome is SessionOutcome.HARNESS_MISCONFIGURED and not calls.exists(request.call_id)

    fixed = paced.run_session(request)
    assert fixed.outcome is SessionOutcome.COMPLETED and calls.exists(request.call_id)
