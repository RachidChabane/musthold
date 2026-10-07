"""Contract: every path changed since the superseded tag is declared in prereg/amendment.json."""

from __future__ import annotations

import json
import subprocess
from pathlib import Path

import pytest

from pilot.domain.amendment import AMENDMENT_FILENAME, IMPORT_DIGESTS_PLACEHOLDER, undeclared_paths
from pilot.domain.preregistration import SUPERSEDED_PREREG_TAG

EXPERIMENT = Path(__file__).resolve().parents[3]
FIXTURE_PATHS = ("fixture/", "prereg/oracle/", "prereg/labels.yaml", "prereg/label_matrix.csv", "prereg/violations/", "prereg/planted/", "prereg/refactor/")


def git(*args):
    return subprocess.run(["git", *args], cwd=EXPERIMENT, capture_output=True, text=True, check=False)


@pytest.fixture(scope="module")
def amendment():
    return json.loads((EXPERIMENT / "prereg" / AMENDMENT_FILENAME).read_text())


def require_tag():
    if git("rev-parse", "--verify", "--quiet", f"{SUPERSEDED_PREREG_TAG}^{{commit}}").returncode != 0:
        pytest.skip("superseded tag not available")


def test_the_amendment_names_the_superseded_tag_and_its_peeled_commit(amendment):
    require_tag()
    assert amendment["superseded_tag"] == SUPERSEDED_PREREG_TAG
    assert amendment["superseded_commit"] == git("rev-parse", f"{SUPERSEDED_PREREG_TAG}^{{commit}}").stdout.strip()
    assert amendment["superseded_commit"].startswith("e95f551")


def test_the_import_digests_hold_a_placeholder_until_the_seal_fills_them(amendment):
    assert amendment["import_digests"] in (IMPORT_DIGESTS_PLACEHOLDER,) or isinstance(amendment["import_digests"], dict)


def test_committed_changes_since_the_superseded_tag_are_declared(amendment):
    require_tag()
    changed = git("diff", "--name-only", "--relative", SUPERSEDED_PREREG_TAG, "HEAD", "--", ".").stdout.split()
    assert undeclared_paths(changed, amendment["declared_paths"]) == []


def test_working_tree_changes_and_new_files_are_declared_too(amendment):
    require_tag()
    changed = git("diff", "--name-only", "--relative", SUPERSEDED_PREREG_TAG, "--", ".").stdout.split()
    untracked = git("ls-files", "--others", "--exclude-standard", "--", ".").stdout.split()
    new = [p for p in untracked if not p.startswith("runs/")]
    assert undeclared_paths(changed + new, amendment["declared_paths"]) == []


def test_the_fixture_oracle_labels_and_violations_are_not_declared(amendment):
    declared = amendment["declared_paths"]
    assert not any(entry.startswith(FIXTURE_PATHS) or any(p.startswith(entry) for p in FIXTURE_PATHS) for entry in declared)
