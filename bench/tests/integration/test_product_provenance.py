from __future__ import annotations

import stat
from collections.abc import Mapping, Sequence
from pathlib import Path

import pytest

from specbench.adapters.product_provenance import LocalProvenance, ProvenanceError, file_digest
from specbench.application.ports import ProcessResult
from specbench.domain.canonical import sha256_hex
from specbench.domain.manifest import PRODUCT_ARTIFACTS, PRODUCT_BINARIES, UMBRELLA_PLUGIN

PIN = "831c5454a564b8cf4e1e010780f35543831efe7e"
VERSIONS = {"claude": "2.4.1 (Claude Code)\n", "codex": "codex-cli 0.9.0\n"}
PLUGIN_RELATIVE = Path(".tools") / "plugins" / "spec-proof"


class FakeCliRunner:
    def __init__(self, versions: Mapping[str, str], returncode: int = 0) -> None:
        self._versions = versions
        self._returncode = returncode
        self.argvs: list[list[str]] = []

    def run(self, argv: Sequence[str], cwd: Path | None = None, env: Mapping[str, str] | None = None) -> ProcessResult:
        self.argvs.append(list(argv))
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
    (plugin / ".claude-plugin" / "plugin.json").write_text('{"name": "spec-proof"}\n')
    (plugin / "skills").mkdir()
    (plugin / "skills" / "SKILL.md").write_text("prove all\n")
    (root / "script").mkdir()
    (root / "script" / "shallnot.pin").write_text(PIN + "\n")
    return root


def test_the_provenance_holds_every_digest_the_pin_and_the_cli_versions(tmp_path: Path) -> None:
    repository = build_repository(tmp_path)
    runner = FakeCliRunner(VERSIONS)
    provenance = LocalProvenance(repository, runner).read()
    assert set(provenance.product_digests) == set(PRODUCT_ARTIFACTS)
    assert provenance.product_digests["spec-proof-s1"] == sha256_hex(b"binary of spec-proof-s1")
    assert provenance.shallnot_commit == PIN
    assert provenance.cli_versions == {"claude": "2.4.1 (Claude Code)", "codex": "codex-cli 0.9.0"}
    assert sorted(argv[0] for argv in runner.argvs) == ["claude", "codex"]


def test_a_changed_binary_or_plugin_file_changes_its_digest_and_nothing_else(tmp_path: Path) -> None:
    repository = build_repository(tmp_path)
    before = LocalProvenance(repository, FakeCliRunner(VERSIONS)).read().product_digests
    (repository / ".tools" / "bin" / "spec-proof-ledger").write_bytes(b"rebuilt")
    (repository / PLUGIN_RELATIVE / "skills" / "SKILL.md").write_text("prove everything\n")
    after = LocalProvenance(repository, FakeCliRunner(VERSIONS)).read().product_digests
    assert {name for name in before if before[name] != after[name]} == {"spec-proof-ledger", UMBRELLA_PLUGIN}


def test_file_digest_is_the_sha256_of_the_content(tmp_path: Path) -> None:
    path = tmp_path / "f"
    path.write_bytes(b"abc")
    assert file_digest(path) == sha256_hex(b"abc")


def test_an_unbuilt_artifact_is_named_with_the_build_command(tmp_path: Path) -> None:
    repository = build_repository(tmp_path)
    (repository / ".tools" / "bin" / "spec-proof-verify").unlink()
    for path in sorted((repository / PLUGIN_RELATIVE).rglob("*"), reverse=True):
        path.unlink() if path.is_file() else path.rmdir()
    (repository / PLUGIN_RELATIVE).rmdir()
    with pytest.raises(ProvenanceError, match=r"spec-proof-verify, umbrella-plugin.*script/product-build"):
        LocalProvenance(repository, FakeCliRunner(VERSIONS)).read()


def test_a_cli_that_reports_no_version_is_an_error(tmp_path: Path) -> None:
    repository = build_repository(tmp_path)
    with pytest.raises(ProvenanceError, match=r"claude --version"):
        LocalProvenance(repository, FakeCliRunner({"claude": "", "codex": "x"})).read()
    with pytest.raises(ProvenanceError, match=r"claude --version"):
        LocalProvenance(repository, FakeCliRunner(VERSIONS, returncode=1)).read()
