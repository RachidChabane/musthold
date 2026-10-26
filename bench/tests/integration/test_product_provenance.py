from __future__ import annotations

import stat
from collections.abc import Mapping, Sequence
from pathlib import Path

import pytest

from specbench.adapters.pinned_claude_code import PinnedClaudeCode
from specbench.adapters.product_provenance import LocalProvenance, ProvenanceError, file_digest
from specbench.application.ports import ProcessResult
from specbench.domain.canonical import sha256_hex
from specbench.domain.manifest import PRODUCT_ARTIFACTS, PRODUCT_BINARIES, UMBRELLA_PLUGIN
from support.pinned_claude import FixedClaude

PIN = "831c5454a564b8cf4e1e010780f35543831efe7e"
VERSIONS = {"claude": "2.4.1 (Claude Code)\n", "codex": "codex-cli 0.9.0\n"}
PINNED = FixedClaude("claude", "2.4.1")
PLUGIN_RELATIVE = Path(".tools") / "plugins" / "musthold"


class FakeCliRunner:
    def __init__(self, versions: Mapping[str, str], returncode: int = 0) -> None:
        self._versions = versions
        self._returncode = returncode
        self.argvs: list[list[str]] = []
        self.environments: list[Mapping[str, str] | None] = []

    def run(self, argv: Sequence[str], cwd: Path | None = None, env: Mapping[str, str] | None = None) -> ProcessResult:
        self.argvs.append(list(argv))
        self.environments.append(env)
        return ProcessResult(self._returncode, self._versions[argv[0]], "")


def build_repository(root: Path) -> Path:
    binaries = root / ".tools" / "bin"
    binaries.mkdir(parents=True)
    for name in PRODUCT_BINARIES:
        binary = binaries / name
        binary.write_bytes(f"binary of {name}".encode())
        binary.chmod(binary.stat().st_mode | stat.S_IXUSR)
    plugin = root / PLUGIN_RELATIVE
    (plugin / ".claude-plugin").mkdir(parents=True)
    (plugin / ".claude-plugin" / "plugin.json").write_text('{"name": "musthold"}\n')
    (plugin / "skills").mkdir()
    (plugin / "skills" / "SKILL.md").write_text("prove all\n")
    (root / "script").mkdir()
    (root / "script" / "shallnot.pin").write_text(PIN + "\n")
    return root


def test_the_provenance_holds_every_digest_the_pin_and_the_cli_versions(tmp_path: Path) -> None:
    repository = build_repository(tmp_path)
    runner = FakeCliRunner(VERSIONS)
    provenance = LocalProvenance(repository, runner, PINNED).read()
    assert set(provenance.product_digests) == set(PRODUCT_ARTIFACTS)
    assert provenance.product_digests["musthold-s1"] == sha256_hex(b"binary of musthold-s1")
    assert provenance.shallnot_commit == PIN
    assert provenance.cli_versions == {"claude": PINNED.version, "codex": "codex-cli 0.9.0"}
    assert sorted(argv[0] for argv in runner.argvs) == ["claude", "codex"]


def test_a_changed_binary_or_plugin_file_changes_its_digest_and_nothing_else(tmp_path: Path) -> None:
    repository = build_repository(tmp_path)
    before = LocalProvenance(repository, FakeCliRunner(VERSIONS), PINNED).read().product_digests
    (repository / ".tools" / "bin" / "musthold-ledger").write_bytes(b"rebuilt")
    (repository / PLUGIN_RELATIVE / "skills" / "SKILL.md").write_text("prove everything\n")
    after = LocalProvenance(repository, FakeCliRunner(VERSIONS), PINNED).read().product_digests
    assert {name for name in before if before[name] != after[name]} == {"musthold-ledger", UMBRELLA_PLUGIN}


def test_file_digest_is_the_sha256_of_the_content(tmp_path: Path) -> None:
    path = tmp_path / "f"
    path.write_bytes(b"abc")
    assert file_digest(path) == sha256_hex(b"abc")


def test_an_unbuilt_artifact_is_named_with_the_build_command(tmp_path: Path) -> None:
    repository = build_repository(tmp_path)
    (repository / ".tools" / "bin" / "musthold-verify").unlink()
    for path in sorted((repository / PLUGIN_RELATIVE).rglob("*"), reverse=True):
        path.unlink() if path.is_file() else path.rmdir()
    (repository / PLUGIN_RELATIVE).rmdir()
    with pytest.raises(ProvenanceError, match=r"musthold-verify, umbrella-plugin.*script/product-build"):
        LocalProvenance(repository, FakeCliRunner(VERSIONS), PINNED).read()


def test_a_cli_that_reports_no_version_is_an_error(tmp_path: Path) -> None:
    repository = build_repository(tmp_path)
    with pytest.raises(ProvenanceError, match=r"claude --version"):
        LocalProvenance(repository, FakeCliRunner({"claude": "", "codex": "x"}), PINNED).read()
    with pytest.raises(ProvenanceError, match=r"claude --version"):
        LocalProvenance(repository, FakeCliRunner(VERSIONS, returncode=1), PINNED).read()


def test_claude_is_asked_through_the_pinned_binary_with_the_updater_off(tmp_path: Path) -> None:
    repository = build_repository(tmp_path)
    runner = FakeCliRunner({**VERSIONS, "/pinned/claude": VERSIONS["claude"]})
    LocalProvenance(repository, runner, FixedClaude("/pinned/claude", PINNED.version)).read()
    assert ["/pinned/claude", "--version"] in runner.argvs and ["claude", "--version"] not in runner.argvs
    [environment] = [
        env for argv, env in zip(runner.argvs, runner.environments, strict=True) if argv[0] == "/pinned/claude"
    ]
    assert environment is not None and environment["DISABLE_AUTOUPDATER"] == "1"


def test_a_claude_that_reports_another_version_than_the_pin_is_refused(tmp_path: Path) -> None:
    repository = build_repository(tmp_path)
    with pytest.raises(ProvenanceError, match=r"reported .*2\.4\.1.*the pin is 2\.4\.0"):
        LocalProvenance(
            repository, FakeCliRunner({**VERSIONS, "claude": "2.4.1 (Claude Code)"}), FixedClaude("claude", "2.4.0")
        ).read()


def test_a_missing_pinned_binary_names_the_install_script(tmp_path: Path) -> None:
    repository = build_repository(tmp_path)
    (repository / "script" / "claude-code.pin").write_text("2.4.1\n")
    with pytest.raises(ProvenanceError, match=r"script/claude-code-install"):
        LocalProvenance(repository, FakeCliRunner(VERSIONS), PinnedClaudeCode(repository)).read()
