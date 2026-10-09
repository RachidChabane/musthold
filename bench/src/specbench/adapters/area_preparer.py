"""Builds a clean-room area: the agent's tree committed alone, the arm's setup committed on top, the CI seed restored.

The session area and the settle area are built by the same steps, so they hold the same tracked files at the same head
and a pushed host ref of one is the ref the other would have pushed.
"""

from __future__ import annotations

import shutil
from collections.abc import Callable, Sequence
from pathlib import Path

from specbench.adapters.area_environment import AreaEnvironment
from specbench.adapters.clean_room_repository import CleanRoomRepository
from specbench.adapters.product_cli import ProductCli
from specbench.adapters.shallnot_cli import ShallnotCli
from specbench.adapters.workspace_repo import GIT_DIRECTORY
from specbench.domain.arm_policy import ArmPolicy
from specbench.domain.ci_seed import CiIdentity
from specbench.domain.clean_room import RoomArea
from specbench.domain.product import ProofModule


class AreaPreparer:
    """Copies a tree into an area and installs Shallnot and each module in it; a prepared area is left as it is."""

    def __init__(
        self,
        cli: ProductCli,
        shallnot: ShallnotCli,
        repository: CleanRoomRepository,
        environment: AreaEnvironment,
    ) -> None:
        self._cli = cli
        self._shallnot = shallnot
        self._repository = repository
        self._environment = environment

    def prepare(
        self, tree: Path, area: RoomArea, policy: ArmPolicy, modules: Sequence[ProofModule], run_id: str
    ) -> CiIdentity:
        """The CI identity of the area, whether it was built now or by an earlier call."""
        if area.is_ready:
            return CiIdentity(run_id, self._repository.agent_head(area.tree), self._repository.head(area.tree))
        shutil.rmtree(area.root, ignore_errors=True)
        shutil.copytree(tree, area.tree, symlinks=True, ignore=_without_history_at(tree))
        area.state.mkdir(parents=True)
        area.cache.mkdir(parents=True)
        agent_head = self._repository.commit_agent_tree(area.tree)
        env = self._environment.of(area)
        if policy.has_shallnot:
            self._shallnot.init(area.tree, env)
        for module in modules:
            self._cli.init(module, area.tree, env)
            self._cli.enable(module, area.tree, env)
        identity = CiIdentity(run_id, agent_head, self._repository.commit_setup(area.tree))
        self._repository.restore_seed(area.tree, identity.seed_record)
        area.ready_marker.write_text("ready\n", encoding="utf-8")
        return identity


def _without_history_at(root: Path) -> Callable[[str, list[str]], list[str]]:
    """A `copytree` filter that leaves out the repository metadata of the tree's root and nothing deeper."""

    def ignore(directory: str, names: list[str]) -> list[str]:
        return [GIT_DIRECTORY] if Path(directory) == root and GIT_DIRECTORY in names else []

    return ignore
