"""The configuration homes of Claude Code sessions: one per trunk, a snapshot at S_T, and a copy per branch.

Claude finds a session to resume or fork by its id in the configuration home it runs with, and by the working
directory. Each trunk therefore has its own home under the run; the home as it stands at S_T is snapshotted, and every
branch starts from its own copy of that snapshot, so a branch never sees another branch's sessions and never touches
the trunk's.
"""

from __future__ import annotations

import shutil
from pathlib import Path
from typing import Final

from specbench.adapters.session_environment import checked_trunk_id
from specbench.domain.arms import Arm
from specbench.domain.manifest import RunLayout
from specbench.domain.sessions import SessionKind, SessionRequest

TRUNK_HOME: Final = "claude"
SNAPSHOT_HOME: Final = "claude-snapshot"
HOME_PREFIX: Final = "claude-"
TRUNK_ARMS: Final = frozenset({Arm.N, Arm.T})
"""N and T share one trunk session; every other arm that runs a trunk of its own has a home of its own."""


class MissingSnapshot(RuntimeError):
    """A branch was asked to start before the trunk's home was snapshotted at S_T."""


class HomeInsideWorkspace(ValueError):
    """A configuration home would live where the coder can reach it."""


class ClaudeConfigHomes:
    def __init__(self, layout: RunLayout) -> None:
        self._layout = layout

    def trunk(self, trunk_id: str) -> Path:
        return self._layout.homes(checked_trunk_id(trunk_id)) / TRUNK_HOME

    def snapshot_path(self, trunk_id: str) -> Path:
        return self._layout.homes(checked_trunk_id(trunk_id)) / SNAPSHOT_HOME

    def branch(self, trunk_id: str, arm: Arm) -> Path:
        return self._layout.homes(checked_trunk_id(trunk_id)) / f"{HOME_PREFIX}{arm.value}"

    def snapshot(self, trunk_id: str) -> Path:
        """Copy the trunk's home as it stands, replacing an earlier snapshot of the same trunk."""
        source = self.trunk(trunk_id)
        source.mkdir(parents=True, exist_ok=True)
        return _replace_with_copy(source, self.snapshot_path(trunk_id))

    def home_for(self, request: SessionRequest, workspace: Path) -> Path:
        """The home a session runs with, created on demand. The first step of a branch starts from the snapshot."""
        arm = _arm_of(request)
        home = self._select(request, arm)
        if request.kind is SessionKind.BRANCH and request.fork:
            self._branch_from_snapshot(request.trunk_id, home)
        home.mkdir(parents=True, exist_ok=True)
        _refuse_inside(home, workspace)
        return home

    def _select(self, request: SessionRequest, arm: Arm) -> Path:
        if request.kind is SessionKind.BRANCH:
            return self.branch(request.trunk_id, arm)
        return self.trunk(request.trunk_id) if arm in TRUNK_ARMS else self.branch(request.trunk_id, arm)

    def _branch_from_snapshot(self, trunk_id: str, home: Path) -> None:
        snapshot = self.snapshot_path(trunk_id)
        if not snapshot.is_dir():
            raise MissingSnapshot(f"trunk {trunk_id} has no S_T snapshot of its configuration home")
        _replace_with_copy(snapshot, home)


def _arm_of(request: SessionRequest) -> Arm:
    if request.arm is None or not request.trunk_id:
        raise ValueError("a coder session names its trunk and its arm")
    return request.arm


def _replace_with_copy(source: Path, destination: Path) -> Path:
    if destination.exists():
        shutil.rmtree(destination)
    shutil.copytree(source, destination, symlinks=True)
    return destination


def _refuse_inside(home: Path, workspace: Path) -> None:
    resolved_home, resolved_workspace = home.resolve(), workspace.resolve()
    if resolved_home == resolved_workspace or resolved_workspace in resolved_home.parents:
        raise HomeInsideWorkspace(f"{home} is inside the workspace {workspace}")
