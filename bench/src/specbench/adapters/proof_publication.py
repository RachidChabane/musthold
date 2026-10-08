"""How the proofs of a clean room reach Shallnot's `verified` directory (ADR 0004, "Clean-room gate").

Strategy with two implementations.

- `SignedPublication` turns the answers recorded by the modules into statements produced by CI, which is the only kind
  `spec-proof-ledger sign` accepts: it commits the tree, pushes each module's answers with `host push`, exports them
  as a CI host bundle, proves again with `prove --ci --import-host`, signs under the benchmark Ed25519 key, appends to
  the ledger of the clean room, exports it and verifies it offline against the benchmark trust policy. A request that
  CI draws afresh and no bundle answers is left pending, and the gate reports it as such.
- `LocalPublication` publishes the statements the modules wrote in the replay as unsigned local statements, the way
  the `prove-all` skill does, for a gate that runs under `proofs.trust: local`.
"""

from __future__ import annotations

import shutil
import tarfile
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Final, Protocol

from specbench.adapters.product_cli import (
    EXIT_CLEAN,
    LOCAL_CACHE_DIRECTORY,
    VERIFIED_DIRECTORY,
    ProductCli,
    ProductCommandFailed,
    ProveRequest,
)
from specbench.adapters.workspace_repo import GIT_DIRECTORY, IDENTITY
from specbench.application.ports import ProcessRunner
from specbench.domain.clean_room import RoomArea
from specbench.domain.product import ProofModule
from specbench.domain.shallnot_config import ProofTrust

GIT: Final = "git"
CI_RUN_ID_ENV: Final = "SPEC_PROOF_CI_RUN_ID"
TRUST_POLICY_PATH: Final = ".spec-proof/trust-policy.yaml"
PENDING_DIRECTORY: Final = ".spec-proof/pending"
HOST_REF_PREFIX: Final = "refs/spec-proof/host"
BENCH_BRANCH: Final = "main"
COMMIT_MESSAGE: Final = "clean room"
HOST_BUNDLES_DIRECTORY: Final = "host"
SIGNED_DIRECTORY: Final = "signed"
LEDGER_BUNDLES_DIRECTORY: Final = "bundles"
HOST_ARCHIVE_NAME: Final = "host.tar"


@dataclass(frozen=True)
class PublicationContext:
    """The settle area whose replayed statements are published, the modules that proved, and the id of the CI run."""

    area: RoomArea
    env: Mapping[str, str]
    modules: Sequence[ProofModule]
    run_id: str


class ProofPublication(Protocol):
    @property
    def trust(self) -> ProofTrust: ...

    def publish(self, context: PublicationContext) -> None: ...


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

    def __init__(self, cli: ProductCli, runner: ProcessRunner, signing_key: Path, trust_policy: Path) -> None:
        self._cli = cli
        self._runner = runner
        self._signing_key = signing_key
        self._trust_policy = trust_policy

    def publish(self, context: PublicationContext) -> None:
        tree = context.area.tree
        head = self._commit(tree)
        bundles = self._export_host_bundles(context, head)
        for stale in (tree / LOCAL_CACHE_DIRECTORY, tree / PENDING_DIRECTORY, tree / VERIFIED_DIRECTORY):
            shutil.rmtree(stale, ignore_errors=True)
        ci_env = {**context.env, CI_RUN_ID_ENV: context.run_id}
        for module in context.modules:
            self._cli.prove(module, tree, ci_env, ProveRequest(ci=True, import_host=bundles[module.host_ref_name]))
        cache = tree / LOCAL_CACHE_DIRECTORY
        if not any(path.is_file() for path in cache.rglob("*")):
            return
        signed = context.area.root / SIGNED_DIRECTORY
        self._cli.sign(cache, signed, self._signing_key, tree, context.env)
        self._cli.ledger_append(signed, tree, head, context.run_id, context.env)
        exported = context.area.root / LEDGER_BUNDLES_DIRECTORY
        self._cli.ledger_export(exported, tree, context.env)
        shutil.copyfile(self._trust_policy, tree / TRUST_POLICY_PATH)
        self._cli.verify_offline(exported, tree / VERIFIED_DIRECTORY, tree, context.env)

    def _export_host_bundles(self, context: PublicationContext, head: str) -> dict[str, Path]:
        """Each module's answers as the directory a CI job would build from the pushed ref."""
        tree = context.area.tree
        bundles: dict[str, Path] = {}
        for module in context.modules:
            bundle = context.area.root / HOST_BUNDLES_DIRECTORY / module.host_ref_name
            bundle.mkdir(parents=True, exist_ok=True)
            bundles[module.host_ref_name] = bundle
            if not self._cli.host_push(module, tree, context.env):
                continue
            archive = context.area.root / HOST_ARCHIVE_NAME
            self._git(
                tree, "archive", "--format=tar", "-o", str(archive), f"{HOST_REF_PREFIX}/{module.host_ref_name}/{head}"
            )
            with tarfile.open(archive) as opened:
                opened.extractall(bundle, filter="data")
            archive.unlink()
        return bundles

    def _commit(self, tree: Path) -> str:
        """Commits the tree under the benchmark identity, whatever history it has, and returns the head."""
        if not (tree / GIT_DIRECTORY).exists():
            self._git(tree, "init", "--initial-branch", BENCH_BRANCH)
        self._git(tree, "add", "--all")
        self._git(tree, "commit", "--allow-empty", "--message", COMMIT_MESSAGE)
        return self._git(tree, "rev-parse", "HEAD").strip()

    def _git(self, tree: Path, *arguments: str) -> str:
        argv = [GIT, "-C", str(tree), *arguments]
        result = self._runner.run(argv, tree, IDENTITY)
        if result.returncode != EXIT_CLEAN:
            raise ProductCommandFailed(argv, result)
        return result.stdout
