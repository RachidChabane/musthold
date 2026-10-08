"""The neutral environment of a coder session: a bench-owned HOME, offline caches in the task environment."""

from __future__ import annotations

from pathlib import Path

from specbench.adapters.session_environment import bench_home, neutral_environment, toolchain_caches
from specbench.domain.manifest import RunLayout

OWNER = "/Users/owner"
TASK_ENV = Path("/work/envs/task-a")
SESSION_HOME = Path("/runs/r1/homes/task-a/home")


def test_home_is_the_bench_home_and_the_owners_is_gone() -> None:
    environment = neutral_environment({"HOME": OWNER, "PATH": "/bin"}, SESSION_HOME, TASK_ENV)
    assert environment["HOME"] == str(SESSION_HOME) and environment["PATH"] == "/bin"


def test_caches_point_into_the_task_environment_and_builds_stay_offline() -> None:
    environment = neutral_environment({"HOME": OWNER}, SESSION_HOME, TASK_ENV)
    for name in (
        "GOMODCACHE", "GOCACHE", "GOPATH", "UV_CACHE_DIR", "PIP_CACHE_DIR", "npm_config_cache", "XDG_CACHE_HOME",
        "GRADLE_USER_HOME",
    ):  # fmt: skip
        assert environment[name].startswith(str(TASK_ENV) + "/"), name
    assert environment["MAVEN_OPTS"] == f"-Dmaven.repo.local={TASK_ENV}/m2"
    assert environment["GOPROXY"] == "off" and environment["GOFLAGS"] == "-mod=readonly"
    assert environment["UV_OFFLINE"] == "1" and environment["GOTOOLCHAIN"] == "local"


def test_without_a_task_environment_no_cache_is_set() -> None:
    assert "GOMODCACHE" not in neutral_environment({"HOME": OWNER}, SESSION_HOME, None)


def test_git_has_an_identity_and_never_reads_the_owners_configuration() -> None:
    environment = neutral_environment({"HOME": OWNER}, SESSION_HOME, None)
    assert environment["GIT_CONFIG_GLOBAL"] == "/dev/null" and environment["GIT_CONFIG_NOSYSTEM"] == "1"
    assert environment["GIT_AUTHOR_EMAIL"] == environment["GIT_COMMITTER_EMAIL"]


def test_values_pointing_into_the_owners_home_are_dropped_except_path() -> None:
    environment = neutral_environment(
        {"HOME": OWNER, "PATH": f"{OWNER}/.local/bin:/bin", "MISE_DATA_DIR": f"{OWNER}/.local/share/mise", "LANG": "C"},
        SESSION_HOME,
        None,
    )
    assert "MISE_DATA_DIR" not in environment and environment["LANG"] == "C"
    assert environment["PATH"] == f"{OWNER}/.local/bin:/bin"


def test_the_bench_home_lives_under_the_trunks_homes(tmp_path: Path) -> None:
    assert bench_home(RunLayout(tmp_path), "task-a") == tmp_path / "homes" / "task-a" / "home"


def test_toolchain_caches_are_all_absolute_paths_of_the_task_environment() -> None:
    caches = toolchain_caches(TASK_ENV)
    assert caches["XDG_CACHE_HOME"] == str(TASK_ENV / "cache")
