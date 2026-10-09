"""The git repository of a clean-room area: the agent's tree committed alone, the arm's setup committed on top.

Both commits carry the benchmark identity and a fixed date, so a commit is a pure function of the files it holds. The
agent-only commit therefore depends on the tree digest and nothing else, which is what makes one CI seed per tree
possible (`domain.ci_seed`). The repository is also where `host push` publishes the answers of a gate-host session and
where the CI job's checkout is simulated.
"""

from __future__ import annotations

import shutil
import tarfile
from pathlib import Path
from typing import Final

from specbench.adapters.product_cli import EXIT_CLEAN, ProductCommandFailed
from specbench.adapters.workspace_repo import GIT_DIRECTORY, IDENTITY
from specbench.application.ports import ProcessRunner
from specbench.domain.ci_seed import SeedRecord

GIT: Final = "git"
BENCH_BRANCH: Final = "main"
AGENT_COMMIT_MESSAGE: Final = "clean room"
SETUP_COMMIT_MESSAGE: Final = "arm setup"
HOST_REF_PREFIX: Final = "refs/spec-proof/host"
EXPORT_ARCHIVE_NAME: Final = "export.tar"


class CleanRoomRepository:
    """Commits, seed records and host-ref exports of a clean-room tree, all through the `git` command line."""

    def __init__(self, runner: ProcessRunner) -> None:
        self._runner = runner

    def commit_agent_tree(self, tree: Path) -> str:
        """Commits the files of `tree` alone, dropping any history it came with, and returns the commit."""
        shutil.rmtree(tree / GIT_DIRECTORY, ignore_errors=True)
        self._git(tree, "init", "--initial-branch", BENCH_BRANCH)
        return self._commit(tree, AGENT_COMMIT_MESSAGE)

    def commit_setup(self, tree: Path) -> str:
        """Commits what the arm's setup wrote on top of the agent-only commit and returns the head."""
        return self._commit(tree, SETUP_COMMIT_MESSAGE)

    def head(self, tree: Path) -> str:
        return self._git(tree, "rev-parse", "HEAD").strip()

    def agent_head(self, tree: Path) -> str:
        return self._git(tree, "rev-parse", "HEAD~1").strip()

    def restore_seed(self, tree: Path, record: SeedRecord) -> None:
        """Writes the seed record of the head where the product's CI cache restore would put it."""
        path = tree / record.path
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(record.to_json(), encoding="utf-8")

    def has_tracked_changes(self, tree: Path) -> bool:
        """Whether a tracked file differs from the head: the state in which `host push` warns and CI finds no answer."""
        return bool(self._git(tree, "status", "--porcelain", "--untracked-files=no").strip())

    def export_host_ref(self, tree: Path, module_ref: str, destination: Path) -> bool:
        """Extracts the tree of `refs/spec-proof/host/<module>/<head>` into `destination`; `False` when it is absent."""
        ref = f"{HOST_REF_PREFIX}/{module_ref}/{self.head(tree)}"
        archive = destination.parent / EXPORT_ARCHIVE_NAME
        destination.mkdir(parents=True, exist_ok=True)
        probe = self._runner.run([GIT, "-C", str(tree), "rev-parse", "--verify", "--quiet", ref], tree, IDENTITY)
        if probe.returncode != EXIT_CLEAN:
            return False
        self._git(tree, "archive", "--format=tar", "-o", str(archive), ref)
        with tarfile.open(archive) as opened:
            opened.extractall(destination, filter="data")
        archive.unlink()
        return True

    def _commit(self, tree: Path, message: str) -> str:
        self._git(tree, "add", "--all")
        self._git(tree, "commit", "--allow-empty", "--message", message)
        return self.head(tree)

    def _git(self, tree: Path, *arguments: str) -> str:
        argv = [GIT, "-C", str(tree), *arguments]
        result = self._runner.run(argv, tree, IDENTITY)
        if result.returncode != EXIT_CLEAN:
            raise ProductCommandFailed(argv, result)
        return result.stdout
