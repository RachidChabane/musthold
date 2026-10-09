from __future__ import annotations

from collections.abc import Mapping, Sequence
from pathlib import Path

import pytest

from specbench.adapters.product_cli import (
    ProductCli,
    ProductCommandFailed,
    ProveRequest,
    enable_argv,
    host_pull_argv,
    host_push_argv,
    init_argv,
    ledger_append_argv,
    ledger_export_argv,
    prove_argv,
    sign_argv,
    verify_local_argv,
    verify_offline_argv,
)
from specbench.application.ports import ProcessResult
from specbench.domain.product import PROOF_MODULES

BIN = Path("/tools/bin")
TREE = Path("/room/tree")
DP = PROOF_MODULES[0]


class ScriptedRunner:
    def __init__(self, returncode: int) -> None:
        self.returncode = returncode
        self.calls: list[tuple[list[str], Path | None, Mapping[str, str] | None]] = []

    def run(self, argv: Sequence[str], cwd: Path | None = None, env: Mapping[str, str] | None = None) -> ProcessResult:
        self.calls.append((list(argv), cwd, env))
        return ProcessResult(self.returncode, "out", "err")


def test_init_installs_no_hooks_and_enable_names_the_directory() -> None:
    assert init_argv(BIN / "spec-proof-dp", TREE) == [
        "/tools/bin/spec-proof-dp",
        "init",
        "--dir",
        "/room/tree",
        "--hooks",
        "none",
    ]
    assert enable_argv(BIN / "spec-proof-dp", TREE) == ["/tools/bin/spec-proof-dp", "enable", "--dir", "/room/tree"]


def test_a_recording_prove_names_the_record_directory() -> None:
    argv = prove_argv(BIN / "spec-proof-dp", TREE, ProveRequest(record=Path("/rec")))
    assert argv == ["/tools/bin/spec-proof-dp", "prove", "--changed", "--root", "/room/tree", "--record", "/rec"]


def test_a_replaying_prove_names_the_recordings_and_never_records() -> None:
    argv = prove_argv(BIN / "spec-proof-dp", TREE, ProveRequest(replay=Path("/rec")))
    assert argv[-2:] == ["--replay", "/rec"]
    assert "--record" not in argv


def test_a_resumed_ci_prove_imports_the_host_bundle() -> None:
    argv = prove_argv(BIN / "spec-proof-dp", TREE, ProveRequest(ci=True, import_host=Path("/host/discpower")))
    assert argv[-3:] == ["--ci", "--import-host", "/host/discpower"]
    assert "--resume" in prove_argv(BIN / "spec-proof-dp", TREE, ProveRequest(resume=True))


@pytest.mark.parametrize(
    "request_kwargs",
    [
        {"record": Path("/a"), "replay": Path("/b")},
        {"ci": True, "record": Path("/a")},
        {"ci": True, "replay": Path("/a")},
        {"import_host": Path("/a")},
    ],
)
def test_flags_the_product_refuses_together_are_refused_before_a_process_starts(
    request_kwargs: dict[str, object],
) -> None:
    with pytest.raises(ValueError, match=r"exclude|needs"):
        ProveRequest(**request_kwargs)  # type: ignore[arg-type]


def test_the_ledger_and_the_verifier_are_called_with_the_documented_flags() -> None:
    ledger, verify = BIN / "spec-proof-ledger", BIN / "spec-proof-verify"
    assert sign_argv(ledger, Path("/c"), Path("/s"), Path("/k.pem")) == [
        "/tools/bin/spec-proof-ledger", "sign", "--identity", "ed25519", "--in", "/c", "--out", "/s", "--key-file", "/k.pem",
    ]  # fmt: skip
    assert ledger_append_argv(ledger, Path("/s"), Path("/r"), "abc", "run-1") == [
        "/tools/bin/spec-proof-ledger", "ledger", "append", "--signed", "/s", "--head-sha", "abc", "--run-id", "run-1", "--repo", "/r",
    ]  # fmt: skip
    assert ledger_export_argv(ledger, Path("/b"), Path("/r")) == [
        "/tools/bin/spec-proof-ledger", "ledger", "export", "--to", "/b", "--repo", "/r",
    ]  # fmt: skip
    assert verify_offline_argv(verify, Path("/b"), Path("/v")) == [
        "/tools/bin/spec-proof-verify", "--bundles", "/b", "--offline", "--out", "/v",
    ]  # fmt: skip
    assert verify_local_argv(verify, Path("/c"), Path("/v")) == [
        "/tools/bin/spec-proof-verify", "--accept-local", "/c", "--out", "/v",
    ]  # fmt: skip


def test_host_push_names_the_root() -> None:
    assert host_push_argv(BIN / "spec-proof-dp", TREE) == [
        "/tools/bin/spec-proof-dp",
        "host",
        "push",
        "--root",
        "/room/tree",
    ]


def test_host_pull_names_the_ci_run_and_the_root_but_no_repository() -> None:
    assert host_pull_argv(BIN / "spec-proof-dp", TREE, "bench-0123456789abcdef") == [
        "/tools/bin/spec-proof-dp",
        "host",
        "pull",
        "--ci-run",
        "bench-0123456789abcdef",
        "--root",
        "/room/tree",
    ]


def test_a_pull_runs_in_the_tree_with_the_environment_it_is_given() -> None:
    runner = ScriptedRunner(0)
    ProductCli(runner, BIN).host_pull(DP, TREE, {"PATH": "/shim"}, "bench-x")
    [(argv, cwd, env)] = runner.calls
    assert argv[1:3] == ["host", "pull"]
    assert (cwd, env) == (TREE, {"PATH": "/shim"})


def test_prove_reports_the_exit_of_a_run_that_waits_for_the_host() -> None:
    runner = ScriptedRunner(3)
    assert ProductCli(runner, BIN).prove(DP, TREE, {"A": "b"}, ProveRequest(replay=Path("/rec"))) == 3
    assert runner.calls[0][1:] == (TREE, {"A": "b"})


def test_a_tool_failure_of_prove_is_an_error_that_names_the_command() -> None:
    with pytest.raises(ProductCommandFailed, match=r"spec-proof-dp prove .* exited 2: err"):
        ProductCli(ScriptedRunner(2), BIN).prove(DP, TREE, {}, ProveRequest())


def test_a_refused_init_is_an_error() -> None:
    with pytest.raises(ProductCommandFailed):
        ProductCli(ScriptedRunner(1), BIN).init(DP, TREE, {})


def test_a_push_with_nothing_answered_is_not_an_error_but_says_so() -> None:
    assert ProductCli(ScriptedRunner(1), BIN).host_push(DP, TREE, {}) is False
    assert ProductCli(ScriptedRunner(0), BIN).host_push(DP, TREE, {}) is True
