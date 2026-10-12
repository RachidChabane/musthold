from datetime import UTC, date, datetime

import pytest

from demos1.domain.candidates import Candidate, Signal, merge_items, repository_of, signals_of
from demos1.domain.ecosystem import ChangedFile, Ecosystem, detect, is_source_path, is_test_path
from demos1.domain.filters import (
    Exclusions,
    changes_a_source,
    changes_a_test,
    is_permissive,
    merge_diff_covers,
    pull_request_is_eligible,
    repository_is_eligible,
)
from demos1.domain.jsonvalue import JsonObject
from demos1.domain.queries import SearchQuery, pull_request_query, repository_query
from demos1.domain.window import Interval, days_latest_first


def item(url: str, login: str, kind: str, body: str | None, merged_at: str) -> JsonObject:
    owner_repo = url.split("github.com/")[1].rsplit("/pull/", 1)[0]
    return {
        "html_url": url,
        "number": int(url.rsplit("/", 1)[1]),
        "repository_url": f"https://api.github.com/repos/{owner_repo}",
        "user": {"login": login, "type": kind},
        "body": body,
        "pull_request": {"merged_at": merged_at},
    }


def test_the_days_of_the_window_run_from_the_last_back_to_the_first():
    days = list(days_latest_first())
    assert days[0] == date(2026, 9, 30)
    assert days[-1] == date(2026, 7, 1)
    assert len(days) == 92


def test_a_day_interval_splits_at_its_middle_second():
    first, second = Interval.of_day(date(2026, 9, 30)).halves()
    assert first.merged_qualifier() == "merged:2026-09-30T00:00:00Z..2026-09-30T11:59:59Z"
    assert second.merged_qualifier() == "merged:2026-09-30T12:00:00Z..2026-09-30T23:59:59Z"


def test_a_one_second_interval_cannot_split():
    moment = datetime(2026, 9, 30, tzinfo=UTC)
    with pytest.raises(ValueError):
        Interval(moment, moment).halves()


def test_the_queries_are_those_the_preregistration_writes():
    day = Interval.of_day(date(2026, 9, 1))
    assert pull_request_query(SearchQuery.COPILOT, Ecosystem.GO, day) == (
        "is:pr is:merged is:public author:app/copilot-swe-agent language:Go "
        "merged:2026-09-01T00:00:00Z..2026-09-01T23:59:59Z"
    )
    assert pull_request_query(SearchQuery.CODEX, Ecosystem.PYTEST, Interval.of_window()) == (
        'is:pr is:merged is:public "chatgpt.com/codex/tasks" in:body language:Python '
        "merged:2026-07-01T00:00:00Z..2026-09-30T23:59:59Z"
    )
    assert repository_query(Ecosystem.PYTEST) == "language:Python stars:>=1000 archived:false fork:false is:public"


def test_signals_need_the_bot_type_and_the_exact_login():
    assert signals_of(item("https://github.com/o/r/pull/1", "Copilot", "Bot", None, "2026-09-30T00:00:00Z")) == {
        Signal.COPILOT
    }
    assert signals_of(item("https://github.com/o/r/pull/1", "claude[bot]", "Bot", None, "2026-09-30T00:00:00Z")) == {
        Signal.CLAUDE
    }
    assert not signals_of(item("https://github.com/o/r/pull/1", "Copilot", "User", None, "2026-09-30T00:00:00Z"))
    body = "Task: https://chatgpt.com/codex/tasks/task_e_123"
    assert signals_of(item("https://github.com/o/r/pull/1", "mona", "User", body, "2026-09-30T00:00:00Z")) == {
        Signal.CODEX
    }
    assert not signals_of(
        item("https://github.com/o/r/pull/1", "mona", "User", "see chatgpt.com/codex/tasks", "2026-09-30T00:00:00Z")
    )


def test_items_merge_by_url_and_name_every_signal_of_their_copies():
    url = "https://github.com/north/ledger/pull/4"
    copies = [
        item(url, "Copilot", "Bot", None, "2026-09-30T10:00:00Z"),
        item(url, "Copilot", "Bot", "https://chatgpt.com/codex/tasks/abc", "2026-09-30T10:00:00Z"),
    ]
    (candidate,) = merge_items(copies)
    assert candidate.signals == {Signal.COPILOT, Signal.CODEX}
    assert candidate.repository == "north/ledger"


def test_candidates_order_by_merge_time_then_repository_bytes_then_number():
    moment = datetime(2026, 9, 30, 12, tzinfo=UTC)
    later = datetime(2026, 9, 30, 13, tzinfo=UTC)
    candidates = [
        Candidate("u1", "b/x", 2, moment, frozenset()),
        Candidate("u2", "B/x", 9, moment, frozenset()),
        Candidate("u3", "b/x", 1, moment, frozenset()),
        Candidate("u4", "z/z", 5, later, frozenset()),
    ]
    ordered = sorted(candidates, key=Candidate.order_key)
    assert [entry.html_url for entry in ordered] == ["u4", "u2", "u3", "u1"]


def test_repository_url_must_name_one_repository():
    with pytest.raises(ValueError):
        repository_of({"repository_url": "https://api.github.com/orgs/north"})


def test_test_paths_follow_the_scan_globs_outside_vendor():
    assert is_test_path(Ecosystem.PYTEST, "tests/test_totals.py")
    assert is_test_path(Ecosystem.PYTEST, "pkg/totals_test.py")
    assert not is_test_path(Ecosystem.PYTEST, "vendor/lib/test_x.py")
    assert not is_test_path(Ecosystem.PYTEST, "conftest.py")
    assert is_test_path(Ecosystem.GO, "queue/queue_test.go")
    assert not is_test_path(Ecosystem.GO, "queue/queue.go")
    assert is_source_path(Ecosystem.GO, "queue/queue.go")
    assert not is_source_path(Ecosystem.PYTEST, "tests/conftest.py")
    assert not is_source_path(Ecosystem.GO, "vendor/x/y.go")


def test_tests_and_sources_changed_read_status_and_paths():
    files = [ChangedFile("ledger/totals.py", "modified"), ChangedFile("tests/test_totals.py", "removed")]
    assert changes_a_source(Ecosystem.PYTEST, files)
    assert not changes_a_test(Ecosystem.PYTEST, files)
    files.append(ChangedFile("tests/test_rates.py", "renamed"))
    assert changes_a_test(Ecosystem.PYTEST, files)


def test_detection_tests_pytest_before_go():
    contents = {"pyproject.toml": "[tool.pytest.ini_options]\n", "tox.ini": "[tox]\n"}
    assert detect({"go.mod", "conftest.py"}, contents.get).ecosystem is Ecosystem.PYTEST
    assert detect({"go.mod", "pyproject.toml"}, contents.get).ecosystem is Ecosystem.PYTEST
    assert detect({"go.mod", "tox.ini"}, contents.get).ecosystem is Ecosystem.GO
    assert detect({"README.md"}, contents.get).ecosystem is None
    assert detect({"setup.cfg"}, lambda _: None).unreadable == "setup.cfg"


def test_the_licence_list_is_the_benchmark_one():
    for spdx in ("MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC"):
        assert is_permissive(spdx)
    for spdx in (None, "NOASSERTION", "GPL-3.0", "MPL-2.0"):
        assert not is_permissive(spdx)


def test_repository_and_pull_request_eligibility():
    repository = {
        "fork": False,
        "archived": False,
        "private": False,
        "stargazers_count": 1000,
        "license": {"spdx_id": "MIT"},
    }
    assert repository_is_eligible(repository, require_stars=True)
    assert not repository_is_eligible({**repository, "stargazers_count": 999}, require_stars=True)
    assert repository_is_eligible({**repository, "stargazers_count": 999}, require_stars=False)
    assert not repository_is_eligible({**repository, "archived": True}, require_stars=True)
    pull = {"merged": True, "merge_commit_sha": "a" * 40, "changed_files": 100}
    assert pull_request_is_eligible(pull)
    assert not pull_request_is_eligible({**pull, "changed_files": 101})
    assert not pull_request_is_eligible({**pull, "merge_commit_sha": None})


def test_the_merge_diff_must_hold_every_file_of_the_pull_request():
    files = [ChangedFile("a.py", "modified"), ChangedFile("tests/test_a.py", "added")]
    commit = {"files": [{"filename": "a.py"}, {"filename": "tests/test_a.py"}, {"filename": "b.py"}]}
    assert merge_diff_covers(files, commit)
    assert not merge_diff_covers([*files, ChangedFile("c.py", "added")], commit)


def test_exclusions_lowercase_and_cover_the_owner_and_listed_repositories():
    exclusions = Exclusions(frozenset({"quarry/sample"}))
    assert exclusions.excludes("Quarry/Sample")
    assert exclusions.excludes("rachidchabane/anything")
    assert not exclusions.excludes("north/ledger")
    assert exclusions.with_listed("North/Ledger").excludes("north/ledger")
