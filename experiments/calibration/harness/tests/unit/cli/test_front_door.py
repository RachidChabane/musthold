"""The front door: one executable script and one Makefile target per verb, over the shared `script/_lib`."""

from __future__ import annotations

import os
import re
from typing import Final

import pytest

from calibration.domain.constants import EXPERIMENT_DIR
from repository_paths import REPOSITORY_ROOT

EXPERIMENT: Final = REPOSITORY_ROOT / EXPERIMENT_DIR
SCRIPTS: Final = EXPERIMENT / "script"
MAKEFILE: Final = EXPERIMENT / "Makefile"
LIBRARY: Final = SCRIPTS / "_lib"
HEAVY_VERBS: Final = ("build-items", "pin", "canary", "run", "analyze")
LIGHT_VERBS: Final = ("snapshot", "run-session", "log-deviation", "derive", "report-lint")
CLI_VERBS: Final = (*HEAVY_VERBS, *LIGHT_VERBS)
TOOLING_VERBS: Final = ("setup", "lint", "test", "check")
ALL_VERBS: Final = (*CLI_VERBS, *TOOLING_VERBS)
SHEBANG: Final = "#!/usr/bin/env bash"
STRICT_MODE: Final = "set -euo pipefail"
SOURCE_LIBRARY: Final = 'source "$(dirname "${BASH_SOURCE[0]}")/_lib"'
MEM_GUARD_NAME: Final = "mem-guard"
WORKFLOW_DIRECTORY: Final = ".github"
WRITING_COMMANDS: Final = ("git add", "git commit", "git push", "git mv", "git rm", "sed -i", "tee ", " > ", " >> ")


def script_text(verb: str) -> str:
    return (SCRIPTS / verb).read_text()


def code_lines(text: str) -> list[str]:
    return [line for line in text.splitlines() if line.strip() and not line.lstrip().startswith("#")]


def call_for(verb: str) -> str:
    runner = "calibration_heavy" if verb in HEAVY_VERBS else "calibration_light"
    return f'{runner} {verb} "$@"'


def make_targets() -> set[str]:
    return set(re.findall(r"^([a-zA-Z0-9_-]+):", MAKEFILE.read_text(), flags=re.MULTILINE))


def recipe_of(verb: str) -> list[str]:
    lines = MAKEFILE.read_text().splitlines()
    start = next(index for index, line in enumerate(lines) if line.startswith(f"{verb}:"))
    recipe: list[str] = []
    for line in lines[start + 1 :]:
        if not line.startswith("\t"):
            break
        recipe.append(line.strip().removeprefix("@"))
    return recipe


@pytest.mark.parametrize("verb", ALL_VERBS)
def test_every_verb_has_an_executable_bash_script(verb: str) -> None:
    path = SCRIPTS / verb
    assert path.is_file()
    assert os.access(path, os.X_OK)
    assert script_text(verb).splitlines()[0] == SHEBANG
    assert STRICT_MODE in script_text(verb)


@pytest.mark.parametrize("verb", CLI_VERBS)
def test_a_cli_verb_script_sources_the_library_and_forwards_its_arguments_to_its_own_verb(verb: str) -> None:
    lines = code_lines(script_text(verb))
    assert SOURCE_LIBRARY in lines
    assert lines[-1] == call_for(verb)
    assert lines.index(SOURCE_LIBRARY) < lines.index(call_for(verb))


@pytest.mark.parametrize("verb", CLI_VERBS)
def test_a_cli_verb_script_calls_exactly_one_verb(verb: str) -> None:
    calls = [line for line in code_lines(script_text(verb)) if line.startswith("calibration_")]
    assert calls == [call_for(verb)]


def test_the_heavy_and_light_verbs_partition_the_cli_verbs() -> None:
    assert set(HEAVY_VERBS).isdisjoint(LIGHT_VERBS)
    assert len(CLI_VERBS) == len(set(CLI_VERBS))


def test_the_library_is_meant_to_be_sourced() -> None:
    assert LIBRARY.is_file()
    assert "source it, never run it" in LIBRARY.read_text()


def test_the_library_names_the_memory_guard_of_the_repository_and_runs_heavy_work_through_it() -> None:
    text = LIBRARY.read_text()
    assert f'MEM_GUARD="${{REPO_ROOT}}/script/{MEM_GUARD_NAME}"' in text
    assert '"${MEM_GUARD}" run -- "$@"' in text
    assert (REPOSITORY_ROOT / "script" / MEM_GUARD_NAME).is_file()


def test_the_library_defines_both_runners_over_the_frozen_harness_project() -> None:
    text = LIBRARY.read_text()
    for runner in ("calibration_light", "calibration_heavy"):
        assert f"{runner}() {{" in text
    assert text.count('uv run --frozen --project "${HARNESS_DIR}" python -m calibration.cli "$@"') == 2


@pytest.mark.parametrize("verb", ALL_VERBS)
def test_the_makefile_has_a_target_per_verb(verb: str) -> None:
    assert verb in make_targets()


@pytest.mark.parametrize("verb", ALL_VERBS)
def test_the_makefile_recipe_of_a_verb_runs_its_script(verb: str) -> None:
    recipe = recipe_of(verb)
    assert len(recipe) == 1
    assert recipe[0].startswith(f"script/{verb}")


def test_the_makefile_declares_every_verb_phony() -> None:
    declared = next(line for line in MAKEFILE.read_text().splitlines() if line.startswith(".PHONY:"))
    assert set(ALL_VERBS) <= set(declared.split()[1:])


@pytest.mark.parametrize("verb", ALL_VERBS)
def test_no_verb_script_touches_the_workflow_directory(verb: str) -> None:
    assert WORKFLOW_DIRECTORY not in script_text(verb)


@pytest.mark.parametrize("verb", ALL_VERBS)
def test_no_verb_script_writes_to_the_repository_history_or_files_by_itself(verb: str) -> None:
    text = "\n".join(code_lines(script_text(verb)))
    assert [command for command in WRITING_COMMANDS if command in text] == []


def test_the_library_and_the_makefile_do_not_mention_the_workflow_directory() -> None:
    assert WORKFLOW_DIRECTORY not in LIBRARY.read_text()
    assert WORKFLOW_DIRECTORY not in MAKEFILE.read_text()


def test_the_scripts_directory_holds_no_verb_without_a_makefile_target() -> None:
    scripts = {path.name for path in SCRIPTS.iterdir() if path.is_file() and not path.name.startswith("_")}
    assert scripts <= make_targets()
