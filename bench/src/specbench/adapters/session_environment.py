"""The neutral environment every coder session runs in, whichever harness it is.

A session never sees the owner's home: `HOME` is an empty directory the bench owns, so the owner's user skills,
instructions and git configuration cannot reach any arm. Builds and tests still run offline because every toolchain
cache points into the prewarmed task environment the environment builder prepared.
"""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Final

from specbench.adapters.env_builder import UV_ENV, go_offline_environment
from specbench.domain.manifest import RunLayout

HOME_VARIABLE: Final = "HOME"
PATH_VARIABLE: Final = "PATH"
PATH_SEPARATOR: Final = ":"
SHIMS_DIRECTORY: Final = "shims"
DEFAULT_SYSTEM_PATH: Final = "/usr/bin:/bin"
BENCH_HOME_DIRECTORY: Final = "home"
GIT_IDENTITY_NAME: Final = "specbench"
GIT_IDENTITY_EMAIL: Final = "specbench@localhost"
NULL_DEVICE: Final = "/dev/null"
CACHE_DIRECTORY: Final = "cache"
GO_BUILD_CACHE_DIRECTORY: Final = "gocache"
GO_PATH_DIRECTORY: Final = "gopath"
UV_CACHE_DIRECTORY: Final = "uv-cache"
PIP_CACHE_DIRECTORY: Final = "pip-cache"
NPM_CACHE_DIRECTORY: Final = "npm-cache"
GRADLE_HOME_DIRECTORY: Final = "gradle"
MAVEN_REPOSITORY_DIRECTORY: Final = "m2"
GIT_ENVIRONMENT: Final = {
    "GIT_AUTHOR_NAME": GIT_IDENTITY_NAME,
    "GIT_AUTHOR_EMAIL": GIT_IDENTITY_EMAIL,
    "GIT_COMMITTER_NAME": GIT_IDENTITY_NAME,
    "GIT_COMMITTER_EMAIL": GIT_IDENTITY_EMAIL,
    "GIT_CONFIG_GLOBAL": NULL_DEVICE,
    "GIT_CONFIG_NOSYSTEM": "1",
}


def checked_trunk_id(trunk_id: str) -> str:
    if not trunk_id or Path(trunk_id).name != trunk_id or trunk_id in {".", ".."}:
        raise ValueError(f"{trunk_id!r} is not a trunk id")
    return trunk_id


def bench_home(layout: RunLayout, trunk_id: str) -> Path:
    """The `HOME` of a trunk's sessions: empty, owned by the bench, under the run."""
    return layout.homes(checked_trunk_id(trunk_id)) / BENCH_HOME_DIRECTORY


def toolchain_caches(task_environment: Path) -> dict[str, str]:
    """Every cache a build or a test run writes, inside the task environment, offline."""
    return {
        **UV_ENV,
        **go_offline_environment(task_environment),
        "GOCACHE": str(task_environment / GO_BUILD_CACHE_DIRECTORY),
        "GOPATH": str(task_environment / GO_PATH_DIRECTORY),
        "UV_CACHE_DIR": str(task_environment / UV_CACHE_DIRECTORY),
        "UV_OFFLINE": "1",
        "PIP_CACHE_DIR": str(task_environment / PIP_CACHE_DIRECTORY),
        "npm_config_cache": str(task_environment / NPM_CACHE_DIRECTORY),
        "XDG_CACHE_HOME": str(task_environment / CACHE_DIRECTORY),
        "GRADLE_USER_HOME": str(task_environment / GRADLE_HOME_DIRECTORY),
        "MAVEN_OPTS": f"-Dmaven.repo.local={task_environment / MAVEN_REPOSITORY_DIRECTORY}",
    }


def _inside(value: str, directory: Path) -> bool:
    prefix = str(directory).rstrip("/")
    return value == prefix or value.startswith(prefix + "/")


def session_path(inherited_path: str | None, tool_dirs: Sequence[str]) -> str:
    """The `PATH` of a session: the resolved toolchain directories first, then the inherited entries without any mise
    shims directory, which cannot resolve once `HOME` is not the owner's."""
    inherited = inherited_path.split(PATH_SEPARATOR) if inherited_path else DEFAULT_SYSTEM_PATH.split(PATH_SEPARATOR)
    entries = [*tool_dirs, *(entry for entry in inherited if entry and Path(entry).name != SHIMS_DIRECTORY)]
    return PATH_SEPARATOR.join(dict.fromkeys(entries))


def neutral_environment(
    inherited: Mapping[str, str],
    session_home: Path,
    task_environment: Path | None,
    tool_dirs: Sequence[str] = (),
) -> dict[str, str]:
    """The inherited environment without any value that points into the owner's home (`PATH` aside, which only
    lists where programs are and is recomposed), with `HOME` replaced, the toolchain caches set and git's configuration cut off."""
    owner_home = Path(inherited.get(HOME_VARIABLE, "")) if inherited.get(HOME_VARIABLE) else None
    kept = {
        name: value
        for name, value in inherited.items()
        if name != HOME_VARIABLE and (name == PATH_VARIABLE or owner_home is None or not _inside(value, owner_home))
    }
    caches = toolchain_caches(task_environment) if task_environment is not None else {}
    path = session_path(inherited.get(PATH_VARIABLE), tool_dirs)
    return {**kept, **caches, **GIT_ENVIRONMENT, HOME_VARIABLE: str(session_home), PATH_VARIABLE: path}
