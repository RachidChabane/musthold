from __future__ import annotations

from pathlib import Path

from specbench.adapters.claude_gate_session import (
    CONFIG_DIRECTORY_ENV,
    OAUTH_TOKEN_ENV,
    plugin_directories,
    session_argv,
    session_environment,
    session_prompt,
)
from specbench.domain.arms import FROZEN_MODEL, Harness, Module
from specbench.domain.product import PROOF_MODULES, proof_modules_in_order

RECORD = Path("/runs/r1/rec")


def flag_values(argv: tuple[str, ...], flag: str) -> list[str]:
    return [argv[i + 1] for i, word in enumerate(argv) if word == flag]


def test_the_session_runs_the_frozen_model_and_streams_hook_events() -> None:
    argv = session_argv(session_prompt(RECORD), (Path("/u"),))
    assert argv[:2] == ("claude", "-p")
    assert flag_values(argv, "--model") == [FROZEN_MODEL[Harness.CLAUDE_CODE]]
    assert flag_values(argv, "--output-format") == ["stream-json"]
    assert {"--verbose", "--include-hook-events"} <= set(argv)
    assert flag_values(argv, "--setting-sources") == ["user"]


def test_each_plugin_of_the_arm_is_loaded_after_the_umbrella_that_holds_prove_all() -> None:
    modules = proof_modules_in_order(frozenset({Module.ALIGNMENT, Module.DISCRIMINATING_POWER}))
    directories = plugin_directories(Path("/repo"), Path("/tools/plugins/spec-proof"), modules)
    assert [str(d) for d in directories] == [
        "/tools/plugins/spec-proof",
        "/repo/discpower/plugin/claude",
        "/repo/alignment/plugin/claude",
    ]
    assert flag_values(session_argv("p", directories), "--plugin-dir") == [str(d) for d in directories]


def test_the_prompt_asks_for_the_skill_and_a_recording_of_every_prove() -> None:
    prompt = session_prompt(RECORD)
    assert "spec-proof:prove-all" in prompt
    assert "--record /runs/r1/rec" in prompt
    assert "prove --resume" in prompt


def test_the_environment_isolates_the_configuration_and_drops_the_nesting_variables() -> None:
    base = {"PATH": "/usr/bin", "CLAUDECODE": "1", "CLAUDE_CODE_ENTRYPOINT": "cli", "HOME": "/home/o"}
    env = session_environment(base, {"XDG_STATE_HOME": "/s"}, Path("/room/claude-config"), "tok")
    assert env[CONFIG_DIRECTORY_ENV] == "/room/claude-config"
    assert env[OAUTH_TOKEN_ENV] == "tok"
    assert env["XDG_STATE_HOME"] == "/s"
    assert "CLAUDECODE" not in env
    assert "CLAUDE_CODE_ENTRYPOINT" not in env


def test_the_proof_modules_are_listed_in_generate_order() -> None:
    assert [m.binary for m in PROOF_MODULES] == ["spec-proof-dp", "spec-proof-alignment", "spec-proof-completeness"]
    assert PROOF_MODULES[0].server == "plugin:spec-proof-dp:exec"
