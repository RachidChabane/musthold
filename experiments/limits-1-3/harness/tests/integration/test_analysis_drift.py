"""The pre-registration drift check against a throwaway git repository."""

from __future__ import annotations

import subprocess

import pytest

from pilot.adapters.git_drift import PreregistrationUncommitted, drifted_paths, preregistration_commit
from pilot.domain.analysis_constants import DRIFT_GUARDED_PATHS, PREREG_DOCUMENT_PATH

pytestmark = pytest.mark.integration


def git(root, *args):
    subprocess.run(["git", "-c", "user.name=t", "-c", "user.email=t@t", *args], cwd=root, check=True, capture_output=True)


@pytest.fixture
def repo(tmp_path):
    for path in ("prereg", "fixture", "harness/src/pilot/analysis", "harness/src/pilot/domain"):
        (tmp_path / path).mkdir(parents=True)
    (tmp_path / "prereg" / "PREREG.md").write_text("pre-registration\n")
    (tmp_path / "fixture" / "f.py").write_text("x = 1\n")
    (tmp_path / "harness" / "src" / "pilot" / "analysis" / "a.py").write_text("y = 1\n")
    (tmp_path / "harness" / "src" / "pilot" / "domain" / "gate.py").write_text("g = 1\n")
    git(tmp_path, "init", "-q")
    return tmp_path


def commit_all(root):
    git(root, "add", "-A")
    git(root, "commit", "-q", "-m", "prereg")


def test_an_uncommitted_preregistration_is_a_clean_refusal(repo):
    with pytest.raises(PreregistrationUncommitted):
        preregistration_commit(repo, PREREG_DOCUMENT_PATH)
    git(repo, "add", "-A")
    with pytest.raises(PreregistrationUncommitted):
        preregistration_commit(repo, PREREG_DOCUMENT_PATH)


def test_a_clean_tree_has_no_drift(repo):
    commit_all(repo)
    commit = preregistration_commit(repo, PREREG_DOCUMENT_PATH)
    assert len(commit) == 40
    assert drifted_paths(repo, commit, DRIFT_GUARDED_PATHS) == []


def test_modified_tracked_staged_and_untracked_guarded_files_are_drift(repo):
    commit_all(repo)
    commit = preregistration_commit(repo, PREREG_DOCUMENT_PATH)
    (repo / "prereg" / "PREREG.md").write_text("changed\n")
    (repo / "harness" / "src" / "pilot" / "analysis" / "new.py").write_text("z = 1\n")
    (repo / "fixture" / "f.py").write_text("x = 2\n")
    git(repo, "add", "fixture/f.py")
    drift = drifted_paths(repo, commit, DRIFT_GUARDED_PATHS)
    assert drift == ["fixture/f.py", "harness/src/pilot/analysis/new.py", "prereg/PREREG.md"]


def test_a_committed_change_after_the_preregistration_is_drift(repo):
    commit_all(repo)
    commit = preregistration_commit(repo, PREREG_DOCUMENT_PATH)
    (repo / "harness" / "src" / "pilot" / "domain" / "gate.py").write_text("g = 2\n")
    commit_all(repo)
    assert drifted_paths(repo, commit, DRIFT_GUARDED_PATHS) == ["harness/src/pilot/domain/gate.py"]


def test_unguarded_files_are_not_drift(repo):
    commit_all(repo)
    commit = preregistration_commit(repo, PREREG_DOCUMENT_PATH)
    (repo / "results.txt").write_text("outputs\n")
    assert drifted_paths(repo, commit, DRIFT_GUARDED_PATHS) == []
