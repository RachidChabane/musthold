"""`script/claude-code-install` and `script/check-claude-code-pin`, run on a copy of the scripts in `tmp_path` with a
fake native versions directory and a fake npm, so nothing here touches the owner's CLI or the network."""

from __future__ import annotations

import hashlib
import os
import shutil
import stat
import subprocess
from dataclasses import dataclass
from pathlib import Path

import pytest

from specbench.adapters.pinned_claude_code import pinned_binary

pytestmark = pytest.mark.integration

REPOSITORY = Path(__file__).resolve().parents[3]
SCRIPTS = ("claude-code-install", "check-claude-code-pin")
PIN = "2.1.293"
EXIT_PRECONDITION = 2
EXIT_VERSION_MISMATCH = 3
UPDATER_VARIABLE = "DISABLE_AUTOUPDATER"
FAKE_CLAUDE = """#!/bin/sh
[ "${{DISABLE_AUTOUPDATER:-}}" = 1 ] || exit 64
echo "{reported} (Claude Code)"
"""
FAKE_NPM = """#!/bin/sh
# npm install --prefix DIR --no-fund --no-audit PACKAGE@VERSION
prefix="$3"
mkdir -p "$prefix/node_modules/.bin"
printf '#!/bin/sh\\n[ "${{DISABLE_AUTOUPDATER:-}}" = 1 ] || exit 64\\necho "{reported} (Claude Code)"\\n' > "$prefix/node_modules/.bin/claude"
chmod 755 "$prefix/node_modules/.bin/claude"
"""
HARNESS_SOURCE = 'const (\n\tClaudeCodeVersion = "{version}"\n)\n'
CLASSIFY_SOURCE = "func ClassifyPinnedCLIVersions() []string {{ return []string{{{versions}}} }}\n"


def executable(path: Path, content: str) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    path.chmod(path.stat().st_mode | stat.S_IXUSR)
    return path


@dataclass
class Checkout:
    """A repository stand-in holding copies of the scripts, so their `cd ..` lands in `tmp_path`."""

    root: Path
    native: Path

    def run(self, script: str, **environment: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [str(self.root / "script" / script)],
            env={**os.environ, "CLAUDE_CODE_NATIVE_VERSIONS": str(self.native), **environment},
            check=False,
            capture_output=True,
            text=True,
            cwd=self.root,
        )

    @property
    def installed(self) -> Path:
        return self.root / ".tools" / "claude-code" / PIN / "bin" / "claude"

    @property
    def checksum(self) -> Path:
        return self.root / ".tools" / "claude-code" / PIN / "SHA256"

    def native_binary(self, reported: str = PIN) -> Path:
        return executable(self.native / PIN, FAKE_CLAUDE.format(reported=reported))


@pytest.fixture
def checkout(tmp_path: Path) -> Checkout:
    (tmp_path / "script").mkdir()
    for name in SCRIPTS:
        shutil.copy2(REPOSITORY / "script" / name, tmp_path / "script" / name)
    (tmp_path / "script" / "claude-code.pin").write_text(f"{PIN}\n", encoding="utf-8")
    return Checkout(tmp_path, tmp_path / "native")


def test_the_native_binary_is_copied_and_its_checksum_recorded(checkout: Checkout) -> None:
    source = checkout.native_binary()
    done = checkout.run("claude-code-install")
    assert done.returncode == 0, done.stderr
    assert done.stdout.strip().splitlines()[-1] == str(checkout.installed)
    assert not checkout.installed.is_symlink() and checkout.installed.read_bytes() == source.read_bytes()
    assert checkout.checksum.read_text(encoding="utf-8").strip() == hashlib.sha256(source.read_bytes()).hexdigest()


def test_the_installed_path_is_the_one_the_bench_resolves(checkout: Checkout) -> None:
    checkout.native_binary()
    done = checkout.run("claude-code-install")
    assert done.stdout.strip().splitlines()[-1] == str(pinned_binary(checkout.root, PIN))


def test_a_rerun_with_the_binary_in_place_installs_nothing(checkout: Checkout) -> None:
    checkout.native_binary()
    checkout.run("claude-code-install")
    stamp = checkout.installed.stat().st_mtime_ns
    done = checkout.run("claude-code-install")
    assert done.returncode == 0 and "up to date" in done.stderr
    assert checkout.installed.stat().st_mtime_ns == stamp


def test_a_tampered_install_is_replaced_from_the_source(checkout: Checkout) -> None:
    source = checkout.native_binary()
    checkout.run("claude-code-install")
    checkout.installed.write_text(FAKE_CLAUDE.format(reported=PIN) + "# changed\n", encoding="utf-8")
    done = checkout.run("claude-code-install")
    assert done.returncode == 0 and "up to date" not in done.stderr
    assert checkout.installed.read_bytes() == source.read_bytes()


@pytest.mark.parametrize("reported", ["2.1.295", "2.1.29", "2.1.2930", ""])
def test_a_binary_that_reports_another_version_than_the_pin_is_refused_and_not_kept(
    checkout: Checkout, reported: str
) -> None:
    checkout.native_binary(reported)
    done = checkout.run("claude-code-install")
    assert done.returncode == EXIT_VERSION_MISMATCH
    assert PIN in done.stderr and "nothing was kept" in done.stderr
    assert not checkout.installed.exists() and not checkout.checksum.exists()


def test_without_a_native_binary_the_npm_package_at_the_pinned_version_is_installed(checkout: Checkout) -> None:
    npm = executable(checkout.root / "fake-bin" / "npm", FAKE_NPM.format(reported=PIN))
    done = checkout.run("claude-code-install", CLAUDE_CODE_NPM=str(npm))
    assert done.returncode == 0, done.stderr
    assert done.stdout.strip().splitlines()[-1] == str(checkout.installed)
    assert (
        checkout.checksum.read_text(encoding="utf-8").strip()
        == hashlib.sha256(checkout.installed.read_bytes()).hexdigest()
    )


def test_the_npm_package_reporting_another_version_is_refused(checkout: Checkout) -> None:
    npm = executable(checkout.root / "fake-bin" / "npm", FAKE_NPM.format(reported="2.1.295"))
    done = checkout.run("claude-code-install", CLAUDE_CODE_NPM=str(npm))
    assert done.returncode == EXIT_VERSION_MISMATCH and not checkout.installed.exists()


def test_with_no_source_at_all_the_install_names_what_is_missing(checkout: Checkout) -> None:
    done = checkout.run("claude-code-install", CLAUDE_CODE_NPM="no-such-npm-command")
    assert done.returncode == EXIT_PRECONDITION and "no-such-npm-command" in done.stderr


def test_a_pin_that_is_not_a_version_is_refused(checkout: Checkout) -> None:
    (checkout.root / "script" / "claude-code.pin").write_text("latest\n", encoding="utf-8")
    assert checkout.run("claude-code-install").returncode == EXIT_PRECONDITION


class PinSources:
    """The two Go files the pin check reads, written with the versions a test gives them."""

    def __init__(self, root: Path) -> None:
        self.harness = root / "harness.go"
        self.classify = root / "classifyprompt.go"
        self.write(PIN, [PIN])

    def write(self, harness_version: str, classify_versions: list[str]) -> None:
        self.harness.write_text(HARNESS_SOURCE.format(version=harness_version), encoding="utf-8")
        listed = ", ".join(f'"{version}"' for version in classify_versions)
        self.classify.write_text(CLASSIFY_SOURCE.format(versions=listed), encoding="utf-8")

    def environment(self) -> dict[str, str]:
        return {"CLAUDE_CODE_PIN_HARNESS_FILE": str(self.harness), "CLAUDE_CODE_PIN_CLASSIFY_FILE": str(self.classify)}


@pytest.fixture
def pin_sources(checkout: Checkout) -> PinSources:
    return PinSources(checkout.root)


def test_the_pin_check_passes_when_both_go_values_hold_the_pin(checkout: Checkout, pin_sources: PinSources) -> None:
    pin_sources.write(PIN, ["2.1.200", PIN])
    assert checkout.run("check-claude-code-pin", **pin_sources.environment()).returncode == 0


def test_the_pin_check_fails_when_the_harness_version_differs(checkout: Checkout, pin_sources: PinSources) -> None:
    pin_sources.write("2.1.295", [PIN])
    done = checkout.run("check-claude-code-pin", **pin_sources.environment())
    assert done.returncode == 1 and "ClaudeCodeVersion" in done.stderr


@pytest.mark.parametrize("versions", [[], ["2.1.295"], ["2.1.29"]])
def test_the_pin_check_fails_when_the_classifier_versions_lack_the_pin(
    checkout: Checkout, pin_sources: PinSources, versions: list[str]
) -> None:
    pin_sources.write(PIN, versions)
    done = checkout.run("check-claude-code-pin", **pin_sources.environment())
    assert done.returncode == 1 and "ClassifyPinnedCLIVersions" in done.stderr


def test_the_pin_check_holds_on_the_repository_itself() -> None:
    done = subprocess.run(
        [str(REPOSITORY / "script" / "check-claude-code-pin")], check=False, capture_output=True, text=True
    )
    assert done.returncode == 0, done.stderr
