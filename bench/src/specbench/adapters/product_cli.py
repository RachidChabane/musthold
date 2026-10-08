"""Typed wrappers around the product binaries (`spec-proof-dp`, `-alignment`, `-completeness`, `-ledger`, `-verify`).

Every method builds its argv with a pure function of this module and runs it through the process runner; the exit
codes a command may end with are named, and any other exit is a `ProductCommandFailed`.
"""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.application.ports import ProcessResult, ProcessRunner
from specbench.domain.product import LEDGER_BINARY, VERIFY_BINARY, ProofModule

EXIT_CLEAN: Final = 0
EXIT_REFUSED: Final = 1
EXIT_FAILURE: Final = 2
EXIT_AWAITING_HOST: Final = 3
PROVEN_EXITS: Final = frozenset({EXIT_CLEAN, EXIT_REFUSED})
"""A `prove` that finished: clean, or blocked or escalated. Exit 3 waits for the host and exit 2 is a tool failure."""
PROVED_OR_AWAITING: Final = PROVEN_EXITS | {EXIT_AWAITING_HOST}
HOOKS_NONE: Final = "none"
ED25519_IDENTITY: Final = "ed25519"
LOCAL_CACHE_DIRECTORY: Final = ".spec-proof/cache"
VERIFIED_DIRECTORY: Final = ".spec-proof/verified"


class ProductCommandFailed(RuntimeError):
    """A product command ended with an exit code its caller does not accept."""

    def __init__(self, argv: list[str], result: ProcessResult) -> None:
        super().__init__(
            f"{' '.join(argv)} exited {result.returncode}: {(result.stderr or result.stdout).strip()[:500]}"
        )
        self.returncode = result.returncode


@dataclass(frozen=True)
class ProveRequest:
    """The flags of one `prove` run: recording, replaying, resuming, or the CI form with a pushed host bundle."""

    record: Path | None = None
    replay: Path | None = None
    resume: bool = False
    ci: bool = False
    import_host: Path | None = None

    def __post_init__(self) -> None:
        if self.record is not None and self.replay is not None:
            raise ValueError("--record and --replay exclude each other")
        if self.ci and (self.record is not None or self.replay is not None):
            raise ValueError("--ci excludes --record and --replay")
        if self.import_host is not None and not self.ci:
            raise ValueError("--import-host needs --ci")


def init_argv(binary: Path, tree: Path) -> list[str]:
    return [str(binary), "init", "--dir", str(tree), "--hooks", HOOKS_NONE]


def enable_argv(binary: Path, tree: Path) -> list[str]:
    return [str(binary), "enable", "--dir", str(tree)]


def prove_argv(binary: Path, tree: Path, request: ProveRequest) -> list[str]:
    argv = [str(binary), "prove", "--changed", "--root", str(tree)]
    if request.resume:
        argv.append("--resume")
    if request.record is not None:
        argv += ["--record", str(request.record)]
    if request.replay is not None:
        argv += ["--replay", str(request.replay)]
    if request.ci:
        argv.append("--ci")
    if request.import_host is not None:
        argv += ["--import-host", str(request.import_host)]
    return argv


def host_push_argv(binary: Path, tree: Path) -> list[str]:
    return [str(binary), "host", "push", "--root", str(tree)]


def sign_argv(ledger: Path, statements: Path, destination: Path, key_file: Path) -> list[str]:
    return [
        str(ledger),
        "sign",
        "--identity",
        ED25519_IDENTITY,
        "--in",
        str(statements),
        "--out",
        str(destination),
        "--key-file",
        str(key_file),
    ]


def ledger_append_argv(ledger: Path, signed: Path, repository: Path, head_sha: str, run_id: str) -> list[str]:
    return [
        str(ledger),
        "ledger",
        "append",
        "--signed",
        str(signed),
        "--head-sha",
        head_sha,
        "--run-id",
        run_id,
        "--repo",
        str(repository),
    ]


def ledger_export_argv(ledger: Path, destination: Path, repository: Path) -> list[str]:
    return [str(ledger), "ledger", "export", "--to", str(destination), "--repo", str(repository)]


def verify_offline_argv(verifier: Path, bundles: Path, destination: Path) -> list[str]:
    return [str(verifier), "--bundles", str(bundles), "--offline", "--out", str(destination)]


def verify_local_argv(verifier: Path, cache: Path, destination: Path) -> list[str]:
    return [str(verifier), "--accept-local", str(cache), "--out", str(destination)]


class ProductCli:
    """The product's command line, run in a project tree with the environment of a clean-room area."""

    def __init__(self, runner: ProcessRunner, binaries: Path) -> None:
        self._runner = runner
        self._binaries = binaries

    def binary(self, name: str) -> Path:
        return self._binaries / name

    def init(self, module: ProofModule, tree: Path, env: Mapping[str, str]) -> None:
        self._run(init_argv(self.binary(module.binary), tree), tree, env, {EXIT_CLEAN})

    def enable(self, module: ProofModule, tree: Path, env: Mapping[str, str]) -> None:
        self._run(enable_argv(self.binary(module.binary), tree), tree, env, {EXIT_CLEAN})

    def prove(self, module: ProofModule, tree: Path, env: Mapping[str, str], request: ProveRequest) -> int:
        """The exit code of a `prove` that finished or waits for the host."""
        argv = prove_argv(self.binary(module.binary), tree, request)
        return self._run(argv, tree, env, PROVED_OR_AWAITING).returncode

    def host_push(self, module: ProofModule, tree: Path, env: Mapping[str, str]) -> bool:
        """Whether the module had answers to publish: `host push` exits 1 when nothing is answered."""
        result = self._run(host_push_argv(self.binary(module.binary), tree), tree, env, {EXIT_CLEAN, EXIT_REFUSED})
        return result.returncode == EXIT_CLEAN

    def sign(self, statements: Path, destination: Path, key_file: Path, tree: Path, env: Mapping[str, str]) -> None:
        self._run(sign_argv(self.binary(LEDGER_BINARY), statements, destination, key_file), tree, env, {EXIT_CLEAN})

    def ledger_append(self, signed: Path, repository: Path, head_sha: str, run_id: str, env: Mapping[str, str]) -> None:
        argv = ledger_append_argv(self.binary(LEDGER_BINARY), signed, repository, head_sha, run_id)
        self._run(argv, repository, env, {EXIT_CLEAN})

    def ledger_export(self, destination: Path, repository: Path, env: Mapping[str, str]) -> None:
        self._run(
            ledger_export_argv(self.binary(LEDGER_BINARY), destination, repository), repository, env, {EXIT_CLEAN}
        )

    def verify_offline(self, bundles: Path, destination: Path, tree: Path, env: Mapping[str, str]) -> None:
        self._run(verify_offline_argv(self.binary(VERIFY_BINARY), bundles, destination), tree, env, {EXIT_CLEAN})

    def verify_local(self, cache: Path, destination: Path, tree: Path, env: Mapping[str, str]) -> None:
        self._run(verify_local_argv(self.binary(VERIFY_BINARY), cache, destination), tree, env, {EXIT_CLEAN})

    def _run(
        self, argv: list[str], cwd: Path, env: Mapping[str, str], accepted: set[int] | frozenset[int]
    ) -> ProcessResult:
        result = self._runner.run(argv, cwd, env)
        if result.returncode not in accepted:
            raise ProductCommandFailed(argv, result)
        return result
