"""The site end to end: the fixture build against its golden digests, and a sealed run through the command line."""

from __future__ import annotations

import dataclasses
import hashlib
import os
import shutil
from collections.abc import Mapping
from pathlib import Path

import pytest

from e2e.test_synthetic_run import manifest
from specbench.adapters.file_corpus import FileCorpus
from specbench.adapters.run_store import RunStore
from specbench.adapters.site_output import BUILD_MARKER, NotASiteDirectory, write_site
from specbench.cli.commands import Console, run_analyze
from specbench.cli.main import run as cli
from specbench.cli.site_command import SiteRequest, run_site
from specbench.domain.arms import Harness
from specbench.domain.languages import Language
from specbench.domain.manifest import RunLayout
from specbench.domain.sealing import CorpusLock, lock_entry_for
from support.builders import DIGEST, make_bundle, task_json
from support.site import GOLDEN_COMMIT, REPOSITORY_ROOT, fixture_site
from support.synthetic_run import write_synthetic_run

GOLDEN = Path(__file__).parent.parent / "golden" / "site.sha256"
UPDATE = os.environ.get("SPECBENCH_UPDATE_GOLDEN") == "1"
FIXTURE_RUN = REPOSITORY_ROOT / "bench/tests/fixtures/site_run"
QUICK_RESAMPLES = 200
QUICK_PERMUTATIONS = 20


def digests(files: Mapping[str, str]) -> str:
    return "".join(
        f"{hashlib.sha256(content.encode('utf-8')).hexdigest()}  {path}\n" for path, content in files.items()
    )


def test_the_fixture_site_is_byte_identical_to_its_golden_digests(tmp_path: Path) -> None:
    site = fixture_site()
    if UPDATE:
        GOLDEN.write_text(digests(site), encoding="utf-8")
    expected = GOLDEN.read_text(encoding="utf-8")
    if digests(site) != expected:
        write_site(site, tmp_path / "site")
        pytest.fail(f"the fixture site differs from {GOLDEN.name}; the build is in {tmp_path / 'site'}")


def test_two_builds_are_byte_identical() -> None:
    assert fixture_site() == fixture_site()


def sealed_root(tmp_path: Path) -> tuple[Path, str]:
    """A repository root holding a committed corpus lock of the sample tasks, and the lock's digest."""
    root = tmp_path / "repository"
    lock = CorpusLock(
        tuple(lock_entry_for(task, Language.PYTHON, task_json(), make_bundle()) for task in manifest().sample_tasks)
    )
    FileCorpus(root / "bench/corpus").write_lock(lock)
    return root, lock.digest


def sealed_run(tmp_path: Path, run_id: str = "sample-sealed-02", corpus_digest: str | None = None) -> tuple[Path, Path]:
    root, digest = sealed_root(tmp_path)
    run = tmp_path / run_id
    named = corpus_digest if corpus_digest is not None else digest
    write_synthetic_run(run, dataclasses.replace(manifest(), run_id=run_id, corpus_lock_digest=named))
    assert run_analyze(run, QUICK_RESAMPLES, QUICK_PERMUTATIONS, Console([])) == 0
    return run, root


def cli_site(run: Path, root: Path, out: Path, console: Console) -> int:
    return cli(["site", "--run", str(run), "--root", str(root), "--out", str(out), "--commit", GOLDEN_COMMIT], console)


def request(run: Path | None, out: Path, check_only: bool = False, root: Path = REPOSITORY_ROOT) -> SiteRequest:
    return SiteRequest(run, root, out, GOLDEN_COMMIT, check_only)


def test_a_sealed_run_builds_an_unbadged_site_that_passes_its_check(tmp_path: Path) -> None:
    run, root = sealed_run(tmp_path)
    console = Console([])
    assert run_site(request(run, tmp_path / "out", root=root), console) == 0, console.out
    index = (tmp_path / "out" / "index.html").read_text(encoding="utf-8")
    assert 'data-build="sealed-run"' in index
    assert "Synthetic data" not in index
    assert "holds no case record" in index


def test_a_run_without_a_sealed_corpus_is_refused(tmp_path: Path) -> None:
    run, _ = sealed_run(tmp_path)
    console = Console([])
    assert cli_site(run, tmp_path / "bare", tmp_path / "out", console) == 1
    assert any("there is no sealed corpus" in line for line in console.out)
    assert not (tmp_path / "out").exists()


def test_synthetic_trunks_under_a_new_run_id_are_refused(tmp_path: Path) -> None:
    run, root = sealed_run(tmp_path, corpus_digest=DIGEST)
    console = Console([])
    assert cli_site(run, root, tmp_path / "out", console) == 1
    assert any("names corpus lock" in line for line in console.out)


def test_a_trunk_outside_the_sample_is_refused(tmp_path: Path) -> None:
    run, root = sealed_run(tmp_path)
    layout = RunLayout(run)
    stray = layout.trunk("T1", Harness.CODEX)
    target = layout.trunk("T9", Harness.CODEX)
    target.parent.mkdir(parents=True)
    target.write_text(stray.read_text(encoding="utf-8").replace('"task_id":"T1"', '"task_id":"T9"'), encoding="utf-8")
    console = Console([])
    assert cli_site(run, root, tmp_path / "out", console) == 1
    assert any("T9 are not of a sample task" in line for line in console.out)


def test_a_run_whose_results_are_not_its_analysis_is_refused(tmp_path: Path) -> None:
    run, root = sealed_run(tmp_path)
    layout = RunLayout(run)
    written = layout.results.read_text(encoding="utf-8")
    layout.results.write_text(written.replace('"label":"descriptive"', '"label":"conclusive"'), encoding="utf-8")
    console = Console([])
    assert cli_site(run, root, tmp_path / "out", console) == 1
    assert any("is not what the analysis writes" in line for line in console.out)
    assert not (tmp_path / "out").exists()


def test_the_fixture_run_is_refused_as_a_sealed_run(tmp_path: Path) -> None:
    run = tmp_path / "copy"
    shutil.copytree(FIXTURE_RUN, run)
    shutil.copy(REPOSITORY_ROOT / "bench/tests/golden/results.json", run / "results.json")
    console = Console([])
    assert cli_site(run, REPOSITORY_ROOT, tmp_path / "o", console) == 1
    assert any("synthetic fixtures" in line for line in console.out)
    assert not (tmp_path / "o").exists()


def test_the_fixtures_build_through_the_command_line(tmp_path: Path) -> None:
    console = Console([])
    assert run_site(request(None, tmp_path / "site"), console) == 0
    assert (tmp_path / "site" / BUILD_MARKER).is_file()
    assert (tmp_path / "site" / "cases" / "parcel-shipping-boundary.html").is_file()
    assert RunStore(RunLayout(FIXTURE_RUN)).read_manifest().run_id in (tmp_path / "site" / "reproduce.html").read_text()


def test_check_mode_writes_nothing(tmp_path: Path) -> None:
    assert run_site(request(None, tmp_path / "site", check_only=True), Console([])) == 0
    assert not (tmp_path / "site").exists()


def test_the_writer_never_replaces_a_directory_it_did_not_write(tmp_path: Path) -> None:
    out = tmp_path / "elsewhere"
    out.mkdir()
    (out / "notes.txt").write_text("keep", encoding="utf-8")
    with pytest.raises(NotASiteDirectory):
        write_site({"index.html": "<p></p>"}, out)
    assert (out / "notes.txt").read_text(encoding="utf-8") == "keep"
