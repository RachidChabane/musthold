from __future__ import annotations

import os
import subprocess
from pathlib import Path

from specbench.adapters.isolated_host_config import GH_SHIM_DIRECTORY
from support.product_world import REPOSITORY

SHIM = REPOSITORY / GH_SHIM_DIRECTORY / "gh"
RUN = "bench-0123456789abcdef"
ARTIFACT = "spec-proof-host-discpower"
EXIT_REFUSED = 2
EXIT_MISSING = 1


def gh(*arguments: str, artifacts: Path | None) -> subprocess.CompletedProcess[str]:
    env = {key: value for key, value in os.environ.items() if key != "SPECBENCH_CI_ARTIFACTS"}
    if artifacts is not None:
        env["SPECBENCH_CI_ARTIFACTS"] = str(artifacts)
    return subprocess.run([str(SHIM), *arguments], env=env, capture_output=True, text=True, check=False)


def stage(root: Path) -> Path:
    artifact = root / RUN / ARTIFACT
    (artifact / "requests").mkdir(parents=True)
    (artifact / "issued.json").write_text("{}", encoding="utf-8")
    (artifact / "requests" / "r1.json").write_text("{}", encoding="utf-8")
    return root


def test_the_shim_is_executable_and_lives_in_a_directory_named_for_the_command_it_replaces() -> None:
    assert SHIM.name == "gh"
    assert os.access(SHIM, os.X_OK)


def test_a_run_download_copies_the_staged_artifact_of_that_run(tmp_path: Path) -> None:
    artifacts = stage(tmp_path / "artifacts")
    destination = tmp_path / "scratch"
    done = gh("run", "download", RUN, "--name", ARTIFACT, "--dir", str(destination), artifacts=artifacts)
    assert done.returncode == 0, done.stderr
    assert sorted(path.name for path in destination.rglob("*.json")) == ["issued.json", "r1.json"]


def test_the_repository_flag_is_accepted_and_ignored(tmp_path: Path) -> None:
    artifacts = stage(tmp_path / "artifacts")
    done = gh(
        "run",
        "download",
        RUN,
        "--name",
        ARTIFACT,
        "--dir",
        str(tmp_path / "d"),
        "--repo",
        "bench/t",
        artifacts=artifacts,
    )
    assert done.returncode == 0, done.stderr


def test_an_artifact_the_bench_did_not_stage_is_missing(tmp_path: Path) -> None:
    artifacts = stage(tmp_path / "artifacts")
    done = gh(
        "run", "download", RUN, "--name", "spec-proof-host-alignment", "--dir", str(tmp_path / "d"), artifacts=artifacts
    )
    assert done.returncode == EXIT_MISSING
    assert "no artifact" in done.stderr


def test_any_other_command_is_refused(tmp_path: Path) -> None:
    artifacts = stage(tmp_path / "artifacts")
    for arguments in (("auth", "status"), ("api", "/user"), ("run", "list"), ("run", "download")):
        assert gh(*arguments, artifacts=artifacts).returncode == EXIT_REFUSED


def test_without_the_staging_directory_the_shim_serves_nothing(tmp_path: Path) -> None:
    done = gh("run", "download", RUN, "--name", ARTIFACT, "--dir", str(tmp_path / "d"), artifacts=None)
    assert done.returncode != 0
