from __future__ import annotations

from pathlib import Path

from specbench.adapters.claude_credentials import OAUTH_TOKEN_VARIABLE
from specbench.adapters.claude_gate_session import (
    CONFIG_DIRECTORY_ENV,
    plugin_directories,
    round_prompt,
    session_argv,
    session_environment,
    session_prompt,
)
from specbench.domain.arms import FROZEN_MODEL, Harness, Module
from specbench.domain.product import PROOF_MODULES, proof_modules_in_order

RECORD = Path("/runs/r1/rec")
EXECUTABLE = "claude"


def flag_values(argv: tuple[str, ...], flag: str) -> list[str]:
    return [argv[i + 1] for i, word in enumerate(argv) if word == flag]


def test_the_session_runs_the_frozen_model_and_streams_hook_events() -> None:
    argv = session_argv(session_prompt(RECORD), (Path("/u"),), EXECUTABLE)
    assert argv[:2] == ("claude", "-p")
    assert flag_values(argv, "--model") == [FROZEN_MODEL[Harness.CLAUDE_CODE]]
    assert flag_values(argv, "--output-format") == ["stream-json"]
    assert {"--verbose", "--include-hook-events"} <= set(argv)
    assert flag_values(argv, "--setting-sources") == ["user"]


def test_each_plugin_of_the_arm_is_loaded_after_the_umbrella_that_holds_prove_all() -> None:
    modules = proof_modules_in_order(frozenset({Module.ALIGNMENT, Module.DISCRIMINATING_POWER}))
    directories = plugin_directories(Path("/repo"), Path("/tools/plugins/musthold"), modules)
    assert [str(d) for d in directories] == [
        "/tools/plugins/musthold",
        "/repo/discpower/plugin/claude",
        "/repo/alignment/plugin/claude",
    ]
    assert flag_values(session_argv("p", directories, EXECUTABLE), "--plugin-dir") == [str(d) for d in directories]


def test_the_prompt_asks_for_the_skill_and_a_recording_of_every_prove() -> None:
    prompt = session_prompt(RECORD)
    assert "musthold:prove-all" in prompt
    assert "--record /runs/r1/rec" in prompt
    assert "prove --resume" in prompt


def test_the_environment_isolates_the_configuration_and_drops_the_nesting_variables() -> None:
    base = {"PATH": "/usr/bin", "CLAUDECODE": "1", "CLAUDE_CODE_ENTRYPOINT": "cli", "HOME": "/home/o"}
    env = session_environment(base, {"XDG_STATE_HOME": "/s"}, Path("/room/claude-config"), "tok")
    assert env[CONFIG_DIRECTORY_ENV] == "/room/claude-config"
    assert env[OAUTH_TOKEN_VARIABLE] == "tok"
    assert env["XDG_STATE_HOME"] == "/s"
    assert "CLAUDECODE" not in env
    assert "CLAUDE_CODE_ENTRYPOINT" not in env


def test_the_proof_modules_are_listed_in_generate_order() -> None:
    assert [m.binary for m in PROOF_MODULES] == ["musthold-dp", "musthold-alignment", "musthold-completeness"]
    assert PROOF_MODULES[0].server == "plugin:musthold-dp:exec"


def test_a_resumed_session_continues_the_session_it_names() -> None:
    fresh = session_argv("p", (Path("/u"),), EXECUTABLE)
    resumed = session_argv("p", (Path("/u"),), EXECUTABLE, resume="5b0c2a64")
    assert "--resume" not in fresh
    assert flag_values(resumed, "--resume") == ["5b0c2a64"]
    assert flag_values(resumed, "--model") == flag_values(fresh, "--model")


def test_the_round_prompt_names_each_modules_fulfil_skill_and_forbids_proving_or_self_answering() -> None:
    prompt = round_prompt(PROOF_MODULES[:2])
    assert "musthold-dp-fulfil" in prompt
    assert "musthold-alignment-fulfil" in prompt
    assert "musthold-completeness" not in prompt
    assert "Run no `prove` command" in prompt
    assert "--record" not in prompt
