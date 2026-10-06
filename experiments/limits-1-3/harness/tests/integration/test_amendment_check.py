"""`amendment-check` on a temporary repository with two annotated tags (ADR 0006)."""

from __future__ import annotations

import hashlib
import json
import subprocess
from datetime import UTC, datetime
from pathlib import Path

import pytest
from run01_fixture import build_run01

from pilot.adapters.amendment_facts import collect_facts
from pilot.adapters.process import SubprocessRunner
from pilot.adapters.run_directory import RunDirectory
from pilot.adapters.run_files import DirectoryImportSource
from pilot.application.import_stage import import_run, plan_from_source
from pilot.domain.amendment import Timestamp, evaluate_amendment
from pilot.domain.preregistration import PREREG_TAG, SUPERSEDED_PREREG_TAG

pytestmark = pytest.mark.integration
ENV = {"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t",
       "PATH": "/usr/bin:/bin:/opt/homebrew/bin:/usr/local/bin", "HOME": "/nonexistent"}
OLD_STAMP, NEW_STAMP = datetime(2026, 10, 6, 21, 49, tzinfo=UTC), datetime(2026, 10, 8, 9, 0, tzinfo=UTC)
RUN_START = datetime(2026, 10, 9, 8, 0, tzinfo=UTC)
DECLARED = ["prereg/PREREG.md", "prereg/amendment.json", "harness/", "script/", "Makefile", "freeze/README.md",
            "freeze/prereg-commit.*", "runs/run-01/"]


def git(cwd, *args):
    return subprocess.run(["git", *args], cwd=cwd, check=True, capture_output=True, text=True, env=ENV).stdout.strip()


def write(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


class FakeTimestamps:
    """Stands in for openssl: the imprint is the hash of the commit file next to the token."""

    def __init__(self, root: Path, overrides=None):
        self.root, self.overrides = root, overrides or {}

    def read(self, reply: Path):
        name = reply.name
        if name in self.overrides:
            return self.overrides[name]
        commit_file = reply.with_name(name.replace(".tsr", ".txt"))
        if not reply.is_file() or not commit_file.is_file():
            return None
        stamp = OLD_STAMP if name.startswith("prereg") else NEW_STAMP
        return Timestamp(hashlib.sha256(commit_file.read_bytes()).hexdigest(), stamp)


@pytest.fixture
def experiment(tmp_path):
    repo = tmp_path / "repo"
    exp = repo / "exp"
    git(tmp_path, "init", "-q", str(repo))
    write(exp / "prereg" / "PREREG.md", "frozen")
    write(exp / "fixture" / "f.py", "x = 1\n")
    git(repo, "add", "."), git(repo, "commit", "-qm", "freeze")
    git(repo, "tag", "-a", "-m", "freeze", SUPERSEDED_PREREG_TAG)
    old = git(repo, "rev-parse", f"{SUPERSEDED_PREREG_TAG}^{{commit}}")
    write(exp / "freeze" / "prereg-commit.txt", old)
    write(exp / "freeze" / "prereg-commit.tsr", "token")
    write(exp / "freeze" / "README.md", "freeze")
    git(repo, "add", "."), git(repo, "commit", "-qm", "timestamp the freeze")
    build_run01(exp / "runs" / "run-01")
    plan, _ = plan_from_source(DirectoryImportSource(exp / "runs" / "run-01"))
    write(exp / "prereg" / "PREREG.md", "amended")
    write(exp / "harness" / "x.py", "y = 2\n")
    write(exp / "prereg" / "amendment.json", json.dumps({
        "superseded_tag": SUPERSEDED_PREREG_TAG, "superseded_commit": old, "declared_paths": DECLARED,
        "import_digests": {e.source_path: e.digest for e in plan.entries},
    }))
    git(repo, "add", "."), git(repo, "commit", "-qm", "amend")
    git(repo, "tag", "-a", "-m", "amend", PREREG_TAG)
    write(exp / "freeze" / "a1-commit.txt", git(repo, "rev-parse", f"{PREREG_TAG}^{{commit}}"))
    write(exp / "freeze" / "a1-commit.tsr", "token")
    import_run(DirectoryImportSource(exp / "runs" / "run-01"), RunDirectory(exp / "runs" / "run-02"))
    write(exp / "runs" / "run-02" / "timing.json", json.dumps({"canary": {"started": RUN_START.isoformat(), "finished": RUN_START.isoformat(), "seconds": 1.0}}))
    return exp


def problems(exp, **overrides):
    facts = collect_facts(exp, "run-02", SubprocessRunner(), FakeTimestamps(exp, overrides))
    return evaluate_amendment(facts)


def test_a_consistent_two_tag_repository_passes(experiment):
    assert problems(experiment) == []


def test_the_diff_is_taken_between_the_tags_from_the_experiment_root(experiment):
    facts = collect_facts(experiment, "run-02", SubprocessRunner(), FakeTimestamps(experiment))
    assert set(facts.changed_paths or ()) >= {"prereg/PREREG.md", "harness/x.py", "freeze/README.md", "freeze/prereg-commit.txt",
                                              "freeze/prereg-commit.tsr", "prereg/amendment.json"}
    assert not any(path.startswith("freeze/a1-commit") for path in facts.changed_paths or ())
    assert facts.superseded_commit and facts.amended_commit and facts.superseded_commit != facts.amended_commit


def test_a_fixture_edit_between_the_tags_is_not_declared(experiment):
    repo = experiment.parent
    git(repo, "tag", "-d", PREREG_TAG)
    write(experiment / "fixture" / "f.py", "x = 2\n")
    git(repo, "add", "."), git(repo, "commit", "-qm", "edit the fixture")
    git(repo, "tag", "-a", "-m", "amend", PREREG_TAG)
    write(experiment / "freeze" / "a1-commit.txt", git(repo, "rev-parse", f"{PREREG_TAG}^{{commit}}"))
    assert any("fixture/f.py" in p for p in problems(experiment))


def test_a_missing_amended_tag_is_refused(experiment):
    git(experiment.parent, "tag", "-d", PREREG_TAG)
    assert f"tag {PREREG_TAG} does not exist" in problems(experiment)


def test_a_commit_file_that_does_not_equal_the_peeled_tag_is_refused(experiment):
    write(experiment / "freeze" / "a1-commit.txt", "0" * 40)
    assert any("a1-commit.txt holds" in p for p in problems(experiment))


def test_a_token_stamped_after_the_first_stage_is_refused(experiment):
    late = Timestamp(hashlib.sha256((experiment / "freeze" / "a1-commit.txt").read_bytes()).hexdigest(), RUN_START)
    assert any("not before the first run stage" in p for p in problems(experiment, **{"a1-commit.tsr": late}))


def test_a_token_over_another_file_is_refused(experiment):
    other = Timestamp("0" * 64, NEW_STAMP)
    assert any("stamps" in p for p in problems(experiment, **{"a1-commit.tsr": other}))


def test_a_modified_sealed_run_is_refused(experiment):
    (experiment / "runs" / "run-01" / "manifest.json").write_text("{}")
    assert any("sealed run" in p for p in problems(experiment))


def test_an_import_that_no_longer_matches_the_run_is_refused(experiment):
    (experiment / "runs" / "run-02" / "raw" / "relabel__R-OAI" / "result.json").write_text("tampered")
    assert any("differs from its digest" in p for p in problems(experiment))


def test_the_verdict_does_not_depend_on_the_working_directory(experiment, tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    assert problems(experiment) == []
