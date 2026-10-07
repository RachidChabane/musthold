"""`Layout`: where the experiment's directories are, discovered from the repository root."""

from __future__ import annotations

from pathlib import Path

import pytest

from calibration.cli.layout import REPO_ROOT_ENV, SHALLNOT_SOURCE_DEFAULT, SHALLNOT_SOURCE_ENV, Layout
from calibration.domain.constants import (
    CACHE_DIR_NAME,
    EXPERIMENT_DIR,
    KEV_COMMIT,
    PILOT_DIR,
    PILOT_RUN_ID,
    RESULTS_DIR_NAME,
    RUNS_DIR_NAME,
)
from repository_paths import REPOSITORY_ROOT


@pytest.fixture
def layout(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> Layout:
    monkeypatch.setenv(REPO_ROOT_ENV, str(tmp_path))
    return Layout.discover()


def test_the_repository_root_environment_variable_overrides_discovery(layout: Layout, tmp_path: Path) -> None:
    assert layout.root == tmp_path.resolve()


def test_the_override_is_resolved_to_an_absolute_path(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    (tmp_path / "repository").mkdir()
    monkeypatch.chdir(tmp_path)
    monkeypatch.setenv(REPO_ROOT_ENV, "repository")
    assert Layout.discover().root == (tmp_path / "repository").resolve()


@pytest.mark.parametrize("blank", [None, ""])
def test_without_an_override_the_root_is_the_repository_holding_the_harness(
    monkeypatch: pytest.MonkeyPatch, blank: str | None
) -> None:
    if blank is None:
        monkeypatch.delenv(REPO_ROOT_ENV, raising=False)
    else:
        monkeypatch.setenv(REPO_ROOT_ENV, blank)
    assert Layout.discover().root == REPOSITORY_ROOT


def test_the_directories_hang_below_the_experiment_directory(layout: Layout, tmp_path: Path) -> None:
    experiment = tmp_path.resolve() / EXPERIMENT_DIR
    assert layout.calibration_dir == experiment
    assert layout.harness_dir == experiment / "harness"
    assert layout.builder_dir == experiment / "builder"
    assert layout.cache_dir == experiment / CACHE_DIR_NAME
    assert layout.results_dir() == experiment / RESULTS_DIR_NAME
    assert layout.run_dir("run-07") == experiment / RUNS_DIR_NAME / "run-07"
    assert layout.freeze_receipt == experiment / "freeze" / "prereg-commit.tsr"
    assert layout.canary_prompt == experiment / "prompts" / "canary.json"
    assert layout.kev_directory == experiment / CACHE_DIR_NAME / f"kev-{KEV_COMMIT}"


def test_the_memory_guard_is_the_repository_script(layout: Layout, tmp_path: Path) -> None:
    assert layout.mem_guard == tmp_path.resolve() / "script" / "mem-guard"


def test_the_pilot_run_directory_is_the_pilot_run_below_the_pilot_experiment(layout: Layout, tmp_path: Path) -> None:
    assert layout.pilot_run_dir == tmp_path.resolve() / PILOT_DIR / RUNS_DIR_NAME / PILOT_RUN_ID


def test_the_run_directory_of_the_experiment_is_never_the_pilot_run_directory(layout: Layout) -> None:
    assert layout.run_dir(PILOT_RUN_ID) != layout.pilot_run_dir


def test_the_shallnot_source_defaults_to_a_sibling_of_the_repository(
    layout: Layout, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.delenv(SHALLNOT_SOURCE_ENV, raising=False)
    assert layout.shallnot_source() == (tmp_path.resolve().parent / SHALLNOT_SOURCE_DEFAULT).resolve()


def test_the_shallnot_source_environment_variable_overrides_the_default(
    layout: Layout, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv(SHALLNOT_SOURCE_ENV, str(tmp_path / "elsewhere"))
    assert layout.shallnot_source() == (tmp_path / "elsewhere").resolve()
