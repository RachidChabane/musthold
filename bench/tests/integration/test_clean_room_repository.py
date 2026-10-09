from __future__ import annotations

import json
import subprocess
from pathlib import Path

import pytest

from specbench.adapters.clean_room_repository import CleanRoomRepository
from specbench.adapters.process import SubprocessRunner
from specbench.domain.ci_seed import CiIdentity, ci_run_id

pytestmark = pytest.mark.integration

FILES = {"src/app.py": "print('hi')\n", "tests/test_app.py": "def test_it():\n    assert True\n"}
DIGEST = "ab" * 32


def write_tree(tree: Path, files: dict[str, str] = FILES) -> Path:
    for relative, content in files.items():
        target = tree / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")
    return tree


def git(tree: Path, *arguments: str) -> str:
    return subprocess.run(["git", "-C", str(tree), *arguments], check=True, capture_output=True, text=True).stdout


@pytest.fixture
def repository() -> CleanRoomRepository:
    return CleanRoomRepository(SubprocessRunner())


def test_the_agent_only_commit_depends_on_the_files_alone_not_on_the_history_the_tree_came_with(
    tmp_path: Path, repository: CleanRoomRepository
) -> None:
    plain = write_tree(tmp_path / "plain")
    with_history = write_tree(tmp_path / "history")
    git(with_history, "init", "--initial-branch", "trunk")
    git(with_history, "-c", "user.name=a", "-c", "user.email=a@b.c", "commit", "--allow-empty", "-m", "earlier")
    assert repository.commit_agent_tree(plain) == repository.commit_agent_tree(with_history)
    assert git(with_history, "rev-list", "--count", "HEAD").strip() == "1"


def test_two_arm_setups_on_one_tree_share_the_agent_commit_and_differ_in_their_heads(
    tmp_path: Path, repository: CleanRoomRepository
) -> None:
    dp, everything = write_tree(tmp_path / "dp"), write_tree(tmp_path / "all")
    agent_dp, agent_all = repository.commit_agent_tree(dp), repository.commit_agent_tree(everything)
    (dp / "dp.yaml").write_text("enabled\n", encoding="utf-8")
    (everything / "all.yaml").write_text("enabled\n", encoding="utf-8")
    head_dp, head_all = repository.commit_setup(dp), repository.commit_setup(everything)
    assert agent_dp == agent_all == repository.agent_head(dp) == repository.agent_head(everything)
    assert head_dp != head_all
    assert repository.head(dp) == head_dp


def test_an_arm_with_no_setup_still_gets_a_second_commit_so_the_agent_commit_is_always_the_parent(
    tmp_path: Path, repository: CleanRoomRepository
) -> None:
    tree = write_tree(tmp_path / "tree")
    agent = repository.commit_agent_tree(tree)
    head = repository.commit_setup(tree)
    assert head != agent
    assert repository.agent_head(tree) == agent


def test_only_a_change_to_a_tracked_file_counts_as_tracked_changes(
    tmp_path: Path, repository: CleanRoomRepository
) -> None:
    tree = write_tree(tmp_path / "tree")
    repository.commit_agent_tree(tree)
    repository.commit_setup(tree)
    assert repository.has_tracked_changes(tree) is False
    (tree / "notes.txt").write_text("untracked\n", encoding="utf-8")
    assert repository.has_tracked_changes(tree) is False
    (tree / "src" / "app.py").write_text("print('changed')\n", encoding="utf-8")
    assert repository.has_tracked_changes(tree) is True


def test_the_seed_record_is_written_where_the_product_restores_it_from(
    tmp_path: Path, repository: CleanRoomRepository
) -> None:
    tree = write_tree(tmp_path / "tree")
    agent = repository.commit_agent_tree(tree)
    identity = CiIdentity(ci_run_id(DIGEST), agent, repository.commit_setup(tree))
    repository.restore_seed(tree, identity.seed_record)
    stored = json.loads((tree / ".spec-proof" / "seeds" / f"{identity.head}.json").read_text(encoding="utf-8"))
    assert stored["headSha"] == identity.head
    assert stored["firstRunId"] == identity.run_id
    assert stored["seed"] == identity.seed_record.seed


def test_a_host_ref_that_was_never_pushed_exports_nothing(tmp_path: Path, repository: CleanRoomRepository) -> None:
    tree = write_tree(tmp_path / "tree")
    repository.commit_agent_tree(tree)
    repository.commit_setup(tree)
    assert repository.export_host_ref(tree, "discpower", tmp_path / "export" / "discpower") is False


def test_a_pushed_host_ref_is_exported_as_the_flat_files_of_its_tree(
    tmp_path: Path, repository: CleanRoomRepository
) -> None:
    tree = write_tree(tmp_path / "tree")
    repository.commit_agent_tree(tree)
    head = repository.commit_setup(tree)
    git(tree, "update-ref", f"refs/spec-proof/host/discpower/{head}", head)
    exported = tmp_path / "export" / "discpower"
    assert repository.export_host_ref(tree, "discpower", exported) is True
    assert (exported / "src" / "app.py").read_text(encoding="utf-8") == FILES["src/app.py"]
