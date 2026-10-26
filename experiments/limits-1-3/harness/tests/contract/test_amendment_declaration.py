"""Contract: every path changed since the superseded tag is declared in prereg/amendment.json."""

from __future__ import annotations

import json
import subprocess
from pathlib import Path

import pytest

from pilot.domain.amendment import AMENDMENT_FILENAME, IMPORT_DIGESTS_PLACEHOLDER, changed_frozen_inputs, undeclared_paths
from pilot.domain.preregistration import PREREG_TAG, SUPERSEDED_PREREG_TAG

EXPERIMENT = Path(__file__).resolve().parents[3]
ARCHIVE_COMMITS = EXPERIMENT.parent / "archive-commits.tsv"
FIXTURE_PATHS = ("fixture/", "prereg/oracle/", "prereg/labels.yaml", "prereg/label_matrix.csv", "prereg/violations/", "prereg/planted/", "prereg/refactor/")


def git(*args):
    return subprocess.run(["git", *args], cwd=EXPERIMENT, capture_output=True, text=True, check=False)


@pytest.fixture(scope="module")
def amendment():
    return json.loads((EXPERIMENT / "prereg" / AMENDMENT_FILENAME).read_text())


def published_commit(commit):
    """The commit of this repository that carries `commit`, when it names a commit of the archive it was published from."""
    if not ARCHIVE_COMMITS.exists():
        return commit
    for row in ARCHIVE_COMMITS.read_text().splitlines()[1:]:
        archive, public = row.split("\t")[:2]
        if archive == commit:
            return public
    return commit


def require_tag():
    if git("rev-parse", "--verify", "--quiet", f"{SUPERSEDED_PREREG_TAG}^{{commit}}").returncode != 0:
        pytest.skip("superseded tag not available")


def test_the_amendment_names_the_superseded_tag_and_its_peeled_commit(amendment):
    require_tag()
    assert amendment["superseded_tag"] == SUPERSEDED_PREREG_TAG
    assert amendment["superseded_commit"].startswith("e95f551")
    peeled = git("rev-parse", f"{SUPERSEDED_PREREG_TAG}^{{commit}}").stdout.strip()
    assert published_commit(amendment["superseded_commit"]) == peeled


def test_the_import_digests_hold_a_placeholder_until_the_seal_fills_them(amendment):
    assert amendment["import_digests"] in (IMPORT_DIGESTS_PLACEHOLDER,) or isinstance(amendment["import_digests"], dict)


def test_every_change_between_the_superseded_and_the_amended_tag_is_declared(amendment):
    require_tag()
    changed = git("diff", "--name-only", "--relative", SUPERSEDED_PREREG_TAG, PREREG_TAG, "--", ".").stdout.split()
    assert undeclared_paths(changed, amendment["declared_paths"]) == []


def test_after_the_amended_tag_the_preregistration_and_the_fixture_never_change(amendment):
    require_tag()
    changed = git("diff", "--name-only", "--relative", PREREG_TAG, "--", ".").stdout.split()
    untracked = git("ls-files", "--others", "--exclude-standard", "--", ".").stdout.split()
    assert changed_frozen_inputs(changed + untracked) == []


def test_the_fixture_oracle_labels_and_violations_are_not_declared(amendment):
    declared = amendment["declared_paths"]
    assert not any(entry.startswith(FIXTURE_PATHS) or any(p.startswith(entry) for p in FIXTURE_PATHS) for entry in declared)
