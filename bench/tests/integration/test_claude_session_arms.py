"""What the arm policy changes in a Claude coder session, and what the session's events turn into."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from specbench.domain.arms import PLUGIN_HOST_ARMS, Arm
from specbench.domain.sessions import SessionOutcome
from support.coder_rigs import make_claude_rig
from support.fake_claude import recorded_session_calls

PLUGIN_NAMES = ("spec-proof-dp", "spec-proof-alignment")


def sources_of(argv: list[str]) -> str:
    return argv[argv.index("--setting-sources") + 1]


def test_honest_arms_read_the_user_source_alone_and_load_no_plugin(tmp_path: Path) -> None:
    plugin_dir = tmp_path / "plugins" / "dp"
    rig = make_claude_rig(tmp_path, [{}], plugin_dirs=(plugin_dir,), required_plugins=PLUGIN_NAMES)

    result = rig.harness.run_session(rig.request("t1", arm=Arm.ALL))

    argv = recorded_session_calls(rig.fake_dir)[0]["argv"]
    assert result.outcome is SessionOutcome.COMPLETED
    assert sources_of(argv) == "user" and "--plugin-dir" not in argv


@pytest.mark.parametrize("arm", [arm for arm in Arm if arm in PLUGIN_HOST_ARMS])
def test_plugin_host_arms_read_user_and_project_and_load_the_product_plugins(tmp_path: Path, arm: Arm) -> None:
    plugin_dir = tmp_path / "plugins" / "dp"
    rig = make_claude_rig(
        tmp_path, [{"plugins": list(PLUGIN_NAMES)}], plugin_dirs=(plugin_dir,), required_plugins=PLUGIN_NAMES
    )

    result = rig.harness.run_session(rig.request("t1", arm=arm))

    argv = recorded_session_calls(rig.fake_dir)[0]["argv"]
    assert result.outcome is SessionOutcome.COMPLETED
    assert sources_of(argv) == "user,project"
    assert argv[argv.index("--plugin-dir") + 1] == str(plugin_dir)


def test_a_plugin_host_session_without_a_required_plugin_is_misconfigured(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"plugins": ["spec-proof-dp"]}], required_plugins=PLUGIN_NAMES)

    result = rig.harness.run_session(rig.request("t1", arm=Arm.ALL_NATIVE))

    assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED
    assert "missing plugin:spec-proof-alignment" in result.problems


def test_an_honest_session_that_shows_a_plugin_is_misconfigured(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"plugins": ["spec-proof-dp"]}])

    result = rig.harness.run_session(rig.request("t1", arm=Arm.T))

    assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED
    assert "unexpected plugin spec-proof-dp" in result.problems


def test_a_session_whose_stream_has_no_init_is_misconfigured(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"omit_init": True}])

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED and "missing system/init" in result.problems


def test_a_session_that_fails_before_streaming_is_misconfigured_and_keeps_its_stderr(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"exit": 1, "stderr": "Invalid API key"}])

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED and result.exit_code == 1
    assert result.stderr == "Invalid API key" and result.events == ()
    assert any("Invalid API key" in problem for problem in result.problems)


def test_the_settings_file_lives_in_the_run_and_denies_the_network_and_the_web_tools(tmp_path: Path) -> None:
    environment_root = tmp_path / "work" / "envs"
    rig = make_claude_rig(tmp_path, [{}], toolchain_roots=(environment_root,))

    rig.harness.run_session(rig.request("t1", arm=Arm.TR))

    settings_file = rig.layout.settings_file(Arm.TR)
    argv = recorded_session_calls(rig.fake_dir)[0]["argv"]
    assert argv[argv.index("--settings") + 1] == str(settings_file)
    assert rig.workspace not in settings_file.parents
    settings = json.loads(settings_file.read_text())
    assert settings["permissions"]["deny"] == ["WebFetch", "WebSearch"]
    assert settings["sandbox"]["network"]["allowedDomains"] == []
    assert settings["sandbox"]["filesystem"]["allowWrite"] == [str(environment_root)]
    assert recorded_session_calls(rig.fake_dir)[0]["settings"] == settings


def test_project_settings_an_agent_writes_do_not_load_on_resume_in_an_honest_arm(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"write_project_settings": True}, {}, {"write_project_settings": True}, {}])
    first = rig.harness.run_session(rig.request("t1", arm=Arm.T))
    rig.harness.run_session(rig.request("t2", arm=Arm.T, resume=first.session_id))
    native = rig.harness.run_session(rig.request("n1", arm=Arm.ALL_NATIVE))
    rig.harness.run_session(rig.request("n2", arm=Arm.ALL_NATIVE, resume=native.session_id))

    loaded = [call["project_settings_loaded"] for call in recorded_session_calls(rig.fake_dir)]
    assert loaded == [False, False, True, True]  # the file exists from the first call on; only user,project reads it


def test_a_session_of_its_own_arm_keeps_a_home_of_its_own(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{}, {}])
    rig.harness.run_session(rig.request("t1", arm=Arm.T))
    rig.harness.run_session(rig.request("n1", arm=Arm.ALL_NATIVE))

    homes = [call["config_dir"] for call in recorded_session_calls(rig.fake_dir)]
    assert homes[0].endswith("/homes/task-a/claude") and homes[1].endswith("/homes/task-a/claude-ALL-native")


@pytest.mark.parametrize(
    ("step", "evidence"),
    [
        ({"web": "WebFetch"}, "network-tool"),
        ({"bash": "curl https://example.org/x"}, "network-command"),
        ({"bash": "cat /etc/passwd"}, "outside-path"),
        ({"edit": "../escape.py"}, "outside-path"),
    ],
)
def test_a_contaminated_session_is_flagged_with_the_event(tmp_path: Path, step: dict[str, str], evidence: str) -> None:
    rig = make_claude_rig(tmp_path, [{"events": [step]}])

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.CONTAMINATED
    assert any(problem.startswith(f"{evidence} at event 1") for problem in result.problems)


def test_a_command_in_a_toolchain_root_is_not_an_outside_path(tmp_path: Path) -> None:
    environment_root = tmp_path / "work" / "envs"
    command = f"{environment_root}/task-a/bin/pytest -q"
    rig = make_claude_rig(tmp_path, [{"events": [{"bash": command}]}], toolchain_roots=(environment_root,))

    assert rig.harness.run_session(rig.request("t1")).outcome is SessionOutcome.COMPLETED


def test_a_session_that_is_both_cut_short_and_contaminated_runs_again(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{"events": [{"web": "WebSearch"}, {"hang": True}]}], timeout_seconds=2)

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.INTERRUPTED
    assert any(problem.startswith("network-tool") for problem in result.problems)


def test_a_missing_executable_is_a_misconfigured_harness(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{}], executable_name=str(tmp_path / "nope"))

    result = rig.harness.run_session(rig.request("t1"))

    assert result.outcome is SessionOutcome.HARNESS_MISCONFIGURED and "executable was not found" in result.problems[0]


def test_a_request_without_a_trunk_or_an_arm_is_a_programming_error(tmp_path: Path) -> None:
    rig = make_claude_rig(tmp_path, [{}])
    with pytest.raises(ValueError, match="names its trunk and its arm"):
        rig.harness.run_session(rig.request("t1", arm=None))
