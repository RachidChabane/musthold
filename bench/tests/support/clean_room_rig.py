"""A real `IsolatedCliHost` over the product's binaries with a fake gate-host session, and the readers of its reports.

Nothing here starts `claude` or `codex`: the fake session plays `prove-all` and the CI round trips with the product's
fake host. `drive` calls the host the way the pacing decorator does, once per entry, until the gate stops asking for
another CI round trip.
"""

from __future__ import annotations

import os
import subprocess
from collections import defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.adapters.execution_counter import ClaudeExecutionCounter
from specbench.adapters.isolated_cli_host import IsolatedCliHost
from specbench.adapters.isolated_host_config import IsolatedHostConfig
from specbench.adapters.isolated_host_factory import SigningMaterial, build_isolated_host
from specbench.adapters.process import SubprocessRunner
from specbench.adapters.schemas import JsonSchemaChecker
from specbench.adapters.workspace_repo import DiskTreeReader
from specbench.domain.arms import Arm
from specbench.domain.canonical import JsonValue
from specbench.domain.ci_seed import CI_ROUND_TRIPS
from specbench.domain.gate import GateMode, GateOutcome, GatePause
from specbench.domain.jsonread import mapping, sequence, text
from specbench.domain.manifest import CALL_EVENTS, RunLayout
from specbench.domain.sessions import ci_round_call_id, live_gate_call_id
from support.credentials import StaticCredentialSource
from support.product_world import (
    BINARIES,
    PARCEL_ANSWERS,
    REPOSITORY,
    UMBRELLA_PLUGIN,
    FakeGateSession,
    make_parcel_tree,
)

BENCH_TOKEN: Final = "test-token"


def make_signing_material(directory: Path) -> SigningMaterial:
    """The benchmark key and trust policy of a test: a fresh Ed25519 key, owner-readable only."""
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
    return SigningMaterial(key, policy)


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

    def events(self, arm: Arm, round_number: int = 0) -> list[str]:
        call_id = (
            live_gate_call_id(arm, self.record)
            if round_number == 0
            else ci_round_call_id(arm, self.record, round_number)
        )
        return (self.layout.call(call_id) / CALL_EVENTS).read_text().splitlines()


def build_rig(
    root: Path,
    fakehost: Path,
    signing: SigningMaterial,
    session: FakeGateSession | None = None,
    tree: Path | None = None,
    answers: Path = PARCEL_ANSWERS,
) -> Rig:
    played = session or FakeGateSession(fakehost, BINARIES, answers)
    config = IsolatedHostConfig(REPOSITORY, BINARIES, UMBRELLA_PLUGIN, root / "rooms")
    host = build_isolated_host(
        config,
        RunLayout(root / "run"),
        SubprocessRunner(),
        JsonSchemaChecker(),
        played,
        signing,
        DiskTreeReader(),
        ClaudeExecutionCounter(),
        StaticCredentialSource(BENCH_TOKEN),
        os.environ,
    )
    return Rig(root, host, played, tree or make_parcel_tree(root / "tree"))


def drive(rig: Rig, arm: Arm) -> list[GateOutcome]:
    """The entries of a live gate: the host is called again for as long as it pauses for another CI round trip."""
    outcomes: list[GateOutcome] = []
    for _ in range(CI_ROUND_TRIPS + 2):
        outcomes.append(rig.host.run_gate(rig.tree, arm, rig.record, GateMode.LIVE))
        if outcomes[-1].pause is not GatePause.CI_ROUND_PENDING:
            break
    return outcomes


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


def trust_of(report: JsonValue) -> str:
    proofs = mapping(mapping(mapping(report, "report").get("extensions"), "extensions").get("proofs"), "proofs")
    return text(proofs.get("trust"), "trust")


def finding_messages(report: JsonValue) -> list[str]:
    return [
        text(mapping(f, "finding").get("message"), "message")
        for f in sequence(mapping(report, "report").get("findings"), "findings")
    ]
