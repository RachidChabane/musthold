from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import pytest

from specbench.adapters.env_builder import (
    ENV_RECORD_NAME,
    EnvironmentBuildError,
    NativeEnvBuilder,
    python_executable,
)
from specbench.adapters.process import MemoryFloorRunner, SubprocessRunner, system_free_percent
from specbench.application.ports import EnvRequest
from specbench.domain.canonical import sha256_hex
from specbench.domain.languages import Language

pytestmark = pytest.mark.integration

OFFLINE = {
    "UV_OFFLINE": "1",
    "UV_PYTHON_DOWNLOADS": "never",
    "GOPROXY": "off",
    "GOTOOLCHAIN": "local",
    "GOFLAGS": "-mod=mod",
}


def builder() -> NativeEnvBuilder:
    return NativeEnvBuilder(MemoryFloorRunner(SubprocessRunner(), system_free_percent), OFFLINE)


@pytest.mark.skipif(shutil.which("uv") is None, reason="uv is not installed")
def test_python_environment_is_built_from_uv_lock(tmp_path: Path) -> None:
    source = tmp_path / "tiny-py"
    source.mkdir()
    (source / "pyproject.toml").write_text(
        '[project]\nname = "tiny"\nversion = "0.1.0"\nrequires-python = ">=3.13"\ndependencies = []\n'
    )
    subprocess.run(
        ["uv", "lock", "--project", str(source)],
        check=True,
        capture_output=True,
        env={**OFFLINE, "PATH": os.environ["PATH"], "HOME": os.environ["HOME"]},
    )
    destination = tmp_path / "envs" / "tiny-py"

    built = builder().build(EnvRequest(Language.PYTHON, source, destination))

    assert python_executable(destination).is_file()
    assert built.lockfile_digests == {"uv.lock": sha256_hex((source / "uv.lock").read_bytes())}
    assert built.toolchain_version.startswith("3.13")
    assert (destination / ENV_RECORD_NAME).is_file()
    assert (
        subprocess.run([str(python_executable(destination)), "-c", "print(1)"], capture_output=True, check=True).stdout
        == b"1\n"
    )


@pytest.mark.skipif(shutil.which("uv") is None, reason="uv is not installed")
def test_python_environment_from_requirements_installs_nothing_when_empty(tmp_path: Path) -> None:
    source = tmp_path / "req-py"
    source.mkdir()
    (source / "requirements.txt").write_text("")
    built = builder().build(EnvRequest(Language.PYTHON, source, tmp_path / "envs" / "req-py"))
    assert set(built.lockfile_digests) == {"requirements.txt"}


def test_a_python_tree_without_a_lockfile_is_refused(tmp_path: Path) -> None:
    source = tmp_path / "bare"
    source.mkdir()
    with pytest.raises(EnvironmentBuildError):
        builder().build(EnvRequest(Language.PYTHON, source, tmp_path / "envs" / "bare"))


@pytest.mark.skipif(shutil.which("go") is None, reason="go is not installed")
def test_go_module_cache_is_prepared_from_go_mod(tmp_path: Path) -> None:
    source = tmp_path / "tiny-go"
    source.mkdir()
    (source / "go.mod").write_text("module example.com/tiny\n\ngo 1.21\n")
    (source / "go.sum").write_text("")
    (source / "tiny.go").write_text("package tiny\n\nfunc One() int { return 1 }\n")
    destination = tmp_path / "envs" / "tiny-go"

    built = builder().build(EnvRequest(Language.GO, source, destination))

    assert set(built.lockfile_digests) == {"go.mod", "go.sum"}
    assert built.toolchain_version.startswith("1.")
    assert (destination / ENV_RECORD_NAME).is_file()
