"""`IsolatedCliHost` over the real product binaries and Shallnot of `.tools/bin`, with the product's fake host standing in
for the gate-host session: no model is called and no `claude` process is started."""

from __future__ import annotations

import os
import shutil
import subprocess
from collections import defaultdict
from dataclasses import dataclass
from pathlib import Path

import pytest

from specbench.adapters.execution_counter import ClaudeExecutionCounter
from specbench.adapters.isolated_cli_host import IsolatedCliHost, IsolatedHostConfig
from specbench.adapters.process import SubprocessRunner
from specbench.adapters.product_cli import ProductCli
from specbench.adapters.proof_publication import LocalPublication, ProofPublication, SignedPublication
from specbench.adapters.schemas import JsonSchemaChecker
from specbench.adapters.shallnot_cli import ShallnotCli
from specbench.adapters.workspace_repo import DiskTreeReader
from specbench.domain.arms import Arm, Module
from specbench.domain.canonical import JsonValue, stable_json
from specbench.domain.clean_room import PORTABLE_ROOT
from specbench.domain.gate import GateMode, GateOutcome, GateStatus
from specbench.domain.jsonread import mapping, sequence, text
from specbench.domain.manifest import CALL_EVENTS, RunLayout
from specbench.domain.sessions import RequestKind, live_gate_call_id
from support.product_world import (
    BINARIES,
    PARCEL_ANSWERS,
    REPOSITORY,
    UMBRELLA_PLUGIN,
    FakeGateSession,
    SessionKilled,
    build_fakehost,
    make_parcel_tree,
    product_is_built,
)

pytestmark = [
    pytest.mark.integration,
    pytest.mark.timeout(900),
    pytest.mark.skipif(not product_is_built(), reason="the product is not built: run script/product-build"),
    pytest.mark.skipif(shutil.which("go") is None or shutil.which("openssl") is None, reason="needs go and openssl"),
]

SESSION_ID = "5b0c2a64-6a3e-4f4e-9d0e-0a1f6c8d2b11"
BENCH_TOKEN = "test-token"
EXPECTED_FINDINGS = {
    "PAR-1": {"misaligned_test"},
    "PAR-2": {"misaligned_test"},
    "PAR-3": {"misaligned_test", "surviving_valid_mutant"},
}
ALIGNMENT_ONLY_FINDINGS = {"PAR-1": {"misaligned_test"}, "PAR-2": {"misaligned_test"}, "PAR-3": {"misaligned_test"}}
DISCRIMINATING_POWER_UNPROVEN = {"PAR-1": {"stale_proof"}, "PAR-2": {"stale_proof"}, "PAR-3": {"stale_proof"}}


@pytest.fixture(scope="module")
def fakehost(tmp_path_factory: pytest.TempPathFactory) -> Path:
    return build_fakehost(tmp_path_factory.mktemp("fakehost"))


@pytest.fixture(scope="module")
def signing(tmp_path_factory: pytest.TempPathFactory) -> tuple[Path, Path]:
    """The benchmark key and trust policy of a test: a fresh Ed25519 key, owner-readable only."""
    directory = tmp_path_factory.mktemp("signing")
    key = directory / "bench-key.pem"
    subprocess.run(["openssl", "genpkey", "-algorithm", "ed25519", "-out", str(key)], check=True, capture_output=True)
    key.chmod(0o600)
    public = subprocess.run(
        f"openssl pkey -in {key} -pubout -outform DER | tail -c 32 | base64",
        shell=True,
        check=True,
        capture_output=True,
        text=True,
    ).stdout.strip()
    policy = directory / "trust-policy.yaml"
    policy.write_text(f"version: 1\ned25519:\n  - publicKey: {public}\n", encoding="utf-8")
    return key, policy


@dataclass
class Rig:
    root: Path
    host: IsolatedCliHost
    session: FakeGateSession
    tree: Path

    @property
    def record(self) -> Path:
        return self.root / "rec"

    @property
    def layout(self) -> RunLayout:
        return RunLayout(self.root / "run")

    def events(self, arm: Arm) -> list[str]:
        return (self.layout.call(live_gate_call_id(arm, self.record)) / CALL_EVENTS).read_text().splitlines()


def build_rig(
    root: Path,
    fakehost: Path,
    publication: ProofPublication | None = None,
    session: FakeGateSession | None = None,
    tree: Path | None = None,
) -> Rig:
    runner = SubprocessRunner()
    cli = ProductCli(runner, BINARIES)
    shallnot = ShallnotCli(runner, BINARIES / "shallnot", JsonSchemaChecker())
    played = session or FakeGateSession(fakehost, BINARIES, PARCEL_ANSWERS)
    config = IsolatedHostConfig(REPOSITORY, BINARIES, UMBRELLA_PLUGIN, root / "rooms", BENCH_TOKEN)
    host = IsolatedCliHost(
        config,
        RunLayout(root / "run"),
        cli,
        shallnot,
        played,
        publication or LocalPublication(cli),
        DiskTreeReader(),
        ClaudeExecutionCounter(),
        os.environ,
    )
    return Rig(root, host, played, tree or make_parcel_tree(root / "tree"))


def signed_publication(signing: tuple[Path, Path]) -> SignedPublication:
    runner = SubprocessRunner()
    return SignedPublication(ProductCli(runner, BINARIES), runner, *signing)


def findings_by_requirement(report: JsonValue) -> dict[str, set[str]]:
    found: dict[str, set[str]] = defaultdict(set)
    for finding in sequence(mapping(report, "report").get("findings"), "findings"):
        body = mapping(finding, "finding")
        found[text(body.get("requirement_id"), "requirement_id")].add(text(body.get("category"), "category"))
    return dict(found)


def consumed(report: JsonValue) -> list[dict[str, object]]:
    proofs = mapping(mapping(mapping(report, "report").get("extensions"), "extensions").get("proofs"), "proofs")
    return [dict(mapping(item, "consumed")) for item in sequence(proofs.get("consumed"), "consumed")]


def proof_kinds(report: JsonValue) -> set[str]:
    return {text(item["predicateType"], "predicateType").rsplit("/", 2)[-2] for item in consumed(report)}


@pytest.fixture(scope="module")
def live_all(tmp_path_factory: pytest.TempPathFactory, fakehost: Path) -> tuple[Rig, GateOutcome]:
    rig = build_rig(tmp_path_factory.mktemp("all"), fakehost)
    return rig, rig.host.run_gate(rig.tree, Arm.ALL, rig.record, GateMode.LIVE)


def test_a_live_gate_of_the_all_arm_is_complete_with_the_verdicts_the_answers_earned(
    live_all: tuple[Rig, GateOutcome],
) -> None:
    _, outcome = live_all
    assert outcome.status is GateStatus.COMPLETE
    assert outcome.session_id == SESSION_ID
    report = mapping(outcome.report, "report")
    assert report["schema_version"] == "1.2"
    assert report["verdict"] == "fail"
    assert findings_by_requirement(outcome.report) == EXPECTED_FINDINGS


def test_every_statement_of_the_three_modules_reached_the_gate_as_valid(live_all: tuple[Rig, GateOutcome]) -> None:
    _, outcome = live_all
    assert {item["state"] for item in consumed(outcome.report)} == {"valid"}
    assert proof_kinds(outcome.report) == {"alignment", "completeness", "discriminating-power"}


def test_the_stored_report_names_no_machine_path(live_all: tuple[Rig, GateOutcome]) -> None:
    rig, outcome = live_all
    serialized = stable_json(outcome.report)
    assert str(rig.root) not in serialized
    assert serialized.count(PORTABLE_ROOT) > 0


def test_the_executions_are_the_session_plus_one_per_executor_launch(live_all: tuple[Rig, GateOutcome]) -> None:
    rig, outcome = live_all
    assert rig.session.answered > 0
    assert outcome.executions == 1 + rig.session.answered
    assert len(outcome.tickets) == rig.session.answered


def test_each_ticket_carries_its_module_its_request_kind_and_its_cost(live_all: tuple[Rig, GateOutcome]) -> None:
    _, outcome = live_all
    kinds = {(ticket.module, ticket.kind) for ticket in outcome.tickets}
    assert (Module.DISCRIMINATING_POWER, RequestKind.MUTANTS) in kinds
    assert (Module.ALIGNMENT, RequestKind.ALIGNMENT) in kinds
    assert (Module.COMPLETENESS, RequestKind.COMPLETENESS) in kinds
    assert all(ticket.tokens.input_tokens == 60 and ticket.tokens.output_tokens == 40 for ticket in outcome.tickets)
    assert {ticket.resolved_model for ticket in outcome.tickets} == {"claude-sonnet-5-5"}


def test_every_streamed_line_of_the_session_was_kept_in_the_call_events(live_all: tuple[Rig, GateOutcome]) -> None:
    rig, outcome = live_all
    lines = rig.events(Arm.ALL)
    assert ClaudeExecutionCounter().count(lines) == outcome.executions


def test_the_replay_reproduces_the_live_report_byte_for_byte_with_no_execution(
    live_all: tuple[Rig, GateOutcome],
) -> None:
    rig, live = live_all
    replay = rig.host.run_gate(rig.tree, Arm.ALL, rig.record, GateMode.REPLAY)
    assert replay.status is GateStatus.COMPLETE
    assert stable_json(replay.report) == stable_json(live.report)
    assert (replay.executions, replay.tickets, replay.session_id) == (0, (), None)
    assert len(rig.session.specs) == 1


def test_a_replay_starts_from_a_fresh_area_and_leaves_the_session_area_alone(live_all: tuple[Rig, GateOutcome]) -> None:
    rig, _ = live_all
    [spec] = rig.session.specs
    marker = spec.cwd.parent / "ready"
    before = marker.stat().st_mtime_ns
    rig.host.run_gate(rig.tree, Arm.ALL, rig.record, GateMode.REPLAY)
    assert marker.stat().st_mtime_ns == before
    settled = spec.cwd.parent.parent / "settle"
    assert (settled / "tree" / "shallnot.all.yaml").is_file()


def test_the_session_ran_in_a_copy_of_the_tree_with_the_plugins_of_every_module(
    live_all: tuple[Rig, GateOutcome],
) -> None:
    rig, _ = live_all
    [spec] = rig.session.specs
    assert spec.cwd != rig.tree
    plugins = [spec.argv[i + 1] for i, word in enumerate(spec.argv) if word == "--plugin-dir"]
    assert plugins[0] == str(UMBRELLA_PLUGIN)
    assert [Path(p).parts[-3] for p in plugins[1:]] == ["discpower", "alignment", "completeness"]
    assert spec.env["CLAUDE_CODE_OAUTH_TOKEN"] == BENCH_TOKEN
    assert (rig.tree / "shallnot.yaml").exists() is False


def test_a_missing_plugin_in_system_init_aborts_the_gate_as_misconfigured(tmp_path: Path, fakehost: Path) -> None:
    session = FakeGateSession(fakehost, BINARIES, PARCEL_ANSWERS, omit_plugin="spec-proof-alignment")
    rig = build_rig(tmp_path, fakehost, session=session)
    outcome = rig.host.run_gate(rig.tree, Arm.AL, rig.record, GateMode.LIVE)
    assert outcome.status is GateStatus.HARNESS_MISCONFIGURED
    assert outcome.report is None
    assert session.answered == 0
    assert outcome.executions == 1


def test_a_gate_killed_mid_session_resumes_from_its_pending_tickets_in_the_same_room(
    tmp_path: Path, fakehost: Path
) -> None:
    reference = build_rig(tmp_path / "reference", fakehost)
    expected = reference.host.run_gate(reference.tree, Arm.DP, reference.record, GateMode.LIVE)

    killed = FakeGateSession(fakehost, BINARIES, PARCEL_ANSWERS, kill_after_waves=1)
    first = build_rig(tmp_path / "killed", fakehost, session=killed)
    with pytest.raises(SessionKilled):
        first.host.run_gate(first.tree, Arm.DP, first.record, GateMode.LIVE)
    answered_in_the_store = list((killed.specs[0].cwd.parent / "state").glob("spec-proof/discpower/*/host/responses/*"))
    assert answered_in_the_store

    resumed = FakeGateSession(fakehost, BINARIES, PARCEL_ANSWERS)
    second = build_rig(tmp_path / "killed", fakehost, session=resumed, tree=first.tree)
    outcome = second.host.run_gate(first.tree, Arm.DP, first.record, GateMode.LIVE)

    assert outcome.status is GateStatus.COMPLETE
    assert stable_json(outcome.report) == stable_json(expected.report)
    assert 0 < resumed.answered < reference.session.answered
    assert killed.answered + resumed.answered == reference.session.answered
    assert resumed.specs[0].cwd == killed.specs[0].cwd
    assert len(first.events(Arm.DP)) > killed.answered


def test_a_signed_gate_is_verified_offline_and_read_by_shallnot_under_signed_trust(
    tmp_path: Path, fakehost: Path, signing: tuple[Path, Path]
) -> None:
    rig = build_rig(tmp_path, fakehost, publication=signed_publication(signing))
    live = rig.host.run_gate(rig.tree, Arm.AL, rig.record, GateMode.LIVE)
    assert live.status is GateStatus.COMPLETE
    proofs = mapping(mapping(mapping(live.report, "report").get("extensions"), "extensions").get("proofs"), "proofs")
    assert proofs["trust"] == "signed"
    assert {item["state"] for item in consumed(live.report)} == {"valid"}
    assert proof_kinds(live.report) == {"alignment"}
    assert findings_by_requirement(live.report) == ALIGNMENT_ONLY_FINDINGS
    replay = rig.host.run_gate(rig.tree, Arm.AL, rig.record, GateMode.REPLAY)
    assert stable_json(replay.report) == stable_json(live.report)
    assert replay.executions == 0


def test_a_signed_gate_leaves_the_discriminating_power_criteria_unproven_while_ci_requests_stay_unanswered(
    tmp_path: Path, fakehost: Path, signing: tuple[Path, Path]
) -> None:
    rig = build_rig(tmp_path, fakehost, publication=signed_publication(signing))
    live = rig.host.run_gate(rig.tree, Arm.DP, rig.record, GateMode.LIVE)
    assert live.status is GateStatus.COMPLETE
    assert findings_by_requirement(live.report) == DISCRIMINATING_POWER_UNPROVEN
    assert consumed(live.report) == []


def test_an_arm_with_no_proof_module_is_gated_without_a_session(tmp_path: Path, fakehost: Path) -> None:
    rig = build_rig(tmp_path, fakehost)
    outcome = rig.host.run_gate(rig.tree, Arm.TR, rig.record, GateMode.LIVE)
    assert outcome.status is GateStatus.COMPLETE
    assert (outcome.executions, outcome.tickets, rig.session.specs) == (0, (), [])
    assert mapping(outcome.report, "report")["schema_version"] == "1.2"
