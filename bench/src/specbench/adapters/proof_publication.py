"""How the proofs of a clean room reach Shallnot's `verified` directory (ADR 0004, "Clean-room gate").

- `SignedPublication` is the CI of a gate. It proves each module with `prove --ci --import-host` on the host bundles it
  is given, reports the modules that still wait for the host, and once none does, signs the statements CI produced under
  the benchmark Ed25519 key, appends them to the ledger of the clean room, exports it and verifies it offline against the
  benchmark trust policy. Shallnot then reads them under `proofs.trust: signed`, the trust every user merges under.
- `LocalPublication` publishes the statements the modules wrote in the replay as unsigned local statements, the way the
  `prove-all` skill does. It produces the developer-loop readout kept beside the signed verdict, never the verdict.
"""

from __future__ import annotations

import shutil
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.adapters.product_cli import (
    EXIT_AWAITING_HOST,
    LOCAL_CACHE_DIRECTORY,
    VERIFIED_DIRECTORY,
    ProductCli,
    ProveRequest,
)
from specbench.domain.ci_seed import CiIdentity
from specbench.domain.clean_room import RoomArea
from specbench.domain.product import ProofModule
from specbench.domain.shallnot_config import ProofTrust

CI_RUN_ID_ENV: Final = "SPEC_PROOF_CI_RUN_ID"
TRUST_POLICY_PATH: Final = ".spec-proof/trust-policy.yaml"
PENDING_DIRECTORY: Final = ".spec-proof/pending"
CI_ISSUED_DIRECTORY: Final = ".spec-proof/ci-issued"
SIGNED_DIRECTORY: Final = "signed"
LEDGER_BUNDLES_DIRECTORY: Final = "bundles"


@dataclass(frozen=True)
class PublicationContext:
    """The settle area whose statements are published, the modules that proved, and the CI identity of its tree."""

    area: RoomArea
    env: Mapping[str, str]
    modules: Sequence[ProofModule]
    identity: CiIdentity


class LocalPublication:
    trust = ProofTrust.LOCAL

    def __init__(self, cli: ProductCli) -> None:
        self._cli = cli

    def publish(self, context: PublicationContext) -> None:
        tree = context.area.tree
        cache = tree / LOCAL_CACHE_DIRECTORY
        cache.mkdir(parents=True, exist_ok=True)
        self._cli.verify_local(cache, tree / VERIFIED_DIRECTORY, tree, context.env)


class SignedPublication:
    trust = ProofTrust.SIGNED

    def __init__(self, cli: ProductCli, signing_key: Path, trust_policy: Path) -> None:
        self._cli = cli
        self._signing_key = signing_key
        self._trust_policy = trust_policy

    def prove_in_ci(self, context: PublicationContext, bundles: Mapping[str, Path]) -> tuple[ProofModule, ...]:
        """Runs `prove --ci --import-host` for each module over its bundle and returns the modules left waiting.

        The statements, the pending files and the requests of an earlier run are cleared first, so the run depends on
        the tree and the bundles alone. A module is pending exactly when it exits 3.
        """
        tree = context.area.tree
        for stale in (CI_ISSUED_DIRECTORY, LOCAL_CACHE_DIRECTORY, PENDING_DIRECTORY, VERIFIED_DIRECTORY):
            shutil.rmtree(tree / stale, ignore_errors=True)
        ci_env = {**context.env, CI_RUN_ID_ENV: context.identity.run_id}
        waiting: list[ProofModule] = []
        for module in context.modules:
            request = ProveRequest(ci=True, import_host=bundles[module.host_ref_name])
            if self._cli.prove(module, tree, ci_env, request) == EXIT_AWAITING_HOST:
                waiting.append(module)
        return tuple(waiting)

    def sign_and_verify(self, context: PublicationContext) -> None:
        """Signs the statements of the last CI proofs and publishes the ones the offline verification accepts."""
        tree = context.area.tree
        cache = tree / LOCAL_CACHE_DIRECTORY
        if not any(path.is_file() for path in cache.rglob("*")):
            return
        signed = context.area.root / SIGNED_DIRECTORY
        self._cli.sign(cache, signed, self._signing_key, tree, context.env)
        self._cli.ledger_append(signed, tree, context.identity.head, context.identity.run_id, context.env)
        exported = context.area.root / LEDGER_BUNDLES_DIRECTORY
        self._cli.ledger_export(exported, tree, context.env)
        shutil.copyfile(self._trust_policy, tree / TRUST_POLICY_PATH)
        self._cli.verify_offline(exported, tree / VERIFIED_DIRECTORY, tree, context.env)
