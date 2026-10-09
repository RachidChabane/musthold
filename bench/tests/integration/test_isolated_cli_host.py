"""`IsolatedCliHost` over the real product binaries and Shallnot of `.tools/bin`, with the product's fake host standing in
for the gate-host session: no model is called and no `claude` process is started.

Every gate here is signed and goes through the simulated CI round trips (ADR 0011, decision 12). A gate is driven the
way the pacing decorator drives it: the host is called once per entry until it stops pausing for another round trip.
"""

from __future__ import annotations

import json
import shutil
import subprocess
from dataclasses import dataclass
from pathlib import Path

import pytest

from specbench.adapters.ci_round_archive import CiRoundArchive
from specbench.adapters.execution_counter import ClaudeExecutionCounter
from specbench.adapters.isolated_host_config import GH_SHIM_DIRECTORY
from specbench.adapters.isolated_host_factory import SigningMaterial
from specbench.domain.arms import Arm, Module
from specbench.domain.canonical import stable_json
from specbench.domain.ci_seed import CI_ROUND_TRIPS
from specbench.domain.clean_room import PORTABLE_ROOT
from specbench.domain.gate import GateMode, GateOutcome, GatePause, GateStatus
from specbench.domain.jsonread import mapping
from specbench.domain.sessions import RequestKind
from support.clean_room_rig import (
    Rig,
    build_rig,
    consumed,
    drive,
    finding_messages,
    findings_by_requirement,
    make_signing_material,
    proof_kinds,
    trust_of,
)
from support.product_world import (
    BINARIES,
    LOWERED_ISOLATION_FLOOR,
    PARCEL_ANSWERS,
    FakeGateSession,
    SessionKilled,
    build_fakehost,
    make_parcel_tree,
    product_is_built,
)

pytestmark = [
    pytest.mark.integration,
    pytest.mark.timeout(3600),
    pytest.mark.skipif(not product_is_built(), reason="the product is not built: run script/product-build"),
    pytest.mark.skipif(shutil.which("go") is None or shutil.which("openssl") is None, reason="needs go and openssl"),
]

SESSION_ID = "5b0c2a64-6a3e-4f4e-9d0e-0a1f6c8d2b11"
UNCORROBORATED = "uncorroborated"


@pytest.fixture(scope="module")
def fakehost(tmp_path_factory: pytest.TempPathFactory) -> Path:
    return build_fakehost(tmp_path_factory.mktemp("fakehost"))


@pytest.fixture(scope="module")
def signing(tmp_path_factory: pytest.TempPathFactory) -> SigningMaterial:
    return make_signing_material(tmp_path_factory.mktemp("signing"))


@dataclass(frozen=True)
class LiveGate:
    rig: Rig
    arm: Arm
    outcomes: list[GateOutcome]

    @property
    def final(self) -> GateOutcome:
        return self.outcomes[-1]

    @property
    def digest_directory(self) -> Path:
        return next((self.rig.root / "rooms" / self.arm.value).iterdir())

    @property
    def settle_tree(self) -> Path:
        return self.digest_directory / "settle" / "tree"


def credited_inline_rig(root: Path, fakehost: Path, signing: SigningMaterial) -> Rig:
    """The project whose policies credit an inline host, answered by the parcel oracle where nothing is recorded."""
    tree = make_parcel_tree(root / "tree", LOWERED_ISOLATION_FLOOR)
    session = FakeGateSession(fakehost, BINARIES, PARCEL_ANSWERS, oracle_for_all=True)
    return build_rig(root, fakehost, signing, session=session, tree=tree)


def live_gate(root: Path, fakehost: Path, signing: SigningMaterial, arm: Arm) -> LiveGate:
    rig = credited_inline_rig(root, fakehost, signing)
    return LiveGate(rig, arm, drive(rig, arm))


@pytest.fixture(scope="module")
def live_dp(tmp_path_factory: pytest.TempPathFactory, fakehost: Path, signing: SigningMaterial) -> LiveGate:
    return live_gate(tmp_path_factory.mktemp("dp"), fakehost, signing, Arm.DP)


@pytest.fixture(scope="module")
def live_all(tmp_path_factory: pytest.TempPathFactory, fakehost: Path, signing: SigningMaterial) -> LiveGate:
    return live_gate(tmp_path_factory.mktemp("all"), fakehost, signing, Arm.ALL)


def git(tree: Path, *arguments: str) -> str:
    return subprocess.run(
        ["git", "-C", str(tree), *arguments], check=True, capture_output=True, text=True
    ).stdout.strip()


def seed_record_of(gate: LiveGate) -> dict[str, object]:
    head = git(gate.settle_tree, "rev-parse", "HEAD")
    return json.loads((gate.settle_tree / ".spec-proof" / "seeds" / f"{head}.json").read_text(encoding="utf-8"))


def archived_response_ids(gate: LiveGate, number: int) -> set[str]:
    bundle = gate.rig.record / "ci" / str(number) / "discpower" / "responses"
    return {path.name.split(".")[0] for path in bundle.glob("*.json")}


# --- the signed path of a gate that needs CI round trips ------------------------------------------------------


def test_a_dp_gate_pauses_for_each_round_trip_and_reaches_valid_statements_after_the_last(live_dp: LiveGate) -> None:
    *waiting, final = live_dp.outcomes
    assert [(o.status, o.pause, o.ci_rounds) for o in waiting] == [
        (GateStatus.PAUSED, GatePause.CI_ROUND_PENDING, rounds) for rounds in range(CI_ROUND_TRIPS + 1)
    ]
    assert final.status is GateStatus.COMPLETE
    assert final.ci_rounds == CI_ROUND_TRIPS
    assert trust_of(final.report) == "signed"
    assert {item["state"] for item in consumed(final.report)} == {"valid"}
    assert proof_kinds(final.report) == {"discriminating-power"}
    assert "stale_proof" not in {c for found in findings_by_requirement(final.report).values() for c in found}


def test_the_dp_gate_archived_each_round_whole_under_its_record_directory(live_dp: LiveGate) -> None:
    archive = CiRoundArchive(live_dp.rig.record)
    assert archive.rounds() == CI_ROUND_TRIPS
    for number in range(1, CI_ROUND_TRIPS + 1):
        assert (live_dp.rig.record / "ci" / str(number) / "discpower" / "manifest.json").is_file()
    assert not list((live_dp.rig.record / "ci").glob("*.partial"))


def test_each_round_trip_ran_a_resumed_session_with_the_fulfil_prompt_and_no_gh_on_its_path(live_dp: LiveGate) -> None:
    session = live_dp.rig.session
    first, *rounds = session.specs
    assert "--resume" not in first.argv
    assert len(rounds) == CI_ROUND_TRIPS
    for spec in rounds:
        assert spec.argv[spec.argv.index("--resume") + 1] == SESSION_ID
        assert "pending ticket" in spec.argv[spec.argv.index("-p") + 1]
    for spec in session.specs:
        assert GH_SHIM_DIRECTORY.as_posix() not in spec.env["PATH"]
        assert "SPECBENCH_CI_ARTIFACTS" not in spec.env


def test_the_executions_are_one_session_per_entry_that_ran_one_plus_one_per_executor_launch(live_dp: LiveGate) -> None:
    sessions = 1 + CI_ROUND_TRIPS
    assert sum(outcome.executions for outcome in live_dp.outcomes) == sessions + live_dp.rig.session.answered
    assert live_dp.final.executions == 0
    assert live_dp.outcomes[-2].executions > 1


def test_every_session_entry_kept_its_events_in_the_events_file_of_its_own_call(live_dp: LiveGate) -> None:
    rig = live_dp.rig
    assert ClaudeExecutionCounter().count(rig.events(Arm.DP)) == live_dp.outcomes[0].executions
    for number in range(1, CI_ROUND_TRIPS + 1):
        assert ClaudeExecutionCounter().count(rig.events(Arm.DP, number)) == live_dp.outcomes[number].executions


def test_the_replay_reproduces_the_live_reports_byte_for_byte_with_no_execution(live_dp: LiveGate) -> None:
    rig = live_dp.rig
    replay = rig.host.run_gate(rig.tree, Arm.DP, rig.record, GateMode.REPLAY)
    assert replay.status is GateStatus.COMPLETE
    assert stable_json(replay.report) == stable_json(live_dp.final.report)
    assert stable_json(replay.local_report) == stable_json(live_dp.final.local_report)
    assert (replay.executions, replay.tickets, replay.session_id, replay.ci_rounds) == (0, (), None, CI_ROUND_TRIPS)
    assert len(rig.session.specs) == 1 + CI_ROUND_TRIPS


def test_a_replay_that_misses_an_archived_round_pauses_for_it_and_calls_no_model(
    live_dp: LiveGate, tmp_path: Path
) -> None:
    rig = live_dp.rig
    partial_record = tmp_path / "partial-rec"
    shutil.copytree(rig.record, partial_record)
    shutil.rmtree(partial_record / "ci" / str(CI_ROUND_TRIPS))
    specs = len(rig.session.specs)
    replay = rig.host.run_gate(rig.tree, Arm.DP, partial_record, GateMode.REPLAY)
    assert (replay.status, replay.pause, replay.report) == (GateStatus.PAUSED, GatePause.CI_ROUND_PENDING, None)
    assert replay.ci_rounds == CI_ROUND_TRIPS - 1
    assert replay.executions == 0
    assert len(rig.session.specs) == specs


def test_the_stored_report_names_no_machine_path(live_dp: LiveGate) -> None:
    for report in (live_dp.final.report, live_dp.final.local_report):
        serialized = stable_json(report)
        assert str(live_dp.rig.root) not in serialized
    assert PORTABLE_ROOT in stable_json(live_dp.final.report)


# --- one CI identity per tree, whatever the arm ----------------------------------------------------------------


def test_dp_and_all_on_one_tree_share_the_agent_commit_the_seed_and_every_ci_issued_request(
    live_dp: LiveGate, live_all: LiveGate
) -> None:
    assert git(live_dp.settle_tree, "rev-parse", "HEAD~1") == git(live_all.settle_tree, "rev-parse", "HEAD~1")
    assert git(live_dp.settle_tree, "rev-parse", "HEAD") != git(live_all.settle_tree, "rev-parse", "HEAD")
    dp_seed, all_seed = seed_record_of(live_dp), seed_record_of(live_all)
    assert dp_seed["seed"] == all_seed["seed"]
    assert dp_seed["firstRunId"] == all_seed["firstRunId"]
    for number in range(1, CI_ROUND_TRIPS + 1):
        assert archived_response_ids(live_dp, number) == archived_response_ids(live_all, number)
    assert live_dp.digest_directory.name == live_all.digest_directory.name


def test_the_composition_check_matches_all_carries_the_dp_statements_dp_alone_earned(
    live_dp: LiveGate, live_all: LiveGate
) -> None:
    def dp_statements(gate: LiveGate) -> set[tuple[str, str]]:
        return {
            (str(item["subject"]), str(item["state"]))
            for item in consumed(gate.final.report)
            if "discriminating-power" in str(item["predicateType"])
        }

    assert dp_statements(live_dp) == dp_statements(live_all)
    assert dp_statements(live_dp)


# --- ALL: every module, signed ---------------------------------------------------------------------------------


def test_a_live_gate_of_the_all_arm_is_complete_and_signed_with_the_local_readout_beside_it(live_all: LiveGate) -> None:
    final = live_all.final
    assert final.status is GateStatus.COMPLETE
    assert final.session_id is None
    assert mapping(final.report, "report")["schema_version"] == "1.2"
    assert trust_of(final.report) == "signed"
    assert trust_of(final.local_report) == "local"
    assert findings_by_requirement(final.local_report) == findings_by_requirement(final.report)
    assert live_all.outcomes[0].session_id == SESSION_ID


def test_every_statement_of_the_three_modules_reached_the_signed_gate_as_valid(live_all: LiveGate) -> None:
    assert {item["state"] for item in consumed(live_all.final.report)} == {"valid"}
    assert proof_kinds(live_all.final.report) == {"alignment", "completeness", "discriminating-power"}


def test_each_ticket_carries_its_module_its_request_kind_and_its_cost(live_all: LiveGate) -> None:
    tickets = [ticket for outcome in live_all.outcomes for ticket in outcome.tickets]
    kinds = {(ticket.module, ticket.kind) for ticket in tickets}
    assert (Module.DISCRIMINATING_POWER, RequestKind.MUTANTS) in kinds
    assert (Module.ALIGNMENT, RequestKind.ALIGNMENT) in kinds
    assert (Module.COMPLETENESS, RequestKind.COMPLETENESS) in kinds
    assert len(tickets) == live_all.rig.session.answered
    assert all(ticket.tokens.input_tokens == 60 and ticket.tokens.output_tokens == 40 for ticket in tickets)
    assert {ticket.resolved_model for ticket in tickets} == {"claude-sonnet-5-5"}


def test_the_session_ran_in_a_copy_of_the_tree_with_the_plugins_of_every_module(live_all: LiveGate) -> None:
    rig = live_all.rig
    spec = rig.session.specs[0]
    assert spec.cwd != rig.tree
    plugins = [spec.argv[i + 1] for i, word in enumerate(spec.argv) if word == "--plugin-dir"]
    assert [Path(p).parts[-3] for p in plugins[1:]] == ["discpower", "alignment", "completeness"]
    assert spec.env["CLAUDE_CODE_OAUTH_TOKEN"] == "test-token"
    assert (rig.tree / "AGENTS.md").exists() is False


def test_a_replay_starts_from_a_fresh_area_and_leaves_the_session_area_alone(live_all: LiveGate) -> None:
    rig = live_all.rig
    marker = live_all.digest_directory / "session" / "ready"
    before = marker.stat().st_mtime_ns
    rig.host.run_gate(rig.tree, Arm.ALL, rig.record, GateMode.REPLAY)
    assert marker.stat().st_mtime_ns == before
    assert (live_all.settle_tree / "shallnot.all.yaml").is_file()
    assert (live_all.settle_tree / "shallnot.all.local.yaml").is_file()


def test_the_all_replay_reproduces_both_reports_with_no_execution(live_all: LiveGate) -> None:
    rig = live_all.rig
    replay = rig.host.run_gate(rig.tree, Arm.ALL, rig.record, GateMode.REPLAY)
    assert stable_json(replay.report) == stable_json(live_all.final.report)
    assert stable_json(replay.local_report) == stable_json(live_all.final.local_report)
    assert (replay.executions, replay.tickets) == (0, ())


# --- an ALIGNED project clears locally and escalates under signed trust ---------------------------------------


def test_an_aligned_project_clears_in_the_local_readout_and_escalates_as_uncorroborated_under_signed_trust(
    tmp_path: Path, fakehost: Path, signing: SigningMaterial
) -> None:
    rig = credited_inline_rig(tmp_path, fakehost, signing)
    [outcome] = drive(rig, Arm.AL)
    assert outcome.status is GateStatus.COMPLETE
    assert findings_by_requirement(outcome.local_report) == {}
    assert mapping(outcome.local_report, "local")["verdict"] == "pass"
    assert mapping(outcome.report, "signed")["verdict"] == "fail"
    messages = finding_messages(outcome.report)
    assert messages
    assert all(UNCORROBORATED in message for message in messages)


def test_with_the_discriminating_power_proof_beside_it_alignment_is_corroborated_where_the_matrix_backs_it(
    live_all: LiveGate,
) -> None:
    signed = " ".join(finding_messages(live_all.final.report))
    assert "judged UNDETERMINED" not in signed
    assert trust_of(live_all.final.report) == "signed"
    assert trust_of(live_all.final.local_report) == "local"


# --- failures of the session, the room and the rounds ---------------------------------------------------------


def test_a_missing_plugin_in_system_init_aborts_the_gate_as_misconfigured(
    tmp_path: Path,
    fakehost: Path,
    signing: SigningMaterial,
) -> None:
    session = FakeGateSession(fakehost, BINARIES, PARCEL_ANSWERS, omit_plugin="spec-proof-alignment")
    rig = build_rig(tmp_path, fakehost, signing, session=session)
    outcome = rig.host.run_gate(rig.tree, Arm.AL, rig.record, GateMode.LIVE)
    assert outcome.status is GateStatus.HARNESS_MISCONFIGURED
    assert outcome.report is None
    assert session.answered == 0
    assert outcome.executions == 1


def test_a_gate_killed_mid_session_resumes_from_its_pending_tickets_in_the_same_room(
    tmp_path: Path,
    fakehost: Path,
    signing: SigningMaterial,
    live_dp: LiveGate,
) -> None:
    killed = FakeGateSession(fakehost, BINARIES, PARCEL_ANSWERS, kill_after_waves=1)
    first = build_rig(tmp_path / "killed", fakehost, signing, session=killed)
    with pytest.raises(SessionKilled):
        first.host.run_gate(first.tree, Arm.DP, first.record, GateMode.LIVE)
    answered_in_the_store = list((killed.specs[0].cwd.parent / "state").glob("spec-proof/discpower/*/host/responses/*"))
    assert answered_in_the_store

    resumed = FakeGateSession(fakehost, BINARIES, PARCEL_ANSWERS)
    second = build_rig(tmp_path / "killed", fakehost, signing, session=resumed, tree=first.tree)
    outcome = second.host.run_gate(first.tree, Arm.DP, first.record, GateMode.LIVE)

    assert (outcome.status, outcome.pause) == (GateStatus.PAUSED, GatePause.CI_ROUND_PENDING)
    assert resumed.answered > 0
    assert resumed.specs[0].cwd == killed.specs[0].cwd
    assert len(first.events(Arm.DP)) > killed.answered
    reference = sorted(path.name for path in live_dp.rig.record.glob("*.json"))
    assert sorted(path.name for path in first.record.glob("*.json")) == reference


def test_a_round_killed_mid_session_resumes_in_the_same_session_and_archives_the_round_once(
    tmp_path: Path,
    fakehost: Path,
    signing: SigningMaterial,
) -> None:
    killed = FakeGateSession(fakehost, BINARIES, PARCEL_ANSWERS, kill_round_after=1)
    rig = build_rig(tmp_path, fakehost, signing, session=killed)
    assert rig.host.run_gate(rig.tree, Arm.DP, rig.record, GateMode.LIVE).pause is GatePause.CI_ROUND_PENDING
    with pytest.raises(SessionKilled):
        rig.host.run_gate(rig.tree, Arm.DP, rig.record, GateMode.LIVE)
    assert CiRoundArchive(rig.record).rounds() == 0

    again = rig.host.run_gate(rig.tree, Arm.DP, rig.record, GateMode.LIVE)
    assert (again.status, again.pause, again.ci_rounds) == (GateStatus.PAUSED, GatePause.CI_ROUND_PENDING, 1)
    assert CiRoundArchive(rig.record).rounds() == 1
    sessions = [spec for spec in killed.specs if "--resume" in spec.argv]
    assert len(sessions) == 2


def test_a_dirty_tracked_session_tree_is_misconfigured_before_any_round_runs(
    tmp_path: Path,
    fakehost: Path,
    signing: SigningMaterial,
) -> None:
    rig = build_rig(tmp_path, fakehost, signing)
    first = rig.host.run_gate(rig.tree, Arm.DP, rig.record, GateMode.LIVE)
    assert first.pause is GatePause.CI_ROUND_PENDING
    session_tree = rig.session.specs[0].cwd
    (session_tree / "shallnot.yaml").write_text("tampered: true\n", encoding="utf-8")
    sessions = len(rig.session.specs)
    outcome = rig.host.run_gate(rig.tree, Arm.DP, rig.record, GateMode.LIVE)
    assert outcome.status is GateStatus.HARNESS_MISCONFIGURED
    assert len(rig.session.specs) == sessions
    assert CiRoundArchive(rig.record).rounds() == 0


def test_a_stale_ci_issued_directory_never_triggers_a_round(
    tmp_path: Path,
    fakehost: Path,
    signing: SigningMaterial,
) -> None:
    rig = build_rig(tmp_path, fakehost, signing)
    stale = rig.tree / ".spec-proof" / "ci-issued" / "alignment"
    stale.mkdir(parents=True)
    (stale / "issued.json").write_text("{}", encoding="utf-8")
    outcomes = drive(rig, Arm.AL)
    assert [(o.status, o.ci_rounds) for o in outcomes] == [(GateStatus.COMPLETE, 0)]
    assert len(rig.session.specs) == 1


def test_a_ticket_still_pending_after_the_round_trips_leaves_the_criteria_unproven_and_escalates(
    tmp_path: Path,
    fakehost: Path,
    signing: SigningMaterial,
) -> None:
    silent = FakeGateSession(fakehost, BINARIES, PARCEL_ANSWERS, answer_rounds=False)
    rig = build_rig(tmp_path, fakehost, signing, session=silent)
    outcomes = drive(rig, Arm.DP)
    final = outcomes[-1]
    assert final.status is GateStatus.COMPLETE
    assert final.ci_rounds == CI_ROUND_TRIPS
    assert len(outcomes) == CI_ROUND_TRIPS + 2
    assert consumed(final.report) == []
    assert findings_by_requirement(final.report) == {
        "PAR-1": {"stale_proof"},
        "PAR-2": {"stale_proof"},
        "PAR-3": {"stale_proof"},
    }
    assert mapping(final.report, "report")["verdict"] != "pass"


def test_an_arm_with_no_proof_module_is_gated_without_a_session(
    tmp_path: Path,
    fakehost: Path,
    signing: SigningMaterial,
) -> None:
    rig = build_rig(tmp_path, fakehost, signing)
    outcome = rig.host.run_gate(rig.tree, Arm.TR, rig.record, GateMode.LIVE)
    assert outcome.status is GateStatus.COMPLETE
    assert (outcome.executions, outcome.tickets, outcome.local_report, rig.session.specs) == (0, (), None, [])
    assert mapping(outcome.report, "report")["schema_version"] == "1.2"
