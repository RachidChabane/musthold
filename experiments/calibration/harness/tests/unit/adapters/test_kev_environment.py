from __future__ import annotations

import hashlib
import json
from collections.abc import Callable, Mapping, Sequence
from pathlib import Path

import pytest

from calibration.adapters.kev_environment import (
    CHECKPOINT_PATTERNS,
    DOWNLOAD_SNIPPET,
    HEAD_META_SNIPPET,
    VERSIONS_SNIPPET,
    KevEnvironmentError,
    SubprocessKevEnvironment,
)
from calibration.domain.ports import ProcessResult, ProcessTimeout

COMMIT = "f" * 40
URL = "https://example.test/kev.git"
LOCK_BYTES = b"version = 1\n"
VERSIONS = {"mlx": "0.31.0", "mlx-lm": "0.28.0", "torch": "2.9.0"}

Responder = Callable[[Sequence[str]], ProcessResult | ProcessTimeout]


def ok(stdout: str = "") -> ProcessResult:
    return ProcessResult(0, stdout.encode(), b"")


def silent(argv: Sequence[str]) -> ProcessResult | ProcessTimeout:
    return ok()


class FakeRunner:
    def __init__(self, responder: Responder | None = None) -> None:
        self.calls: list[tuple[list[str], Path | None, Mapping[str, str] | None]] = []
        self._responder: Responder = responder or silent

    def run(
        self,
        argv: Sequence[str],
        *,
        cwd: Path | None = None,
        env: Mapping[str, str] | None = None,
        unset: Sequence[str] = (),
        input_bytes: bytes | None = None,
        timeout_seconds: float | None = None,
    ) -> ProcessResult:
        self.calls.append((list(argv), cwd, env))
        outcome = self._responder(argv)
        if isinstance(outcome, ProcessTimeout):
            raise outcome
        return outcome


def kev_responder(
    head: str = COMMIT, head_meta: str = '{"base": "Qwen/Qwen3-4B", "base_revision": "abc"}'
) -> Responder:
    def respond(argv: Sequence[str]) -> ProcessResult | ProcessTimeout:
        if "rev-parse" in argv:
            return ok(head + "\n")
        if VERSIONS_SNIPPET in argv:
            return ok(json.dumps(VERSIONS) + "\n")
        if HEAD_META_SNIPPET in argv:
            return ok("Downloading...\n" + head_meta + "\n")
        return ok()

    return respond


@pytest.fixture
def checkout(tmp_path: Path) -> Path:
    return tmp_path / "kev"


def present_checkout(directory: Path) -> None:
    directory.mkdir()
    (directory / "uv.lock").write_bytes(LOCK_BYTES)


def test_setup_clones_an_absent_directory_then_pins_syncs_and_reads_versions(checkout: Path) -> None:
    def clone_creates_directory(argv: Sequence[str]) -> ProcessResult | ProcessTimeout:
        if argv[:2] == ["git", "clone"]:
            present_checkout(checkout)
        return kev_responder()(argv)

    runner = FakeRunner(clone_creates_directory)
    info = SubprocessKevEnvironment(runner, checkout, URL, COMMIT).setup()

    argvs = [call[0] for call in runner.calls]
    assert argvs[0] == ["git", "clone", URL, str(checkout)]
    assert argvs[1] == ["git", "-C", str(checkout), "checkout", "--detach", COMMIT]
    assert argvs[2] == ["uv", "sync", "--frozen", "--extra", "serve"]
    assert runner.calls[2][1] == checkout
    assert info.commit == COMMIT
    assert info.uv_lock_sha256 == hashlib.sha256(LOCK_BYTES).hexdigest()
    assert dict(info.versions) == VERSIONS


def test_setup_leaves_a_present_checkout_alone_when_it_is_at_the_commit(checkout: Path) -> None:
    present_checkout(checkout)
    runner = FakeRunner(kev_responder())
    SubprocessKevEnvironment(runner, checkout, URL, COMMIT).setup()
    argvs = [call[0] for call in runner.calls]
    assert ["git", "-C", str(checkout), "rev-parse", "HEAD"] in argvs
    assert not any(argv[:2] == ["git", "clone"] for argv in argvs)
    assert not any("checkout" in argv for argv in argvs)


def test_setup_refuses_a_present_checkout_at_another_commit(checkout: Path) -> None:
    present_checkout(checkout)
    runner = FakeRunner(kev_responder(head="0" * 40))
    with pytest.raises(KevEnvironmentError, match="not at the pinned commit"):
        SubprocessKevEnvironment(runner, checkout, URL, COMMIT).setup()
    assert not any(call[0][0] == "uv" for call in runner.calls)


def test_versions_are_read_for_mlx_mlx_lm_and_torch_inside_the_environment(checkout: Path) -> None:
    present_checkout(checkout)
    runner = FakeRunner(kev_responder())
    SubprocessKevEnvironment(runner, checkout, URL, COMMIT).setup()
    [(argv, cwd, _)] = [call for call in runner.calls if VERSIONS_SNIPPET in call[0]]
    assert argv == [
        "uv",
        "run",
        "--frozen",
        "--extra",
        "serve",
        "python",
        "-c",
        VERSIONS_SNIPPET,
        "mlx",
        "mlx-lm",
        "torch",
    ]
    assert cwd == checkout


def test_a_failing_sync_is_reported_with_its_stderr(checkout: Path) -> None:
    present_checkout(checkout)

    def respond(argv: Sequence[str]) -> ProcessResult | ProcessTimeout:
        if argv[:2] == ["uv", "sync"]:
            return ProcessResult(2, b"", b"resolution failed")
        return kev_responder()(argv)

    with pytest.raises(KevEnvironmentError, match="resolution failed"):
        SubprocessKevEnvironment(FakeRunner(respond), checkout, URL, COMMIT).setup()


def test_a_timeout_is_a_kev_environment_error(checkout: Path) -> None:
    present_checkout(checkout)
    runner = FakeRunner(lambda argv: ProcessTimeout("slow"))
    with pytest.raises(KevEnvironmentError, match="timed out"):
        SubprocessKevEnvironment(runner, checkout, URL, COMMIT).setup()


def test_head_meta_runs_the_snippet_with_repository_and_revision_as_arguments(checkout: Path) -> None:
    present_checkout(checkout)
    runner = FakeRunner(kev_responder())
    meta = SubprocessKevEnvironment(runner, checkout, URL, COMMIT).head_meta("owner/kev-4b", "r1")
    assert (meta.base, meta.base_revision) == ("Qwen/Qwen3-4B", "abc")
    [(argv, cwd, env)] = runner.calls
    assert argv == [
        "uv",
        "run",
        "--frozen",
        "--extra",
        "serve",
        "python",
        "-c",
        HEAD_META_SNIPPET,
        "owner/kev-4b",
        "r1",
    ]
    assert cwd == checkout
    assert env is None


def test_head_meta_accepts_a_null_base_revision(checkout: Path) -> None:
    runner = FakeRunner(kev_responder(head_meta='{"base": "b", "base_revision": null}'))
    meta = SubprocessKevEnvironment(runner, checkout, URL, COMMIT).head_meta("o/r", "x")
    assert meta.base_revision is None


@pytest.mark.parametrize("printed", ["", "garbage", '{"base": null, "base_revision": "x"}', "[1]", '{"base": 3}'])
def test_head_meta_refuses_an_unreadable_summary(checkout: Path, printed: str) -> None:
    runner = FakeRunner(kev_responder(head_meta=printed))
    with pytest.raises(KevEnvironmentError, match=r"unreadable head\.pt summary"):
        SubprocessKevEnvironment(runner, checkout, URL, COMMIT).head_meta("o/r", "x")


def test_download_fetches_the_checkpoint_patterns_of_any_repository_without_forcing_offline(checkout: Path) -> None:
    runner = FakeRunner()
    environment = SubprocessKevEnvironment(runner, checkout, URL, COMMIT)
    environment.download("owner/kev-4b", "r1")
    environment.download("Qwen/Qwen3-4B", "abc")
    assert [call[0][-2:] for call in runner.calls] == [["owner/kev-4b", "r1"], ["Qwen/Qwen3-4B", "abc"]]
    assert all(call[0][7] == DOWNLOAD_SNIPPET for call in runner.calls)
    assert all(call[2] is None for call in runner.calls)
    assert "HF_HUB_OFFLINE" not in DOWNLOAD_SNIPPET
    for pattern in CHECKPOINT_PATTERNS:
        assert repr(pattern) in DOWNLOAD_SNIPPET


def test_the_snippets_are_valid_python() -> None:
    for snippet in (VERSIONS_SNIPPET, HEAD_META_SNIPPET, DOWNLOAD_SNIPPET):
        compile(snippet, "<snippet>", "exec")
