"""`PinnedClaudeCode`: the version of `script/claude-code.pin`, the binary under `.tools/claude-code/<version>/bin`, and
the environment of a session whose CLI must not update itself."""

from __future__ import annotations

from pathlib import Path

import pytest

from specbench.adapters.pinned_claude_code import (
    INSTALL_COMMAND,
    UPDATER_VARIABLE,
    PinnedClaudeCode,
    pinned_binary,
    updater_disabled,
)
from specbench.application.ports import PinnedClaudeUnavailable

VERSION = "2.1.290"


def repository_with_pin(root: Path, content: str = f"{VERSION}\n") -> Path:
    (root / "script").mkdir()
    (root / "script" / "claude-code.pin").write_text(content, encoding="utf-8")
    return root


def install(root: Path, version: str = VERSION, mode: int = 0o755) -> Path:
    binary = pinned_binary(root, version)
    binary.parent.mkdir(parents=True)
    binary.write_text("#!/bin/sh\n", encoding="utf-8")
    binary.chmod(mode)
    return binary


def test_the_version_is_the_one_line_of_the_pin_file(tmp_path: Path) -> None:
    assert PinnedClaudeCode(repository_with_pin(tmp_path)).version == VERSION


def test_the_executable_is_the_installed_binary_of_the_pinned_version(tmp_path: Path) -> None:
    repository = repository_with_pin(tmp_path)
    binary = install(repository)
    install(repository, "2.1.295")
    assert PinnedClaudeCode(repository).executable() == str(binary)
    assert binary == repository / ".tools" / "claude-code" / VERSION / "bin" / "claude"


def test_a_binary_of_another_version_does_not_stand_in_for_the_pinned_one(tmp_path: Path) -> None:
    repository = repository_with_pin(tmp_path)
    install(repository, "2.1.295")
    with pytest.raises(PinnedClaudeUnavailable, match=INSTALL_COMMAND):
        PinnedClaudeCode(repository).executable()


def test_a_binary_that_cannot_be_executed_is_unavailable(tmp_path: Path) -> None:
    repository = repository_with_pin(tmp_path)
    install(repository, mode=0o644)
    with pytest.raises(PinnedClaudeUnavailable, match=INSTALL_COMMAND):
        PinnedClaudeCode(repository).executable()


@pytest.mark.parametrize("content", [None, "", "\n"])
def test_a_missing_or_empty_pin_is_unavailable(tmp_path: Path, content: str | None) -> None:
    repository = tmp_path if content is None else repository_with_pin(tmp_path, content)
    with pytest.raises(PinnedClaudeUnavailable, match=r"claude-code\.pin"):
        PinnedClaudeCode(repository).executable()


def test_the_updater_is_switched_off_in_the_copy_and_never_in_the_process_environment() -> None:
    original = {"PATH": "/bin"}
    assert updater_disabled(original) == {"PATH": "/bin", UPDATER_VARIABLE: "1"}
    assert UPDATER_VARIABLE not in original
