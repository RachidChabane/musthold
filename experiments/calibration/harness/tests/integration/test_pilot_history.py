from __future__ import annotations

import json
from pathlib import Path

import pytest

from calibration.adapters.git_vcs import GitVersionControl
from calibration.adapters.pilot_git import GitPilotHistory
from calibration.domain.constants import PILOT_DECISIONS_FILE, PILOT_FREEZE_TAG, PILOT_RUN_ID
from real_process_runner import RealProcessRunner
from temp_repository import TempRepository

pytestmark = pytest.mark.integration

OTHER_RUN_ID = "run-01"


def decisions(*run_ids: str) -> str:
    return json.dumps({"analysis": {"runs": [{"id": run_id} for run_id in run_ids]}})


@pytest.fixture
def repository(tmp_path: Path) -> TempRepository:
    return TempRepository(tmp_path / "repository")


@pytest.fixture
def history(repository: TempRepository) -> GitPilotHistory:
    return GitPilotHistory(GitVersionControl(repository.root, RealProcessRunner()))


def test_tag_commit_peels_the_tag(repository: TempRepository, history: GitPilotHistory) -> None:
    commit = repository.commit({"a.txt": "a"})
    repository.tag(PILOT_FREEZE_TAG, annotated=True)
    assert history.tag_commit(PILOT_FREEZE_TAG) == commit
    assert history.tag_commit("prereg-missing") is None


def test_analysis_commit_is_the_first_descendant_of_the_freeze_that_names_the_run(
    repository: TempRepository, history: GitPilotHistory
) -> None:
    repository.commit({PILOT_DECISIONS_FILE: decisions(PILOT_RUN_ID)}, "before the freeze")
    freeze = repository.commit({"freeze.txt": "frozen"}, "freeze")
    repository.commit({PILOT_DECISIONS_FILE: decisions(OTHER_RUN_ID)}, "another run")
    repository.commit({PILOT_DECISIONS_FILE: "not json at all"}, "broken")
    analysis = repository.commit({PILOT_DECISIONS_FILE: decisions(OTHER_RUN_ID, PILOT_RUN_ID)}, "analysis")
    repository.commit({PILOT_DECISIONS_FILE: decisions(PILOT_RUN_ID, "run-03")}, "later edit")
    assert history.analysis_commit(PILOT_RUN_ID, freeze) == analysis


def test_analysis_commit_skips_commits_that_do_not_descend_from_the_freeze(
    repository: TempRepository, history: GitPilotHistory
) -> None:
    base = repository.commit({"a.txt": "a"})
    repository.branch_at("side", base)
    freeze = repository.commit({"freeze.txt": "frozen"}, "freeze")
    repository.checkout("side")
    repository.commit({PILOT_DECISIONS_FILE: decisions(PILOT_RUN_ID)}, "side analysis")
    repository.checkout("main")
    repository.merge("side")
    assert history.analysis_commit(PILOT_RUN_ID, freeze) is None


def test_analysis_commit_is_none_when_no_commit_names_the_run(
    repository: TempRepository, history: GitPilotHistory
) -> None:
    freeze = repository.commit({"a.txt": "a"})
    assert history.analysis_commit(PILOT_RUN_ID, freeze) is None
    repository.commit({PILOT_DECISIONS_FILE: decisions(OTHER_RUN_ID)})
    assert history.analysis_commit(PILOT_RUN_ID, freeze) is None


def test_analysis_commit_ignores_a_decisions_file_deleted_by_the_commit(
    repository: TempRepository, history: GitPilotHistory
) -> None:
    freeze = repository.commit({"a.txt": "a"})
    repository.commit({PILOT_DECISIONS_FILE: decisions(OTHER_RUN_ID)})
    repository.commit({}, remove=(PILOT_DECISIONS_FILE,))
    assert history.analysis_commit(PILOT_RUN_ID, freeze) is None
