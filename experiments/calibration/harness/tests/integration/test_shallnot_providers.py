from __future__ import annotations

import stat
from pathlib import Path

import pytest

from calibration.adapters.shallnot_build import PinnedShallnotProvider, RecordedShallnotProvider, ShallnotUnavailable
from real_process_runner import RealProcessRunner
from temp_repository import TempRepository

pytestmark = pytest.mark.integration

BUILD_SCRIPT = """#!/bin/sh
set -e
pwd > .build-cwd
mkdir -p .tools/bin
printf '%s\\n' "$SHALLNOT_SRC" > .tools/bin/shallnot
chmod +x .tools/bin/shallnot
"""
FAILING_BUILD_SCRIPT = "#!/bin/sh\necho 'go is required' >&2\nexit 2\n"
SILENT_BUILD_SCRIPT = "#!/bin/sh\nexit 0\n"
GUARD_SCRIPT = """#!/bin/sh
test "$1" = "run" && test "$2" = "--" || exit 64
shift 2
echo guarded > .guard-ran
exec "$@"
"""


def executable(path: Path, text: str) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)
    path.chmod(path.stat().st_mode | stat.S_IXUSR)
    return path


def export_with_pin(root: Path, pin: str, build_script: str) -> Path:
    export = root / "export"
    (export / "script").mkdir(parents=True)
    (export / "script" / "shallnot.pin").write_text(pin + "\n")
    executable(export / "script" / "shallnot-build", build_script)
    return export


@pytest.fixture
def source(tmp_path: Path) -> tuple[TempRepository, str]:
    repository = TempRepository(tmp_path / "shallnot-source")
    return repository, repository.commit({"main.go": "package main\n"})


@pytest.fixture
def guard(tmp_path: Path) -> Path:
    return executable(tmp_path / "mem-guard", GUARD_SCRIPT)


def test_the_pinned_build_runs_the_exported_script_under_the_guard_with_an_absolute_source(
    tmp_path: Path, source: tuple[TempRepository, str], guard: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    repository, pin = source
    export = export_with_pin(tmp_path, pin, BUILD_SCRIPT)
    monkeypatch.chdir(repository.root.parent)
    provider = PinnedShallnotProvider(RealProcessRunner(), guard, Path(repository.root.name))
    binary = provider.provide(export)
    assert binary == export / ".tools" / "bin" / "shallnot"
    assert binary.read_text().strip() == str(repository.root.resolve())
    assert Path((export / ".build-cwd").read_text().strip()).resolve() == export.resolve()
    assert (export / ".guard-ran").exists()


def test_a_missing_pin_commit_names_the_commit_and_the_source(
    tmp_path: Path, source: tuple[TempRepository, str], guard: Path
) -> None:
    repository, _ = source
    missing = "f" * 40
    export = export_with_pin(tmp_path, missing, BUILD_SCRIPT)
    provider = PinnedShallnotProvider(RealProcessRunner(), guard, repository.root)
    with pytest.raises(ShallnotUnavailable) as failure:
        provider.provide(export)
    assert missing in str(failure.value)
    assert str(repository.root.resolve()) in str(failure.value)
    assert not (export / ".tools").exists()


def test_a_source_that_is_not_a_repository_is_unavailable(tmp_path: Path, guard: Path) -> None:
    export = export_with_pin(tmp_path, "a" * 40, BUILD_SCRIPT)
    provider = PinnedShallnotProvider(RealProcessRunner(), guard, tmp_path / "nowhere")
    with pytest.raises(ShallnotUnavailable):
        provider.provide(export)


def test_a_failing_build_is_unavailable_with_its_message(
    tmp_path: Path, source: tuple[TempRepository, str], guard: Path
) -> None:
    repository, pin = source
    export = export_with_pin(tmp_path, pin, FAILING_BUILD_SCRIPT)
    with pytest.raises(ShallnotUnavailable, match="go is required"):
        PinnedShallnotProvider(RealProcessRunner(), guard, repository.root).provide(export)


def test_a_build_that_leaves_no_binary_is_unavailable(
    tmp_path: Path, source: tuple[TempRepository, str], guard: Path
) -> None:
    repository, pin = source
    export = export_with_pin(tmp_path, pin, SILENT_BUILD_SCRIPT)
    with pytest.raises(ShallnotUnavailable, match=r"\.tools/bin/shallnot"):
        PinnedShallnotProvider(RealProcessRunner(), guard, repository.root).provide(export)


def test_the_recorded_provider_returns_its_executable_whatever_the_export(tmp_path: Path) -> None:
    script = executable(tmp_path / "fake-shallnot", "#!/bin/sh\n")
    assert RecordedShallnotProvider(script).provide(tmp_path / "any-export") == script
