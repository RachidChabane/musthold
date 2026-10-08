from __future__ import annotations

import json
import sys
from datetime import date
from pathlib import Path

import pytest

from specbench.adapters.gh_miner import GitHubSource, RecordingGhClient
from specbench.application.mine import mine
from specbench.cli.commands import Console
from specbench.cli.main import run
from specbench.domain.arms import FROZEN_MODEL, Harness
from specbench.domain.canonical import JsonValue, canonical_json
from specbench.domain.languages import Language
from specbench.domain.manifest import Manifest
from specbench.domain.mining import MiningConfig
from specbench.domain.pacing import EXIT_PAUSED
from support.builders import PROVENANCE, task_json
from support.github_world import FakeGitHub, WorldRepo
from support.synthetic_run import write_synthetic_run
from support.upstream import GOOD_V1, TASK, bundle, make_upstream, task

pytestmark = pytest.mark.integration


def cli(*arguments: str) -> tuple[int, str]:
    console = Console([])
    code = run(list(arguments), console)
    return code, "\n".join(console.out)


def test_mine_dry_run_replays_the_archive_and_writes_the_four_outputs(tmp_path: Path) -> None:
    world = [
        WorldRepo("acme/alpha"),
        WorldRepo("acme/gopher", language="Go", root_files=("LICENSE", "go.mod", "go.sum")),
    ]
    config = MiningConfig(5, date(2026, 12, 31), tuple(Language), 3)
    archive = tmp_path / "archive"
    mine(GitHubSource(RecordingGhClient(FakeGitHub(world), archive)), config)

    code, output = cli(
        "mine",
        "--dry-run",
        "--archive",
        str(archive),
        "--out",
        str(tmp_path / "out"),
        "--seed",
        "5",
        "--freeze-date",
        "2026-12-31",
        "--per-language",
        "3",
    )

    assert code == 0 and "mined" in output
    manifest = json.loads((tmp_path / "out" / "mining.json").read_text())
    assert manifest["seed"] == 5 and manifest["freeze_date"] == "2026-12-31"
    assert (tmp_path / "out" / "candidates.jsonl").read_text().count("\n") == manifest["candidates"]


def test_mine_dry_run_without_a_recording_fails_instead_of_reaching_the_network(tmp_path: Path) -> None:
    code, output = cli("mine", "--dry-run", "--archive", str(tmp_path / "empty"), "--out", str(tmp_path / "out"))
    assert code == 1 and "no recording" in output


def test_seal_verify_and_unseal_round_trip_through_the_cli(tmp_path: Path) -> None:
    corpus, keys, plain = tmp_path / "corpus", tmp_path / "keys" / "age.key", tmp_path / "plain"
    assert cli("keygen", "--identity", str(keys), "--recipients", str(corpus / "recipients.txt"))[0] == 0
    for path, content in bundle(GOOD_V1).files.items():
        (plain / path).parent.mkdir(parents=True, exist_ok=True)
        (plain / path).write_bytes(content)
    (plain / "task.json").write_text(
        task_json(task_id=TASK, language=Language.PYTHON, criteria=task("a" * 40, "b" * 40).criteria)
    )

    assert cli("seal", "--corpus", str(corpus), "--plain", str(plain))[0] == 0
    assert (corpus / "corpus.lock.json").is_file()
    assert cli("verify-corpus", "--corpus", str(corpus))[0] == 0
    assert cli("verify-corpus", "--corpus", str(corpus), "--identity", str(keys))[0] == 0
    unsealed = tmp_path / "unsealed"
    assert (
        cli("unseal", "--corpus", str(corpus), "--identity", str(keys), "--task-id", TASK, "--out", str(unsealed))[0]
        == 0
    )
    assert (unsealed / "SPEC.md").read_bytes() == bundle(GOOD_V1).files["SPEC.md"]

    next(corpus.rglob("task.json")).write_text(
        next(corpus.rglob("task.json")).read_text().replace("acme/parkfee", "acme/x")
    )
    code, output = cli("verify-corpus", "--corpus", str(corpus))
    assert code == 1 and "task.json digest differs" in output


def test_an_identity_inside_the_corpus_repository_is_refused(tmp_path: Path) -> None:
    repository = tmp_path / "repo"
    (repository / ".git").mkdir(parents=True)
    code, output = cli(
        "keygen",
        "--identity",
        str(repository / "age.key"),
        "--recipients",
        str(repository / "corpus" / "recipients.txt"),
    )
    assert code == 1 and "inside" in output
    assert not (repository / "age.key").exists()


def test_sealed_plaintext_is_never_written_inside_the_repository(tmp_path: Path) -> None:
    repo, base, merge = make_upstream(tmp_path)
    corpus, keys, plain = tmp_path / "corpus", tmp_path / "keys" / "age.key", tmp_path / "plain"
    cli("keygen", "--identity", str(keys), "--recipients", str(corpus / "recipients.txt"))
    for path, content in bundle(GOOD_V1).files.items():
        (plain / path).parent.mkdir(parents=True, exist_ok=True)
        (plain / path).write_bytes(content)
    (plain / "task.json").write_text(task(base, merge).to_canonical_json())
    cli("seal", "--corpus", str(corpus), "--plain", str(plain))
    (tmp_path / "corpus" / ".git").mkdir()
    inside = tmp_path / "corpus" / "bench" / "unsealed"
    code, output = cli(
        "unseal", "--corpus", str(corpus), "--identity", str(keys), "--task-id", TASK, "--out", str(inside)
    )
    assert code == 1 and "inside" in output and not inside.exists()
    code, output = cli(
        "build-workspace",
        "--corpus",
        str(corpus),
        "--identity",
        str(keys),
        "--task-id",
        TASK,
        "--clone",
        str(repo),
        "--out",
        str(inside),
    )
    assert code == 1 and "inside" in output and not inside.exists()


def test_verify_task_needs_the_task_environment(tmp_path: Path) -> None:
    code, _ = cli("verify-task", "--corpus", "c", "--identity", "i", "--task-id", TASK, "--clone", "x")
    assert code == 2


def test_environment_and_tree_failures_are_messages_not_tracebacks(tmp_path: Path) -> None:
    source = tmp_path / "bare"
    source.mkdir()
    code, output = cli("build-env", "--language", "python", "--source", str(source), "--dest", str(tmp_path / "env"))
    assert code == 1 and output.startswith("error:")


def test_verify_task_through_the_cli(tmp_path: Path) -> None:
    repo, base, merge = make_upstream(tmp_path)
    corpus, keys, plain = tmp_path / "corpus", tmp_path / "keys" / "age.key", tmp_path / "plain"
    cli("keygen", "--identity", str(keys), "--recipients", str(corpus / "recipients.txt"))
    for path, content in bundle(GOOD_V1).files.items():
        (plain / path).parent.mkdir(parents=True, exist_ok=True)
        (plain / path).write_bytes(content)
    (plain / "task.json").write_text(task(base, merge).to_canonical_json())
    cli("seal", "--corpus", str(corpus), "--plain", str(plain))
    report = tmp_path / "report.json"

    code, output = cli(
        "verify-task",
        "--corpus",
        str(corpus),
        "--identity",
        str(keys),
        "--task-id",
        TASK,
        "--clone",
        str(repo),
        "--env-dir",
        str(tmp_path / "env"),
        "--python",
        sys.executable,
        "--out",
        str(report),
    )

    assert code == 0 and f"{TASK}: verified" in output
    document = json.loads(report.read_text())
    assert document["verified"] is True and document["runs"] == 3
    assert report.read_text() == canonical_json(document)


def test_analyze_and_report_lint_through_the_cli(tmp_path: Path) -> None:
    manifest = Manifest(
        run_id="sample-synthetic-01",
        seed=7,
        prereg_tag="prereg-sample-1",
        provenance=PROVENANCE,
        corpus_lock_digest="2" * 64,
        prices_lock_digest="3" * 64,
        os_name="darwin",
        toolchains={},
        harness_models={h: FROZEN_MODEL[h] for h in Harness},
        sample_tasks=(),
    )
    layout = write_synthetic_run(tmp_path / "run", manifest)
    assert cli("analyze", "--run", str(layout.root), "--resamples", "200", "--permutations", "100")[0] == 0
    first = (layout.results.read_bytes(), layout.report.read_bytes())
    assert cli("analyze", "--run", str(layout.root), "--resamples", "200", "--permutations", "100")[0] == 0
    assert first == (layout.results.read_bytes(), layout.report.read_bytes())
    assert cli("report-lint", "--report", str(layout.report))[0] == 0
    layout.report.write_text(layout.report.read_text() + "\nThe gate proves it.\n")
    code, output = cli("report-lint", "--report", str(layout.report))
    assert code == 1 and "proves" in output


def test_window_status_exits_with_the_pause_code_outside_the_window() -> None:
    assert cli("window-status", "--now", "2026-10-08T23:00:00")[0] == 0
    code, output = cli("window-status", "--now", "2026-10-09T12:00:00")
    assert code == EXIT_PAUSED == 3 and "closed" in output
    assert cli("window-status", "--window", "always", "--now", "2026-10-09T12:00:00")[0] == 0
    assert cli("window-status", "--window", "bogus")[0] == 1


def test_an_unknown_verb_is_a_usage_error() -> None:
    assert cli("frobnicate")[0] == 2


def test_select_and_construct_refuse_a_work_directory_inside_the_repository(tmp_path: Path) -> None:
    selection = tmp_path / "selection"
    selection.mkdir()
    work = selection / "work"
    code, output = cli(
        "select",
        "--mining",
        str(selection),
        "--mirrors",
        str(tmp_path / "m"),
        "--work",
        str(work),
        "--out",
        str(selection),
    )
    assert code == 1 and "inside" in output
    code, output = cli(
        "construct",
        "--selection",
        str(selection),
        "--mining",
        str(selection),
        "--corpus",
        str(tmp_path / "corpus"),
        "--mirrors",
        str(tmp_path / "m"),
        "--work",
        str(work),
    )
    assert code == 1 and "inside" in output


def test_mine_compact_rewrites_the_archive_and_stops(tmp_path: Path) -> None:
    responses = tmp_path / "archive" / "responses"
    responses.mkdir(parents=True)
    document: dict[str, JsonValue] = {
        "params": {},
        "path": "repos/o/r/pulls/1/files",
        "response": [{"filename": "a.py", "patch": "code"}],
        "status": 200,
    }
    (responses / "x.json").write_text(canonical_json(document))
    code, output = cli("mine", "--compact", "--archive", str(tmp_path / "archive"), "--out", str(tmp_path / "out"))
    assert code == 0 and "compacted 1" in output
    assert "patch" not in (responses / "x.json").read_text()
