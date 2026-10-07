"""`build_workspace`: the agent's workspace for a task, from its base tree, its spec and the product's init output."""

from __future__ import annotations

from collections.abc import Mapping

from specbench.application.ports import TreeFactory, TreeReader, TreeSpec
from specbench.domain.oracle import SPEC_PATH
from specbench.domain.sealing import SealedBundle
from specbench.domain.task import Task
from specbench.domain.workspace import Workspace, build_workspace


def build_agent_workspace(
    trees: TreeFactory, reader: TreeReader, task: Task, bundle: SealedBundle, init_files: Mapping[str, bytes]
) -> Workspace:
    """The base tree at the task's base commit, the sealed spec as `SPEC.md`, and the init output; no sealed byte."""
    with trees.materialize(TreeSpec(task.base_sha)) as base:
        return build_workspace(reader.read(base), bundle.files[SPEC_PATH], init_files, bundle)
