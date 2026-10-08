from __future__ import annotations

import subprocess
from datetime import datetime
from pathlib import Path

import pytest

from specbench.adapters.run_store import FileCallStore, FileLedgerStore, RunStore
from specbench.adapters.workspace_repo import DiskTreeReader
from specbench.application.paced_calls import CallGate, PacedAgentHarness
from specbench.cli.commands import Console
from specbench.cli.main import run
from specbench.domain.arms import FROZEN_MODEL, Arm, Harness
from specbench.domain.canonical import sha256_hex
from specbench.domain.deviations import DeviationEntry, DeviationKind
from specbench.domain.manifest import CALL_FILES, Manifest, RunLayout
from specbench.domain.pacing import PacingPolicy, RunPaused, RunWindow
from specbench.domain.sealing import SealedBundle
from specbench.domain.sessions import CallRecord, SessionKind, SessionRequest, SessionResult
from support.builders import PROVENANCE
from support.fakes import FakeAgentHarness, FixedClock
from support.upstream import GOOD_V1, TASK, bundle, make_upstream, task

pytestmark = pytest.mark.integration


def cli(*arguments: str) -> int:
    return run(list(arguments), Console([]))


def head(path: Path) -> str:
    return subprocess.run(
        ["git", "-C", str(path), "rev-parse", "HEAD"], capture_output=True, text=True, check=True
    ).stdout


def seal_upstream_task(tmp_path: Path) -> tuple[Path, Path, Path]:
    repo, base, merge = make_upstream(tmp_path)
    corpus, keys, plain = tmp_path / "corpus", tmp_path / "keys" / "age.key", tmp_path / "plain"
    cli("keygen", "--identity", str(keys), "--recipients", str(corpus / "recipients.txt"))
    for path, content in bundle(GOOD_V1).files.items():
        (plain / path).parent.mkdir(parents=True, exist_ok=True)
        (plain / path).write_bytes(content)
    (plain / "task.json").write_text(task(base, merge).to_canonical_json())
    cli("seal", "--corpus", str(corpus), "--plain", str(plain))
    return repo, corpus, keys


def test_the_workspace_is_a_single_commit_without_any_sealed_byte(tmp_path: Path) -> None:
    repo, corpus, keys = seal_upstream_task(tmp_path)
    init_dir = tmp_path / "init"
    init_dir.mkdir()
    (init_dir / "AGENTS.md").write_text("Work from SPEC.md.\n")
    sealed = bundle(GOOD_V1)

    def build(destination: Path) -> int:
        return cli(
            "build-workspace",
            "--corpus",
            str(corpus),
            "--identity",
            str(keys),
            "--task-id",
            TASK,
            "--clone",
            str(repo),
            "--init-dir",
            str(init_dir),
            "--out",
            str(destination),
        )

    assert build(tmp_path / "w1") == 0 and build(tmp_path / "w2") == 0
    workspace = tmp_path / "w1"
    files = DiskTreeReader().read(workspace)
    assert {"SPEC.md", "AGENTS.md", "pkg/fee.py", "tests/test_r.py"} <= set(files)
    assert not any(path.startswith(("oracle/", "violations/", "planted/")) for path in files)
    digests = {sha256_hex(c).encode() for p, c in sealed.files.items() if p != "SPEC.md"}
    assert not any(digest in content for content in files.values() for digest in digests)
    count = subprocess.run(
        ["git", "-C", str(workspace), "rev-list", "--count", "HEAD"], capture_output=True, text=True, check=True
    ).stdout.strip()
    assert count == "1"
    assert head(tmp_path / "w1") == head(tmp_path / "w2")


def test_call_records_round_trip_through_the_run_directory(tmp_path: Path) -> None:
    store = FileCallStore(RunLayout(tmp_path))
    record = CallRecord(
        "call-1", ("claude", "-p"), "prompt\nline", "answer", ('{"a":1}', '{"b":2}'), 0, "s-1", "claude-sonnet-5-5"
    )
    assert not store.exists("call-1")
    store.write(record)
    assert store.exists("call-1") and store.read("call-1") == record
    assert {p.name for p in (tmp_path / "calls" / "call-1").iterdir()} == set(CALL_FILES)


def test_a_paused_run_resumes_on_the_next_night_without_repeating_recorded_calls(tmp_path: Path) -> None:
    layout = RunLayout(tmp_path)
    window = RunWindow.parse("21:00-07:30")
    answers = {"calls": 0}

    def answer(request: SessionRequest) -> SessionResult:
        answers["calls"] += 1
        return SessionResult(f"s-{answers['calls']}", f"done {request.prompt}", 0, (), "claude-sonnet-5-5")

    def harness_at(moment: datetime) -> PacedAgentHarness:
        inner = FakeAgentHarness(Harness.CLAUDE_CODE, answer)
        gate = CallGate(
            Harness.CLAUDE_CODE, PacingPolicy(window, budget_per_night=2), FileLedgerStore(layout), FixedClock(moment)
        )
        return PacedAgentHarness(inner, gate, FileCallStore(layout))

    def request(prompt: str) -> SessionRequest:
        return SessionRequest(
            Harness.CLAUDE_CODE,
            FROZEN_MODEL[Harness.CLAUDE_CODE],
            SessionKind.TRUNK,
            "T1/claude/stop-1",
            tmp_path,
            prompt,
        )

    first_night = harness_at(datetime(2026, 10, 8, 22, 0))
    first_night.run_session(request("a"))
    first_night.run_session(request("b"))
    with pytest.raises(RunPaused):
        first_night.run_session(request("c"))
    assert FileLedgerStore(layout).load().calls_on(datetime(2026, 10, 8).date(), Harness.CLAUDE_CODE) == 2

    next_night = harness_at(datetime(2026, 10, 9, 22, 0))
    assert next_night.run_session(request("a")).final_message == "done a"
    assert next_night.run_session(request("c")).final_message == "done c"
    assert answers["calls"] == 3


def test_manifest_and_deviations_are_stored_in_canonical_form(tmp_path: Path) -> None:
    run_store = RunStore(RunLayout(tmp_path / "run"))
    manifest = Manifest(
        run_id="sample-claude-01",
        seed=1,
        prereg_tag="prereg-sample-1",
        provenance=PROVENANCE,
        corpus_lock_digest="2" * 64,
        prices_lock_digest="3" * 64,
        os_name="darwin",
        toolchains={"python": "3.13.14"},
        harness_models={Harness.CLAUDE_CODE: FROZEN_MODEL[Harness.CLAUDE_CODE]},
        sample_tasks=(TASK,),
    )
    run_store.write_manifest(manifest)
    assert run_store.read_manifest() == manifest
    run_store.append_deviation(DeviationEntry.create(DeviationKind.QUOTA_PAUSE, [], "window closed"))
    run_store.append_deviation(
        DeviationEntry.create(DeviationKind.HARNESS_MISCONFIGURED, [Arm.ALL], "plugins missing from init")
    )
    assert [e.kind for e in run_store.read_deviations()] == [
        DeviationKind.QUOTA_PAUSE,
        DeviationKind.HARNESS_MISCONFIGURED,
    ]
    assert SealedBundle({"a": b"x"}).paths == {"a"}
