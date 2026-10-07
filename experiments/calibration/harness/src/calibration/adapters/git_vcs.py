"""`VersionControl` through plain `git -C <root>`; archives are streamed into `tarfile` and never leave the destination."""

from __future__ import annotations

import subprocess
import tarfile
import tempfile
from collections.abc import Callable, Generator, Sequence
from contextlib import AbstractContextManager, contextmanager
from datetime import UTC, datetime
from pathlib import Path
from typing import IO, Final

from calibration.domain.constants import ENCODING
from calibration.domain.ports import ProcessResult, ProcessRunner

GIT: Final = "git"
GIT_DIRECTORY_OPTION: Final = "-C"
TAG_REF_PREFIX: Final = "refs/tags/"
PEEL_TO_COMMIT: Final = "^{commit}"
EXCLUDE_MAGIC: Final = ":(exclude)"
PATH_SEPARATOR_ARGUMENT: Final = "--"
TAR_STREAM_MODE: Final = "r|"
TAR_SAFE_FILTER: Final = "data"
IS_ANCESTOR_YES: Final = 0
IS_ANCESTOR_NO: Final = 1
STDERR_PREVIEW_CHARS: Final = 400

StreamOpener = Callable[[Sequence[str]], AbstractContextManager[IO[bytes]]]


class GitFailure(Exception):
    """A `git` command that failed where a failure is not an answer."""


@contextmanager
def subprocess_stream(argv: Sequence[str]) -> Generator[IO[bytes]]:
    """The standard output of a command as a binary stream.

    A command that exits non-zero raises `GitFailure`, also when it made the reader of the stream fail first.
    """
    with tempfile.TemporaryFile() as errors:
        process = subprocess.Popen(list(argv), stdout=subprocess.PIPE, stderr=errors)
        assert process.stdout is not None
        reader_failure: Exception | None = None
        try:
            yield process.stdout
        except Exception as failure:
            reader_failure = failure
        finally:
            process.stdout.close()
            returncode = process.wait()
        if returncode != 0:
            errors.seek(0)
            detail = errors.read().decode(ENCODING, errors="replace")[:STDERR_PREVIEW_CHARS].strip()
            raise GitFailure(f"{' '.join(argv)} exited {returncode}: {detail}") from reader_failure
        if reader_failure is not None:
            raise reader_failure


class GitVersionControl:
    """Implements `VersionControl` over the repository at `root`, through `git` only."""

    def __init__(self, root: Path, runner: ProcessRunner, stream: StreamOpener = subprocess_stream) -> None:
        self._root = root
        self._runner = runner
        self._stream = stream

    def peel_tag(self, tag: str) -> str | None:
        result = self._run(["rev-parse", "--verify", "--quiet", f"{TAG_REF_PREFIX}{tag}{PEEL_TO_COMMIT}"])
        commit = result.stdout.decode(ENCODING).strip()
        return commit if result.returncode == 0 and commit else None

    def head_commit(self) -> str:
        return self._output(["rev-parse", "--verify", f"HEAD{PEEL_TO_COMMIT}"])

    def is_clean(self, paths: Sequence[str]) -> bool:
        return not self._output(["status", "--porcelain", PATH_SEPARATOR_ARGUMENT, *paths])

    def archive(
        self, revision: str, pathspecs: Sequence[str], destination: Path, *, exclude: Sequence[str] = ()
    ) -> None:
        destination.mkdir(parents=True, exist_ok=True)
        excluded = [f"{EXCLUDE_MAGIC}{path}" for path in exclude]
        argv = self._git_argv(["archive", revision, PATH_SEPARATOR_ARGUMENT, *pathspecs, *excluded])
        with self._stream(argv) as stream, tarfile.open(fileobj=stream, mode=TAR_STREAM_MODE) as archive:
            archive.extractall(destination, filter=TAR_SAFE_FILTER)

    def is_ancestor(self, ancestor: str, descendant: str) -> bool:
        result = self._run(["merge-base", "--is-ancestor", ancestor, descendant])
        if result.returncode not in (IS_ANCESTOR_YES, IS_ANCESTOR_NO):
            raise GitFailure(self._describe(["merge-base", "--is-ancestor", ancestor, descendant], result.stderr))
        return result.returncode == IS_ANCESTOR_YES

    def commit_time(self, commit: str) -> datetime:
        seconds = int(self._output(["show", "--no-patch", "--format=%ct", commit]))
        return datetime.fromtimestamp(seconds, UTC)

    def commits_touching(self, revision: str, path: str) -> list[str]:
        listing = self._output(["log", "--format=%H", "--reverse", revision, PATH_SEPARATOR_ARGUMENT, path])
        return listing.splitlines()

    def blob(self, revision: str, path: str) -> bytes | None:
        result = self._run(["show", f"{revision}:{path}"])
        return result.stdout if result.returncode == 0 else None

    def _git_argv(self, arguments: Sequence[str]) -> list[str]:
        return [GIT, GIT_DIRECTORY_OPTION, str(self._root), *arguments]

    def _run(self, arguments: Sequence[str]) -> ProcessResult:
        return self._runner.run(self._git_argv(arguments))

    def _output(self, arguments: Sequence[str]) -> str:
        result = self._run(arguments)
        if result.returncode != 0:
            raise GitFailure(self._describe(arguments, result.stderr))
        return result.stdout.decode(ENCODING).strip()

    @staticmethod
    def _describe(arguments: Sequence[str], stderr: bytes) -> str:
        detail = stderr.decode(ENCODING, errors="replace")[:STDERR_PREVIEW_CHARS].strip()
        return f"git {' '.join(arguments)} failed: {detail}"
