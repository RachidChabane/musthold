"""The gate-host session on Claude Code: one `claude -p` that runs the umbrella `prove-all` skill in a clean room.

Its command is built like the coder session's: the frozen model, an isolated configuration directory authenticated by
`CLAUDE_CODE_OAUTH_TOKEN` (supplied by a credential source and held by this process alone), the plugins of the arm's modules loaded with `--plugin-dir`, user settings only, and the
streamed events with the hook events. `SubprocessGateSessionRunner` is the only place a process for it is started.
"""

from __future__ import annotations

import subprocess
import tempfile
from collections.abc import Callable, Mapping, Sequence
from pathlib import Path
from typing import Final

from specbench.adapters.claude_cli import CLAUDE_EXECUTABLE, isolated_environment
from specbench.adapters.claude_credentials import OAUTH_TOKEN_VARIABLE
from specbench.adapters.process import MachineUnsafe, min_free_percent
from specbench.application.ports import EventSink, GateSessionEnd, GateSessionSpec
from specbench.domain.arms import FROZEN_MODEL, Harness
from specbench.domain.product import UMBRELLA_PLUGIN_NAME, ProofModule

CONFIG_DIRECTORY_ENV: Final = "CLAUDE_CONFIG_DIR"
SETTING_SOURCES: Final = "user"
PERMISSION_MODE: Final = "acceptEdits"
PROVE_ALL_SKILL: Final = f"{UMBRELLA_PLUGIN_NAME}:prove-all"
CLAUDE_MODEL: Final = FROZEN_MODEL[Harness.CLAUDE_CODE]


def session_prompt(record_dir: Path) -> str:
    """What the gate-host session is told: run `prove-all`, record every consumed answer, resume rather than restart."""
    return (
        f"Use the {PROVE_ALL_SKILL} skill on this project, and change no file of the project yourself. "
        f"Add `--record {record_dir}` to every `prove` command you run, `prove --resume` included. "
        "A module whose run is already awaiting the host is continued with `prove --resume`, never started over."
    )


def session_argv(
    prompt: str, plugin_directories: Sequence[Path], executable: str = CLAUDE_EXECUTABLE, model: str = CLAUDE_MODEL
) -> tuple[str, ...]:
    argv = [
        executable,
        "-p",
        prompt,
        "--model",
        model,
        "--output-format",
        "stream-json",
        "--verbose",
        "--include-hook-events",
        "--forward-subagent-text",
        "--setting-sources",
        SETTING_SOURCES,
        "--permission-mode",
        PERMISSION_MODE,
    ]
    for directory in plugin_directories:
        argv += ["--plugin-dir", str(directory)]
    return tuple(argv)


def plugin_directories(repository: Path, umbrella_plugin: Path, modules: Sequence[ProofModule]) -> tuple[Path, ...]:
    """The umbrella plugin, which holds `prove-all`, then the Claude plugin of each module of the arm."""
    return (umbrella_plugin, *[repository / module.plugin_directory for module in modules])


def session_environment(
    base: Mapping[str, str], extra: Mapping[str, str], config_directory: Path, token: str
) -> dict[str, str]:
    """The whole environment of the session: `base` without the nesting variables, the area's variables, the config
    home and the token."""
    return {
        **isolated_environment(base),
        **extra,
        CONFIG_DIRECTORY_ENV: str(config_directory),
        OAUTH_TOKEN_VARIABLE: token,
    }


class SubprocessGateSessionRunner:
    """Starts the session, streams its lines to the sink and stops it when the sink asks; refuses below the memory floor."""

    def __init__(self, free_percent: Callable[[], int], minimum_percent: int | None = None) -> None:
        self._free_percent = free_percent
        self._minimum = minimum_percent if minimum_percent is not None else min_free_percent()

    def run(self, spec: GateSessionSpec, on_event: EventSink) -> GateSessionEnd:
        free = self._free_percent()
        if free < self._minimum:
            raise MachineUnsafe(f"free memory is {free} %, below the {self._minimum} % floor")
        with tempfile.TemporaryFile("w+", encoding="utf-8") as errors:
            process = subprocess.Popen(
                list(spec.argv),
                cwd=spec.cwd,
                env=dict(spec.env),
                stdin=subprocess.DEVNULL,
                stdout=subprocess.PIPE,
                stderr=errors,
                text=True,
            )
            assert process.stdout is not None
            for line in process.stdout:
                if not on_event(line.rstrip("\n")):
                    process.terminate()
                    break
            exit_code = process.wait()
            errors.seek(0)
            return GateSessionEnd(exit_code, errors.read())
