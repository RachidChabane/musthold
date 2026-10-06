"""`import-run-01` on a synthetic copy of a sealed run (ADR 0006)."""

from __future__ import annotations

import hashlib
import json
from types import SimpleNamespace

import pytest
from run01_fixture import build_run01

from pilot.adapters.run_directory import RunDirectory
from pilot.adapters.run_files import DirectoryImportSource, file_digests
from pilot.application.arm_t import ensure_plan
from pilot.application.import_stage import import_run
from pilot.cli.main import main
from pilot.domain.import_manifest import (
    IMPORTED_CANARY_DIRECTORY,
    OPUS_GENERATION_IDS,
    RETIRED_CALL_IDS,
    TERRA_GENERATION_IDS,
    ImportRefused,
    ImportRole,
)

pytestmark = pytest.mark.integration


@pytest.fixture
def root(tmp_path):
    build_run01(tmp_path / "runs" / "run-01")
    return tmp_path


def target(root):
    return RunDirectory(root / "runs" / "run-02")


def test_the_import_copies_every_completed_call_byte_for_byte_and_records_digests(root):
    source = DirectoryImportSource(root / "runs" / "run-01")
    plan = import_run(source, target(root))
    run02 = root / "runs" / "run-02"
    for entry in plan.entries:
        assert (run02 / entry.target_path).read_bytes() == (root / "runs" / "run-01" / entry.source_path).read_bytes()
    record = json.loads((run02 / "imports.json").read_text())
    assert record["source_run"] == "run-01" and record["retired"] == list(RETIRED_CALL_IDS)
    assert {e["role"] for e in record["entries"]} == {r.value for r in ImportRole}
    assert all(set(e) == {"source_path", "target_path", "digest", "role", "call_id"} for e in record["entries"])
    sums_digest = hashlib.sha256((root / "runs" / "run-01" / "SHA256SUMS").read_bytes()).hexdigest()
    assert record["sums_digest"] == sums_digest
    assert set(record["nonces"]) == set(OPUS_GENERATION_IDS + TERRA_GENERATION_IDS)


def test_the_retired_call_and_the_run_level_files_are_not_imported(root):
    import_run(DirectoryImportSource(root / "runs" / "run-01"), target(root))
    run02 = root / "runs" / "run-02"
    assert not (run02 / "raw" / RETIRED_CALL_IDS[0]).exists()
    assert not any((run02 / name).exists() for name in ("plan.json", "DEVIATIONS.md", "manifest.json", "SHA256SUMS"))
    assert len(list((run02 / IMPORTED_CANARY_DIRECTORY).glob("*.json"))) == 12
    assert not (run02 / "canary").exists()


def test_an_import_refuses_when_a_source_file_no_longer_matches_the_seal_and_writes_nothing(root):
    (root / "runs" / "run-01" / "raw" / "relabel__R-OAI" / "result.json").write_text("{}")
    with pytest.raises(ImportRefused, match="modified or missing"):
        import_run(DirectoryImportSource(root / "runs" / "run-01"), target(root))
    assert not (root / "runs" / "run-02").exists()


def test_an_unsealed_source_is_refused(tmp_path):
    build_run01(tmp_path / "runs" / "run-01", seal=False)
    with pytest.raises(ImportRefused, match="not sealed"):
        import_run(DirectoryImportSource(tmp_path / "runs" / "run-01"), RunDirectory(tmp_path / "runs" / "run-02"))


def test_an_import_refuses_when_any_target_path_exists_and_leaves_the_target_untouched(root):
    existing = root / "runs" / "run-02" / "raw" / "relabel__R-AUTHOR" / "result.json"
    existing.parent.mkdir(parents=True)
    existing.write_text("pre-existing")
    with pytest.raises(ImportRefused, match="already holds"):
        import_run(DirectoryImportSource(root / "runs" / "run-01"), target(root))
    assert existing.read_text() == "pre-existing" and not (root / "runs" / "run-02" / "imports.json").exists()


def test_the_import_runs_once(root):
    source = DirectoryImportSource(root / "runs" / "run-01")
    import_run(source, target(root))
    with pytest.raises(ImportRefused):
        import_run(source, target(root))


def test_an_import_after_the_plan_exists_is_refused(root):
    (root / "runs" / "run-02").mkdir(parents=True)
    (root / "runs" / "run-02" / "plan.json").write_text("{}")
    with pytest.raises(ImportRefused, match="plan.json"):
        import_run(DirectoryImportSource(root / "runs" / "run-01"), target(root))


def test_the_source_run_is_left_untouched(root):
    before = file_digests(root / "runs" / "run-01")
    import_run(DirectoryImportSource(root / "runs" / "run-01"), target(root))
    assert file_digests(root / "runs" / "run-01") == before


def test_the_verb_imports_logs_a_timing_entry_and_exits_zero(root, capsys):
    assert main(["import-run-01", "--root", str(root), "--run-id", "run-02"]) == 0
    timing = json.loads((root / "runs" / "run-02" / "timing.json").read_text())
    assert set(timing) == {"import-run-01"} and "started" in timing["import-run-01"]
    assert main(["import-run-01", "--root", str(root), "--run-id", "run-02"]) == 1


def test_plan_only_prints_the_import_digests_and_writes_nothing(root, capsys):
    assert main(["import-run-01", "--root", str(root), "--run-id", "run-02", "--plan-only"]) == 0
    printed = json.loads(capsys.readouterr().out)
    digests = printed["import_digests"]
    assert digests["raw/relabel__R-AUTHOR/result.json"] == hashlib.sha256(
        (root / "runs" / "run-01" / "raw" / "relabel__R-AUTHOR" / "result.json").read_bytes()).hexdigest()
    assert not (root / "runs" / "run-02").exists()


def test_the_plan_of_the_decisional_run_keeps_the_original_nonces_of_the_imported_generations(root):
    import_run(DirectoryImportSource(root / "runs" / "run-01"), target(root))
    criteria = [SimpleNamespace(ref=f"PARK-{n}~1") for n in range(1, 11)]
    plan = ensure_plan(SimpleNamespace(store=target(root)), criteria)
    imported = {c.call_id: c.nonce for c in plan.contrast_x + plan.terra_x if c.imported}
    assert imported == {c: f"nonce-{c}" for c in OPUS_GENERATION_IDS + TERRA_GENERATION_IDS}
    assert RETIRED_CALL_IDS[0] not in {c.call_id for c in plan.all_calls()}
