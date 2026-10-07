"""Native per-task environments from lockfiles: uv for Python, `go mod download` for Go (a Strategy per language)."""

from __future__ import annotations

from collections.abc import Mapping
from pathlib import Path
from typing import Final, Protocol

from specbench.application.ports import BuiltEnvironment, EnvRequest, ProcessResult, ProcessRunner
from specbench.domain.canonical import canonical_json, sha256_hex
from specbench.domain.languages import Language
from specbench.domain.mining import lockfiles_for

VENV_DIRECTORY: Final = "venv"
GO_MODULE_CACHE_DIRECTORY: Final = "gomodcache"
ENV_RECORD_NAME: Final = "env.json"
UV_LOCK: Final = "uv.lock"
REQUIREMENTS: Final = "requirements.txt"
UV_ENV: Final = {"UV_PYTHON_DOWNLOADS": "never", "UV_PYTHON_PREFERENCE": "only-system"}
GO_ENV: Final = {"GOFLAGS": "-mod=readonly"}


class EnvironmentBuildError(RuntimeError):
    """A tool failed while building an environment."""


def python_executable(destination: Path) -> Path:
    return destination / VENV_DIRECTORY / "bin" / "python"


def go_environment(destination: Path) -> dict[str, str]:
    return {**GO_ENV, "GOMODCACHE": str(destination / GO_MODULE_CACHE_DIRECTORY)}


def _checked(result: ProcessResult, what: str) -> ProcessResult:
    if result.returncode != 0:
        raise EnvironmentBuildError(f"{what} failed: {result.stderr.strip() or result.stdout.strip()}")
    return result


class LanguageEnvStrategy(Protocol):
    def build(self, runner: ProcessRunner, request: EnvRequest, extra_env: Mapping[str, str]) -> str:
        """Build the environment and return the toolchain version."""
        ...


class PythonUvStrategy:
    """`uv sync --frozen` from `uv.lock`, else a venv with `requirements.txt` installed by `uv pip`."""

    def build(self, runner: ProcessRunner, request: EnvRequest, extra_env: Mapping[str, str]) -> str:
        venv = request.destination / VENV_DIRECTORY
        env = {**UV_ENV, **extra_env}
        if (request.source_tree / UV_LOCK).is_file():
            sync_env = {**env, "UV_PROJECT_ENVIRONMENT": str(venv)}
            _checked(
                runner.run(
                    ["uv", "sync", "--frozen", "--project", str(request.source_tree)], request.source_tree, sync_env
                ),
                "uv sync",
            )
        elif (request.source_tree / REQUIREMENTS).is_file():
            _checked(runner.run(["uv", "venv", str(venv)], request.source_tree, env), "uv venv")
            _checked(
                runner.run(
                    [
                        "uv",
                        "pip",
                        "install",
                        "--python",
                        str(python_executable(request.destination)),
                        "-r",
                        REQUIREMENTS,
                    ],
                    request.source_tree,
                    env,
                ),
                "uv pip install",
            )
        else:
            raise EnvironmentBuildError(f"no {UV_LOCK} or {REQUIREMENTS} in {request.source_tree}")
        version = _checked(
            runner.run([str(python_executable(request.destination)), "--version"], None, env), "python --version"
        )
        return (version.stdout or version.stderr).strip().removeprefix("Python ")


class GoModuleStrategy:
    """`go mod download` into a module cache under the destination."""

    def build(self, runner: ProcessRunner, request: EnvRequest, extra_env: Mapping[str, str]) -> str:
        env = {**go_environment(request.destination), **extra_env}
        _checked(runner.run(["go", "mod", "download"], request.source_tree, env), "go mod download")
        version = _checked(runner.run(["go", "env", "GOVERSION"], request.source_tree, env), "go env GOVERSION")
        return version.stdout.strip().removeprefix("go")


STRATEGIES: Final[Mapping[Language, LanguageEnvStrategy]] = {
    Language.PYTHON: PythonUvStrategy(),
    Language.GO: GoModuleStrategy(),
}


def lockfile_digests(language: Language, tree: Path) -> dict[str, str]:
    return {name: sha256_hex((tree / name).read_bytes()) for name in lockfiles_for(language) if (tree / name).is_file()}


class NativeEnvBuilder:
    """`EnvBuilder`: dispatches to the language's strategy and writes `env.json` beside the environment."""

    def __init__(self, runner: ProcessRunner, extra_env: Mapping[str, str] | None = None) -> None:
        self._runner = runner
        self._extra_env = dict(extra_env or {})

    def build(self, request: EnvRequest) -> BuiltEnvironment:
        request.destination.mkdir(parents=True, exist_ok=True)
        version = STRATEGIES[request.language].build(self._runner, request, self._extra_env)
        digests = lockfile_digests(request.language, request.source_tree)
        record = {"language": request.language.value, "lockfile_digests": digests, "toolchain_version": version}
        (request.destination / ENV_RECORD_NAME).write_text(canonical_json(record), encoding="utf-8")
        return BuiltEnvironment(request.language, request.destination, digests, version)
