from __future__ import annotations

from datetime import date
from pathlib import Path

import pytest

from specbench.adapters.gh_miner import (
    GhNotFound,
    GitHubSource,
    MissingRecording,
    RecordingGhClient,
    ReplayGhClient,
)
from specbench.adapters.mining_store import write_mining
from specbench.application.mine import mine
from specbench.domain import licence
from specbench.domain.candidates import attrition_counts, select_sample, task_id_for
from specbench.domain.languages import Language, is_non_test_source_path, is_test_path
from specbench.domain.mining import (
    ChangedFile,
    MiningConfig,
    PullRequest,
    Stage,
    check_pull_request,
    is_feature_pull_request,
    seeded_key,
)
from support.github_world import GPL_TEXT, FakeGitHub, WorldPull, WorldRepo

SEED = 20261007
CONFIG = MiningConfig(
    seed=SEED, freeze_date=date(2026, 12, 31), languages=(Language.PYTHON, Language.GO), candidates_per_language=3
)


def pull(**changes: object) -> PullRequest:
    base: dict[str, object] = {
        "number": 7,
        "title": "feat: add --strict",
        "labels": frozenset[str](),
        "merged_on": date(2026, 8, 1),
        "base_sha": "a" * 40,
        "merge_sha": "b" * 40,
        "files": (ChangedFile("pkg/a.py", "modified", 40, 5), ChangedFile("tests/test_a.py", "added", 30, 0)),
    }
    return PullRequest(**{**base, **changes})  # type: ignore[arg-type]


@pytest.mark.parametrize(
    ("path", "expected"),
    [
        ("tests/test_x.py", True),
        ("pkg/test_x.py", True),
        ("pkg/x_test.py", True),
        ("pkg/conftest.py", True),
        ("pkg/x.py", False),
    ],
)
def test_python_test_paths(path: str, expected: bool) -> None:
    assert is_test_path(Language.PYTHON, path) is expected


def test_go_test_paths_and_documentation_are_not_sources() -> None:
    assert is_test_path(Language.GO, "a/b_test.go")
    assert is_test_path(Language.GO, "a/testdata/x.go")
    assert is_non_test_source_path(Language.GO, "a/b.go")
    assert not is_non_test_source_path(Language.GO, "docs/example.go")
    assert not is_non_test_source_path(Language.PYTHON, "README.md")


@pytest.mark.parametrize(
    ("title", "labels", "expected"),
    [
        ("feat: add a flag", (), True),
        ("Add support for tz", (), True),
        ("fix: crash on empty input", (), False),
        ("Fix: add guard", (), False),
        ("Refactor the loader", (), False),
        ("Rework the loader", ("enhancement",), True),
    ],
)
def test_feature_signal(title: str, labels: tuple[str, ...], expected: bool) -> None:
    assert is_feature_pull_request(title, frozenset(labels)) is expected


def test_licence_text_recognition() -> None:
    assert (
        licence.classify_licence_text("Permission  is hereby granted,\nfree of charge, to any person obtaining a copy")
        == "MIT"
    )
    assert licence.classify_licence_text("Apache License\nVersion 2.0, January 2004") == "Apache-2.0"
    assert licence.classify_licence_text(GPL_TEXT) is None
    assert licence.is_permissive("ISC") and not licence.is_permissive("GPL-3.0") and not licence.is_permissive(None)


@pytest.mark.parametrize(
    ("changes", "stage"),
    [
        ({"merged_on": date(2026, 6, 30)}, Stage.MERGE_WINDOW),
        ({"merged_on": date(2026, 12, 2)}, Stage.MERGE_WINDOW),
        ({"title": "docs: typo"}, Stage.FEATURE_SIGNAL),
        ({"files": (ChangedFile("tests/test_a.py", "added", 40, 0),)}, Stage.NON_TEST_FILES),
        (
            {
                "files": tuple(ChangedFile(f"pkg/m{i}.py", "modified", 10, 0) for i in range(7))
                + (ChangedFile("tests/test_a.py", "added", 3, 0),)
            },
            Stage.NON_TEST_FILES,
        ),
        (
            {"files": (ChangedFile("pkg/a.py", "modified", 10, 5), ChangedFile("tests/test_a.py", "added", 30, 0))},
            Stage.NON_TEST_LINES,
        ),
        (
            {"files": (ChangedFile("pkg/a.py", "modified", 401, 0), ChangedFile("tests/test_a.py", "added", 30, 0))},
            Stage.NON_TEST_LINES,
        ),
        ({"files": (ChangedFile("pkg/a.py", "modified", 40, 0),)}, Stage.TESTS_CHANGED),
        (
            {"files": (ChangedFile("pkg/a.py", "modified", 40, 0), ChangedFile("tests/test_a.py", "removed", 0, 30))},
            Stage.TESTS_CHANGED,
        ),
    ],
)
def test_pull_request_filters_name_their_stage(changes: dict[str, object], stage: Stage) -> None:
    rejection = check_pull_request(Language.PYTHON, pull(**changes), CONFIG)
    assert rejection is not None and rejection.stage is stage


def test_a_conforming_pull_request_passes_and_bounds_are_inclusive() -> None:
    assert check_pull_request(Language.PYTHON, pull(), CONFIG) is None
    edge = pull(
        files=(ChangedFile("pkg/a.py", "modified", 30, 0), ChangedFile("tests/test_a.py", "added", 1, 0)),
        merged_on=date(2026, 7, 1),
    )
    assert check_pull_request(Language.PYTHON, edge, CONFIG) is None
    no_freeze = MiningConfig(SEED, None, (Language.PYTHON,), 1)
    assert check_pull_request(Language.PYTHON, pull(merged_on=date(2030, 1, 1)), no_freeze) is None


def test_seeded_key_depends_on_the_seed_and_is_stable() -> None:
    assert seeded_key(1, "a/b") == seeded_key(1, "a/b")
    assert seeded_key(1, "a/b") != seeded_key(2, "a/b")


def world() -> list[WorldRepo]:
    return [
        WorldRepo("acme/alpha"),
        WorldRepo("acme/beta", pulls=[WorldPull(n) for n in range(1, 6)]),
        WorldRepo("acme/gamma", spdx_id="Apache-2.0", licence_text="Apache License Version 2.0"),
        WorldRepo("acme/delta", stars=20),
        WorldRepo("acme/epsilon", workflows=()),
        WorldRepo("acme/zeta", root_files=("LICENSE", "setup.py")),
        WorldRepo("acme/eta", licence_text=GPL_TEXT),
        WorldRepo("acme/theta", pulls=[WorldPull(1, title="fix: crash")]),
        WorldRepo("acme/gopher", language="Go", root_files=("LICENSE", "go.mod", "go.sum")),
        WorldRepo(
            "acme/goblin",
            language="Go",
            root_files=("LICENSE", "go.mod", "go.sum"),
            pulls=[WorldPull(1, files=(("a.go", "modified", 50, 0), ("a_test.go", "modified", 20, 0)))],
        ),
    ]


def test_mine_applies_every_filter_and_logs_attrition() -> None:
    result = mine(GitHubSource(FakeGitHub(world())), CONFIG)
    stages = {entry.subject: entry.stage for entry in result.attrition}
    assert stages["acme/delta"] is Stage.STARS
    assert stages["acme/epsilon#1"] is Stage.CI_WORKFLOW
    assert stages["acme/zeta#1"] is Stage.LOCKFILE
    assert stages["acme/eta#1"] is Stage.LICENCE_FILE
    assert stages["acme/theta#1"] is Stage.FEATURE_SIGNAL
    assert stages["acme/gopher#1"] is Stage.NON_TEST_FILES
    assert {c.repository for c in result.candidates} == {"acme/alpha", "acme/beta", "acme/gamma", "acme/goblin"}
    assert len(attrition_counts(result.attrition)) == len(Stage)


def test_at_most_two_tasks_per_repository_and_the_rest_are_logged() -> None:
    result = mine(GitHubSource(FakeGitHub(world())), CONFIG)
    beta = [c for c in result.candidates if c.repository == "acme/beta"]
    assert len(beta) == 2
    capped = [e.subject for e in result.attrition if e.stage is Stage.REPOSITORY_CAP]
    assert sum(subject.startswith("acme/beta#") for subject in capped) == 3
    assert all(
        sum(c.repository == name for c in result.candidates) <= 2 for name in {c.repository for c in result.candidates}
    )


def test_the_seed_fixes_the_order_and_the_sample_uses_distinct_repositories() -> None:
    first = mine(GitHubSource(FakeGitHub(world())), CONFIG)
    again = mine(GitHubSource(FakeGitHub(list(reversed(world())))), CONFIG)
    assert [c.label for c in first.candidates] == [c.label for c in again.candidates]
    other = mine(GitHubSource(FakeGitHub(world())), MiningConfig(SEED + 1, CONFIG.freeze_date, CONFIG.languages, 3))
    assert {c.label for c in other.candidates} != set()
    sample = select_sample(SEED, first.candidates, per_language=2)
    python = [c.repository for c in sample if c.language is Language.PYTHON]
    assert len(python) == len(set(python)) == 2
    assert [c.repository for c in sample if c.language is Language.GO] == ["acme/goblin"]


def test_candidates_outside_the_sample_are_logged_as_not_selected() -> None:
    result = mine(GitHubSource(FakeGitHub(world())), CONFIG)
    unselected = {e.subject for e in result.attrition if e.stage is Stage.NOT_SELECTED}
    assert unselected == {c.label for c in result.candidates} - {c.label for c in result.sample}
    assert attrition_counts(result.attrition)["not-selected"] == len(unselected) > 0


def test_a_recorder_resumes_from_its_archive_without_calling_the_network_again(tmp_path: Path) -> None:
    live = FakeGitHub(world())
    first = mine(GitHubSource(RecordingGhClient(live, tmp_path)), CONFIG)
    requests = len(live.requests)
    again = mine(GitHubSource(RecordingGhClient(live, tmp_path)), CONFIG)
    assert len(live.requests) == requests
    assert [c.label for c in again.candidates] == [c.label for c in first.candidates]


def test_task_ids_are_upper_case_slugs() -> None:
    result = mine(GitHubSource(FakeGitHub(world())), CONFIG)
    candidate = next(c for c in result.candidates if c.repository == "acme/goblin")
    assert task_id_for(candidate) == "ACME-GOBLIN-1"


def test_recording_then_replay_is_byte_identical_and_offline(tmp_path: Path) -> None:
    live = FakeGitHub(world())
    archive = tmp_path / "archive"
    recorded = mine(GitHubSource(RecordingGhClient(live, archive)), CONFIG)
    write_mining(recorded, tmp_path / "out-live")
    before = len(live.requests)

    replayed = mine(GitHubSource(ReplayGhClient(archive)), CONFIG)
    write_mining(replayed, tmp_path / "out-replay")

    assert len(live.requests) == before
    for name in ("candidates.jsonl", "attrition.jsonl", "sample.json", "mining.json"):
        assert (tmp_path / "out-live" / name).read_bytes() == (tmp_path / "out-replay" / name).read_bytes()
    assert len(list((archive / "responses").glob("*.json"))) == len(set(live.requests))


def test_every_response_is_archived_including_missing_resources(tmp_path: Path) -> None:
    client = RecordingGhClient(FakeGitHub(world()), tmp_path)
    with pytest.raises(GhNotFound):
        client.get("repos/acme/unknown/contents")
    with pytest.raises(GhNotFound):
        ReplayGhClient(tmp_path).get("repos/acme/unknown/contents")
    assert len(list((tmp_path / "responses").glob("*.json"))) == 1


def test_replay_refuses_an_unrecorded_request(tmp_path: Path) -> None:
    with pytest.raises(MissingRecording):
        ReplayGhClient(tmp_path).get("search/repositories", {"q": "x"})
