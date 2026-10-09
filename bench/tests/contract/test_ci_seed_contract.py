"""The Python `ci_seed` against the Go `CISeed` the product's CI run draws its mutants from."""

from __future__ import annotations

import json
import os
import shutil
from pathlib import Path

import pytest

from specbench.adapters.clean_room_repository import CleanRoomRepository
from specbench.adapters.process import SubprocessRunner
from specbench.adapters.workspace_repo import IDENTITY
from specbench.domain.ci_seed import ci_run_id, ci_seed
from support.product_world import BINARIES, PARCEL_FIXTURE, product_is_built

pytestmark = [
    pytest.mark.integration,
    pytest.mark.skipif(not product_is_built(), reason="the product is not built: run script/product-build"),
]

DIGESTS = ("00" * 32, "ab" * 32, "9f3c" * 16)
DP_BINARY = BINARIES / "spec-proof-dp"
SEED_DIRECTORY = ".spec-proof/seeds"


def drawn_seed(root: Path, run_id: str, runner: SubprocessRunner) -> tuple[str, int]:
    """The head commit of a project and the seed the product's CI run draws for it when no seed record is restored."""
    tree = root / "tree"
    shutil.copytree(PARCEL_FIXTURE, tree)
    repository = CleanRoomRepository(runner)
    repository.commit_agent_tree(tree)
    env = {
        **os.environ,
        **IDENTITY,
        "XDG_STATE_HOME": str(root / "state"),
        "XDG_CACHE_HOME": str(root / "cache"),
        "PATH": f"{BINARIES}{os.pathsep}{os.environ['PATH']}",
        "SPEC_PROOF_SHALLNOT": str(BINARIES / "shallnot"),
        "SPEC_PROOF_CI_RUN_ID": run_id,
    }
    assert (
        runner.run([str(BINARIES / "shallnot"), "init", "--dir", str(tree), "--hooks", "none"], tree, env).returncode
        == 0
    )
    for arguments in (["init", "--dir", str(tree), "--hooks", "none"], ["enable", "--dir", str(tree)]):
        assert runner.run([str(DP_BINARY), *arguments], tree, env).returncode == 0
    head = repository.commit_setup(tree)
    empty_bundle = root / "bundle"
    empty_bundle.mkdir()
    runner.run(
        [str(DP_BINARY), "prove", "--changed", "--root", str(tree), "--ci", "--import-host", str(empty_bundle)],
        tree,
        env,
    )
    record = json.loads((tree / SEED_DIRECTORY / f"{head}.json").read_text(encoding="utf-8"))
    assert record["headSha"] == head
    assert record["firstRunId"] == run_id
    return head, record["seed"]


@pytest.mark.parametrize("digest", DIGESTS)
def test_the_python_seed_equals_the_seed_the_product_draws_for_the_same_head_and_run(
    tmp_path: Path, digest: str
) -> None:
    run_id = ci_run_id(digest)
    head, seed = drawn_seed(tmp_path, run_id, SubprocessRunner())
    assert ci_seed(head, run_id) == seed
