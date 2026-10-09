"""Wires an `IsolatedCliHost` and the collaborators of its settle step and CI round trips from the machine's pieces."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path

from specbench.adapters.area_environment import AreaEnvironment
from specbench.adapters.area_preparer import AreaPreparer
from specbench.adapters.clean_room_repository import CleanRoomRepository
from specbench.adapters.clean_room_settlement import CleanRoomSettlement
from specbench.adapters.gate_session_driver import GateSessionDriver
from specbench.adapters.isolated_ci_round_trip import IsolatedCiRoundTrip
from specbench.adapters.isolated_cli_host import IsolatedCliHost
from specbench.adapters.isolated_host_config import IsolatedHostConfig
from specbench.adapters.product_cli import ProductCli
from specbench.adapters.proof_publication import LocalPublication, SignedPublication
from specbench.adapters.shallnot_cli import ShallnotCli
from specbench.application.ports import (
    ClaudeCredentialSource,
    ExecutionCounter,
    GateSessionRunner,
    ProcessRunner,
    SchemaChecker,
    TreeReader,
)
from specbench.domain.manifest import RunLayout


@dataclass(frozen=True)
class SigningMaterial:
    """The benchmark's Ed25519 key and the trust policy that pins its public half."""

    key_file: Path
    trust_policy: Path


def build_isolated_host(
    config: IsolatedHostConfig,
    layout: RunLayout,
    runner: ProcessRunner,
    checker: SchemaChecker,
    sessions: GateSessionRunner,
    signing: SigningMaterial,
    tree_reader: TreeReader,
    counter: ExecutionCounter,
    credentials: ClaudeCredentialSource,
    base_environment: Mapping[str, str],
) -> IsolatedCliHost:
    cli = ProductCli(runner, config.binaries)
    shallnot = ShallnotCli(runner, config.shallnot, checker)
    repository = CleanRoomRepository(runner)
    environment = AreaEnvironment(config.binaries, config.tool_path, base_environment)
    preparer = AreaPreparer(cli, shallnot, repository, environment)
    driver = GateSessionDriver(sessions)
    settlement = CleanRoomSettlement(
        cli,
        shallnot,
        preparer,
        repository,
        environment,
        LocalPublication(cli),
        SignedPublication(cli, signing.key_file, signing.trust_policy),
    )
    round_trip = IsolatedCiRoundTrip(
        config, layout, driver, preparer, environment, cli, repository, counter, credentials, base_environment
    )
    return IsolatedCliHost(
        config,
        layout,
        driver,
        preparer,
        settlement,
        round_trip,
        tree_reader,
        counter,
        environment,
        credentials,
        base_environment,
    )
