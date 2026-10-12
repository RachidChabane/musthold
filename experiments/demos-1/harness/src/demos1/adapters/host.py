"""Facts of the host, and the spec-proof repository's tags, commits and origin."""

from __future__ import annotations

import platform
import subprocess
from collections.abc import Mapping, Sequence
from pathlib import PurePosixPath
from typing import Final

from demos1.adapters.engine import DockerEngine
from demos1.domain.environment import Fact, gh_version_word

SW_VERS: Final = "sw_vers"


def _output(argv: list[str], cwd: PurePosixPath | None = None) -> tuple[int, str]:
    completed = subprocess.run(argv, capture_output=True, text=True, cwd=cwd, check=False)
    return completed.returncode, completed.stdout.strip()


class LocalHostFacts:
    def __init__(self, engine: DockerEngine) -> None:
        self._engine = engine

    def host(self) -> Mapping[str, str]:
        _, name = _output([SW_VERS, "-productName"])
        _, version = _output([SW_VERS, "-productVersion"])
        _, gh = _output(["gh", "--version"])
        return {
            Fact.HOST_OS_NAME.value: name or platform.system(),
            Fact.HOST_OS_VERSION.value: version or platform.release(),
            Fact.HOST_ARCH.value: platform.machine(),
            Fact.GH_VERSION.value: gh_version_word(gh.splitlines()[0] if gh else "") or "",
        }

    def engine(self) -> Mapping[str, str]:
        return self._engine.engine_facts()


class GitRepository:
    """Git on the spec-proof repository only, never on a clone."""

    def __init__(self, root: PurePosixPath) -> None:
        self._root = root

    def _git(self, *arguments: str) -> tuple[int, str]:
        return _output(["git", "-C", str(self._root), *arguments])

    def commit_of(self, ref: str) -> str | None:
        status, output = self._git("rev-parse", "--verify", "--quiet", f"{ref}^{{commit}}")
        return output if status == 0 and output else None

    def is_ancestor(self, ancestor: str, descendant: str) -> bool:
        status, _ = self._git("merge-base", "--is-ancestor", ancestor, descendant)
        return status == 0

    def commits_touching(self, since: str, until: str, paths: Sequence[str]) -> Sequence[str]:
        _, output = self._git("log", "--format=%H", f"{since}..{until}", "--", *paths)
        return [line for line in output.splitlines() if line]

    def head(self) -> str:
        _, output = self._git("rev-parse", "HEAD")
        return output

    def origin_visibility(self) -> str:
        _, output = _output(["gh", "repo", "view", "--json", "visibility", "--jq", ".visibility"], cwd=self._root)
        return output
