"""The command lines of coder sessions, per arm, and the pure pieces behind them. Nothing is launched."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from specbench.adapters.claude_homes import ClaudeConfigHomes, HomeInsideWorkspace
from specbench.adapters.claude_session import claude_argv, claude_environment, claude_settings
from specbench.adapters.codex_session import codex_argv, codex_home_of
from specbench.adapters.run_store import FileCallStore
from specbench.domain.arm_policy import policy_for
from specbench.domain.arms import FROZEN_MODEL, PLUGIN_HOST_ARMS, Arm, Harness
from specbench.domain.codex_config import names_project, remove_project_table, trust_override
from specbench.domain.manifest import RunLayout
from specbench.domain.sessions import CallRecord, SessionKind, SessionOutcome, SessionRequest
from support.pinned_claude import UNUSED_EXECUTABLE

WORKSPACE = Path("/runs/r1/work/ws")
SETTINGS = Path("/runs/r1/settings/T.json")
PLUGINS = (Path("/repo/umbrella/plugin/claude"),)
SESSION = "5b0c2a64-6a3e-4f4e-9d0e-0a1f6c8d2b11"
PROJECT = "/private/var/folders/x/T/ws"


def request(harness: Harness, arm: Arm, resume: str | None = None, fork: bool = False) -> SessionRequest:
    kind = SessionKind.BRANCH if fork else SessionKind.TRUNK
    return SessionRequest(harness, FROZEN_MODEL[harness], kind, "k", WORKSPACE, "p", resume, fork, "task-a", arm)


def value_after(argv: list[str], flag: str) -> str:
    return argv[argv.index(flag) + 1]


def test_a_fresh_claude_session_has_the_frozen_model_stream_json_and_the_bench_settings() -> None:
    argv = claude_argv(UNUSED_EXECUTABLE, request(Harness.CLAUDE_CODE, Arm.T), policy_for(Arm.T), SETTINGS, PLUGINS)

    assert argv[:3] == [UNUSED_EXECUTABLE, "-p", "--model"] and value_after(argv, "--model") == "claude-sonnet-5-5"
    assert value_after(argv, "--output-format") == "stream-json"
    assert {"--verbose", "--include-hook-events"} <= set(argv)
    assert value_after(argv, "--settings") == str(SETTINGS)
    assert "--resume" not in argv and "--fork-session" not in argv and "--plugin-dir" not in argv


@pytest.mark.parametrize("arm", [arm for arm in Arm if arm not in PLUGIN_HOST_ARMS])
def test_honest_arms_read_user_settings_only(arm: Arm) -> None:
    argv = claude_argv(UNUSED_EXECUTABLE, request(Harness.CLAUDE_CODE, arm), policy_for(arm), SETTINGS, PLUGINS)
    assert value_after(argv, "--setting-sources") == "user"
    assert "--plugin-dir" not in argv


@pytest.mark.parametrize("arm", [arm for arm in Arm if arm in PLUGIN_HOST_ARMS])
def test_plugin_host_arms_read_user_and_project_settings_and_load_the_plugins(arm: Arm) -> None:
    argv = claude_argv(UNUSED_EXECUTABLE, request(Harness.CLAUDE_CODE, arm), policy_for(arm), SETTINGS, PLUGINS)
    assert value_after(argv, "--setting-sources") == "user,project"
    assert value_after(argv, "--plugin-dir") == str(PLUGINS[0])


def test_resume_and_fork_add_the_session_and_only_a_fork_adds_fork_session() -> None:
    resumed = claude_argv(
        UNUSED_EXECUTABLE, request(Harness.CLAUDE_CODE, Arm.T, SESSION), policy_for(Arm.T), SETTINGS, ()
    )
    forked = claude_argv(
        UNUSED_EXECUTABLE, request(Harness.CLAUDE_CODE, Arm.AL, SESSION, True), policy_for(Arm.AL), SETTINGS, ()
    )

    assert resumed[-2:] == ["--resume", SESSION] and "--fork-session" not in resumed
    assert forked[-3:] == ["--resume", SESSION, "--fork-session"]


def test_the_prompt_is_never_on_the_command_line() -> None:
    sent = SessionRequest(
        Harness.CLAUDE_CODE, "claude-sonnet-5-5", SessionKind.TRUNK, "k", WORKSPACE, "--rm -rf", None, False, "t", Arm.T
    )
    assert "--rm -rf" not in claude_argv(UNUSED_EXECUTABLE, sent, policy_for(Arm.T), SETTINGS, ())
    assert "--rm -rf" not in codex_argv(
        SessionRequest(
            Harness.CODEX, "gpt-6-luna", SessionKind.TRUNK, "k", WORKSPACE, "--rm -rf", None, False, "t", Arm.T
        ),
        policy_for(Arm.T),
        WORKSPACE,
    )


def test_the_environment_points_claude_at_the_bench_home_and_drops_what_would_reroute_it() -> None:
    environment = claude_environment(
        {
            "PATH": "/bin",
            "HOME": "/Users/owner",
            "CLAUDE_CONFIG_DIR": "/Users/owner/.claude",
            "ANTHROPIC_API_KEY": "sk",
            "CLAUDE_CODE_OAUTH_TOKEN": "old",
            "CLAUDECODE": "1",
            "CLAUDE_CODE_ENTRYPOINT": "cli",
        },
        Path("/runs/r1/homes/t/claude"),
        "fresh",
    )
    assert environment == {
        "PATH": "/bin",
        "HOME": "/Users/owner",
        "CLAUDE_CONFIG_DIR": "/runs/r1/homes/t/claude",
        "CLAUDE_CODE_OAUTH_TOKEN": "fresh",
        "DISABLE_AUTOUPDATER": "1",
    }


def test_the_settings_deny_web_tools_and_every_domain_and_allow_only_the_toolchain_roots() -> None:
    settings = claude_settings([Path("/work/envs")])
    assert json.loads(json.dumps(settings)) == {
        "permissions": {"defaultMode": "acceptEdits", "deny": ["WebFetch", "WebSearch"]},
        "sandbox": {
            "enabled": True,
            "autoAllowBashIfSandboxed": True,
            "allowUnsandboxedCommands": False,
            "network": {"allowedDomains": []},
            "filesystem": {"allowWrite": ["/work/envs"]},
        },
    }


def test_a_fresh_codex_session_sets_sandbox_cwd_network_search_and_trust() -> None:
    argv = codex_argv(request(Harness.CODEX, Arm.T), policy_for(Arm.T), WORKSPACE)

    assert argv[:2] == ["codex", "exec"] and value_after(argv, "-m") == "gpt-6-luna" and "--json" in argv
    assert [argv[i + 1] for i, word in enumerate(argv) if word == "--enable"] == ["hooks", "multi_agent"]
    assert value_after(argv, "-s") == "workspace-write" and value_after(argv, "-C") == str(WORKSPACE)
    overrides = [argv[i + 1] for i, word in enumerate(argv) if word == "-c"]
    assert overrides == [
        "web_search=disabled",
        "sandbox_workspace_write.network_access=false",
        "include_apps_instructions=false",
        f'projects."{WORKSPACE}".trust_level="trusted"',
    ]
    assert argv[-1] == "-" and "--ignore-user-config" not in argv


def test_codex_resume_and_fork_pass_the_sandbox_as_config_and_the_session_before_the_prompt() -> None:
    resumed = codex_argv(request(Harness.CODEX, Arm.T, SESSION), policy_for(Arm.T), WORKSPACE)
    forked = codex_argv(request(Harness.CODEX, Arm.AL, SESSION, True), policy_for(Arm.AL), WORKSPACE)

    assert resumed[:3] == ["codex", "exec", "resume"] and forked[:3] == ["codex", "exec", "fork"]
    for argv in (resumed, forked):
        assert "-s" not in argv and "-C" not in argv
        assert 'sandbox_mode="workspace-write"' in argv
        assert argv[-2:] == [SESSION, "-"]


@pytest.mark.parametrize("arm", list(Arm))
def test_only_plugin_host_arms_bypass_codex_hook_trust(arm: Arm) -> None:
    argv = codex_argv(request(Harness.CODEX, arm), policy_for(arm), WORKSPACE)
    assert ("--dangerously-bypass-hook-trust" in argv) is (arm in PLUGIN_HOST_ARMS)


def test_the_codex_home_is_the_owners_own() -> None:
    assert codex_home_of({"CODEX_HOME": "/x/codex", "HOME": "/h"}) == Path("/x/codex")
    assert codex_home_of({"HOME": "/h"}) == Path("/h/.codex")


def test_trust_override_quotes_the_path_as_a_toml_key() -> None:
    assert trust_override('/a "b"/ws') == 'projects."/a \\"b\\"/ws".trust_level="trusted"'


def test_a_home_is_refused_inside_the_workspace(tmp_path: Path) -> None:
    layout = RunLayout(tmp_path / "run")
    homes = ClaudeConfigHomes(layout)
    inside = SessionRequest(
        Harness.CLAUDE_CODE, "m", SessionKind.TRUNK, "k", tmp_path / "run", "p", None, False, "task-a", Arm.T
    )
    with pytest.raises(HomeInsideWorkspace):
        homes.home_for(inside, tmp_path / "run")


@pytest.mark.parametrize("trunk_id", ["", ".", "..", "a/b", "../x"])
def test_a_trunk_id_cannot_name_a_path(tmp_path: Path, trunk_id: str) -> None:
    with pytest.raises(ValueError, match="not a trunk id"):
        ClaudeConfigHomes(RunLayout(tmp_path)).trunk(trunk_id)


OWNER = 'model = "gpt-6-luna"\n\n[projects."/home/u/work"]\ntrust_level = "trusted"\n'


def test_the_table_codex_appends_is_removed_and_the_file_is_what_it_was() -> None:
    appended = OWNER + f'\n[projects."{PROJECT}"]\ntrust_level = "trusted"\n'
    removal = remove_project_table(appended, PROJECT)
    assert removal.found and removal.cleaned == OWNER
    assert removal.table == f'[projects."{PROJECT}"]\ntrust_level = "trusted"\n'


def test_a_table_in_the_middle_takes_its_own_keys_and_no_other_table() -> None:
    content = f'[tui]\ntheme = "dark"\n\n[projects."{PROJECT}"]\ntrust_level = "trusted"\n\n[projects."/o"]\nk = 1\n'
    removal = remove_project_table(content, PROJECT)
    assert removal.cleaned == '[tui]\ntheme = "dark"\n\n[projects."/o"]\nk = 1\n'


def test_a_file_that_holds_only_the_table_becomes_empty() -> None:
    assert remove_project_table(f'[projects."{PROJECT}"]\ntrust_level = "trusted"\n', PROJECT).cleaned == ""


def test_a_literal_key_and_a_trailing_comment_name_the_same_table() -> None:
    content = f"a = 1\n[projects.'{PROJECT}'] # trusted for the run\ntrust_level = \"trusted\"\n"
    removal = remove_project_table(content, PROJECT)
    assert removal.found and removal.cleaned == "a = 1\n"


def test_another_projects_table_a_sub_table_and_a_longer_path_are_left_alone() -> None:
    content = (
        f'[projects."{PROJECT}-2"]\ntrust_level = "trusted"\n[projects."{PROJECT}".extra]\nk = 1\n[projects."/other"]\n'
    )
    removal = remove_project_table(content, PROJECT)
    assert not removal.found and removal.cleaned == content


@pytest.mark.parametrize("content", ["", "a = 1", "a = 1\r\n[b]\r\nc = 2\r\n"])
def test_a_file_without_the_table_comes_back_byte_for_byte(content: str) -> None:
    removal = remove_project_table(content, PROJECT)
    assert not removal.found and removal.cleaned == content


def test_a_file_names_the_project_wherever_the_path_appears() -> None:
    assert names_project(f'k = "{PROJECT}"\n', PROJECT) and not names_project("k = 1\n", PROJECT)


@pytest.mark.parametrize("arm", list(Arm))
def test_setting_sources_follow_the_host_of_the_arm(arm: Arm) -> None:
    policy = policy_for(arm)
    assert policy.loads_product_plugins is (arm in PLUGIN_HOST_ARMS)
    assert policy.setting_sources == (("user", "project") if arm in PLUGIN_HOST_ARMS else ("user",))


def test_the_run_layout_keeps_settings_and_homes_in_the_run(tmp_path: Path) -> None:
    layout = RunLayout(tmp_path)
    assert layout.settings_file(Arm.ALL_NATIVE) == tmp_path / "settings" / "ALL-native.json"
    assert layout.homes("task-a") == tmp_path / "homes" / "task-a"


def test_the_outcome_and_executions_of_a_session_survive_the_call_store(tmp_path: Path) -> None:
    store = FileCallStore(RunLayout(tmp_path))
    store.write(CallRecord("call-1", ("a",), "in", "out", ("{}",), 0, "s", "m", SessionOutcome.CONTAMINATED, 7))
    recorded = store.read("call-1")
    assert recorded.outcome is SessionOutcome.CONTAMINATED and recorded.executions == 7
    meta = tmp_path / "calls" / "call-1" / "meta.json"
    legacy = json.loads(meta.read_text())
    del legacy["outcome"], legacy["executions"]
    meta.write_text(json.dumps(legacy))
    assert store.read("call-1").outcome is SessionOutcome.COMPLETED and store.read("call-1").executions == 1


@pytest.mark.parametrize("arm", list(Arm))
@pytest.mark.parametrize(("resume", "fork"), [(None, False), (SESSION, False), (SESSION, True)])
def test_every_codex_session_disables_the_apps_feature_and_its_instructions(
    arm: Arm, resume: str | None, fork: bool
) -> None:
    argv = codex_argv(request(Harness.CODEX, arm, resume, fork), policy_for(arm), WORKSPACE)
    assert [argv[i + 1] for i, word in enumerate(argv) if word == "--disable"] == ["apps"]
    assert "include_apps_instructions=false" in argv
