from __future__ import annotations

import hashlib
from pathlib import Path

import pytest

from calibration.adapters.git_vcs import GitVersionControl
from calibration.adapters.product_export import GitProductExporter
from real_process_runner import RealProcessRunner
from temp_repository import TempRepository

pytestmark = pytest.mark.integration

PRODUCT_FILES = {
    "format/format.go": "package format\n",
    "script/shallnot.pin": "831c5454a564b8cf4e1e010780f35543831efe7e\n",
    "script/shallnot-build": "#!/usr/bin/env bash\nexit 0\n",
    "docs/readme.md": "line one\r\nline two\n",
    "binary.dat": b"\x00\xff\x10binary",
}
EXPERIMENT_FILES = {
    "experiments/calibration/PREREG.md": "frozen",
    "experiments/limits-1-3/runs/run-02/mutants/x.json": "{}",
}


def sha256(content: bytes | str) -> str:
    raw = content if isinstance(content, bytes) else content.encode()
    return hashlib.sha256(raw).hexdigest()


@pytest.fixture
def exporter_and_repository(tmp_path: Path) -> tuple[GitProductExporter, TempRepository]:
    repository = TempRepository(tmp_path / "repository")
    repository.commit(
        {**PRODUCT_FILES, **EXPERIMENT_FILES}, executable=frozenset({"script/shallnot-build"}), message="tagged"
    )
    repository.tag("prereg-test", annotated=True)
    vcs = GitVersionControl(repository.root, RealProcessRunner())
    return GitProductExporter(vcs, repository.root), repository


def test_export_unpacks_the_tagged_tree_without_experiments(
    exporter_and_repository: tuple[GitProductExporter, TempRepository], tmp_path: Path
) -> None:
    exporter, _ = exporter_and_repository
    destination = tmp_path / "export"
    digests = exporter.export("prereg-test", destination)
    assert not (destination / "experiments").exists()
    assert set(digests) == set(PRODUCT_FILES)


def test_export_digests_every_file_by_its_bytes_sorted_by_path(
    exporter_and_repository: tuple[GitProductExporter, TempRepository], tmp_path: Path
) -> None:
    exporter, _ = exporter_and_repository
    digests = exporter.export("prereg-test", tmp_path / "export")
    assert list(digests) == sorted(digests)
    assert digests == {path: sha256(content) for path, content in sorted(PRODUCT_FILES.items())}


def test_export_never_edits_a_file_and_keeps_the_executable_bit(
    exporter_and_repository: tuple[GitProductExporter, TempRepository], tmp_path: Path
) -> None:
    exporter, repository = exporter_and_repository
    destination = tmp_path / "export"
    exporter.export("prereg-test", destination)
    for relative, content in PRODUCT_FILES.items():
        raw = content if isinstance(content, bytes) else content.encode()
        assert (destination / relative).read_bytes() == raw
    assert (destination / "script" / "shallnot-build").stat().st_mode & 0o100
    assert not list(repository.root.glob("**/.tools"))


def test_export_reads_the_tag_not_the_working_tree(
    exporter_and_repository: tuple[GitProductExporter, TempRepository], tmp_path: Path
) -> None:
    exporter, repository = exporter_and_repository
    (repository.root / "format" / "format.go").write_text("package changed\n")
    digests = exporter.export("prereg-test", tmp_path / "export")
    assert digests["format/format.go"] == sha256(PRODUCT_FILES["format/format.go"])
