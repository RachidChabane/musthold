"""`KevEnvironment` over `git` and `uv`: a pinned checkout of Kev, its locked environment and Hub reads inside it."""

from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Final

from calibration.application.pin_ports import HeadMeta, KevEnvironmentInfo
from calibration.domain.canonical import sha256_hex
from calibration.domain.constants import ENCODING
from calibration.domain.jsonread import RecordError, mapping, optional_text, text
from calibration.domain.ports import ProcessResult, ProcessRunner, ProcessTimeout

GIT: Final = "git"
UV: Final = "uv"
UV_LOCK_FILE: Final = "uv.lock"
KEV_EXTRA: Final = "serve"
PACKAGES_WITH_VERSIONS: Final = ("mlx", "mlx-lm", "torch")
SYNC_TIMEOUT_SECONDS: Final = 3600.0
PROBE_TIMEOUT_SECONDS: Final = 600.0
DOWNLOAD_TIMEOUT_SECONDS: Final = 14400.0
GIT_TIMEOUT_SECONDS: Final = 1800.0
STDERR_PREVIEW_CHARS: Final = 400
HEAD_FILE: Final = "head.pt"
CHECKPOINT_PATTERNS: Final = ("*.json", "*.safetensors", "*.pt", "*.txt", "*.jinja")
BASE_FIELD: Final = "base"
BASE_REVISION_FIELD: Final = "base_revision"

VERSIONS_SNIPPET: Final = (
    "import importlib.metadata as m, json, sys\nprint(json.dumps({name: m.version(name) for name in sys.argv[1:]}))\n"
)
HEAD_META_SNIPPET: Final = (
    "import json, sys\n"
    "import torch\n"
    "from huggingface_hub import hf_hub_download\n"
    f"path = hf_hub_download(sys.argv[1], {HEAD_FILE!r}, revision=sys.argv[2])\n"
    "meta = torch.load(path, map_location='cpu')\n"
    f"print(json.dumps({{{BASE_FIELD!r}: meta.get({BASE_FIELD!r}), {BASE_REVISION_FIELD!r}: meta.get({BASE_REVISION_FIELD!r})}}))\n"
)
DOWNLOAD_SNIPPET: Final = (
    "import sys\n"
    "from huggingface_hub import snapshot_download\n"
    f"patterns = {list(CHECKPOINT_PATTERNS)!r}\n"
    "snapshot_download(sys.argv[1], revision=sys.argv[2], allow_patterns=patterns)\n"
)


class KevEnvironmentError(Exception):
    """A step of the Kev environment that failed: the checkout, the sync, a probe or a download."""


class SubprocessKevEnvironment:
    """Implements `KevEnvironment`: every command goes through the injected `ProcessRunner`, never through a shell."""

    def __init__(self, runner: ProcessRunner, directory: Path, repository_url: str, commit: str) -> None:
        self._runner = runner
        self._directory = directory
        self._repository_url = repository_url
        self._commit = commit

    def setup(self) -> KevEnvironmentInfo:
        self._checkout()
        self._run(self._uv("sync"), SYNC_TIMEOUT_SECONDS)
        lock = (self._directory / UV_LOCK_FILE).read_bytes()
        return KevEnvironmentInfo(commit=self._commit, uv_lock_sha256=sha256_hex(lock), versions=self._versions())

    def head_meta(self, repository: str, revision: str) -> HeadMeta:
        output = self._run(self._python(HEAD_META_SNIPPET, repository, revision), PROBE_TIMEOUT_SECONDS)
        try:
            document = mapping(json.loads(_last_line(output)), "head.pt")
            return HeadMeta(
                base=text(document.get(BASE_FIELD), BASE_FIELD),
                base_revision=optional_text(document.get(BASE_REVISION_FIELD), BASE_REVISION_FIELD),
            )
        except (RecordError, ValueError) as error:
            raise KevEnvironmentError(f"{repository}@{revision}: unreadable head.pt summary: {error}") from error

    def download(self, repository: str, revision: str) -> None:
        self._run(self._python(DOWNLOAD_SNIPPET, repository, revision), DOWNLOAD_TIMEOUT_SECONDS)

    def _checkout(self) -> None:
        if not self._directory.exists():
            self._run(
                [GIT, "clone", self._repository_url, str(self._directory)], GIT_TIMEOUT_SECONDS, in_directory=False
            )
            self._run(
                [GIT, "-C", str(self._directory), "checkout", "--detach", self._commit],
                GIT_TIMEOUT_SECONDS,
                in_directory=False,
            )
            return
        head = _last_line(
            self._run([GIT, "-C", str(self._directory), "rev-parse", "HEAD"], GIT_TIMEOUT_SECONDS, in_directory=False)
        )
        if head != self._commit:
            raise KevEnvironmentError(f"{self._directory} is at {head}, not at the pinned commit {self._commit}")

    def _versions(self) -> Mapping[str, str]:
        output = self._run(self._python(VERSIONS_SNIPPET, *PACKAGES_WITH_VERSIONS), PROBE_TIMEOUT_SECONDS)
        try:
            document = mapping(json.loads(_last_line(output)), "versions")
            return {name: text(document.get(name), name) for name in PACKAGES_WITH_VERSIONS}
        except (RecordError, ValueError) as error:
            raise KevEnvironmentError(f"unreadable versions report: {error}") from error

    @staticmethod
    def _uv(*arguments: str) -> list[str]:
        return [UV, *arguments, "--frozen", "--extra", KEV_EXTRA]

    def _python(self, snippet: str, *arguments: str) -> list[str]:
        return [*self._uv("run"), "python", "-c", snippet, *arguments]

    def _run(self, argv: Sequence[str], timeout_seconds: float, *, in_directory: bool = True) -> str:
        try:
            result = self._runner.run(
                argv, cwd=self._directory if in_directory else None, timeout_seconds=timeout_seconds
            )
        except ProcessTimeout as error:
            raise KevEnvironmentError(f"{' '.join(argv[:3])} timed out after {timeout_seconds:.0f}s") from error
        _require_success(argv, result)
        return result.stdout.decode(ENCODING, errors="replace")


def _require_success(argv: Sequence[str], result: ProcessResult) -> None:
    if result.returncode != 0:
        detail = result.stderr.decode(ENCODING, errors="replace")[-STDERR_PREVIEW_CHARS:].strip()
        raise KevEnvironmentError(f"{' '.join(argv[:3])} exited {result.returncode}: {detail}")


def _last_line(output: str) -> str:
    lines = [line for line in output.splitlines() if line.strip()]
    return lines[-1].strip() if lines else ""
