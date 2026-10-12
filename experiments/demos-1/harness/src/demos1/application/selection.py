"""The `select` use case: the four lists and the frame, as a function of the API responses (PREREG 5 to 7)."""

from __future__ import annotations

import base64
import binascii
from collections import defaultdict
from collections.abc import Callable
from dataclasses import dataclass, field
from datetime import date
from typing import Final

from demos1.application.search import Requests, Searcher
from demos1.domain import queries
from demos1.domain.api import ApiResponse
from demos1.domain.candidates import Candidate, merge_items, ordered_signals
from demos1.domain.ecosystem import Ecosystem, detect
from demos1.domain.filters import (
    Exclusions,
    Filter,
    changed_files,
    changes_a_source,
    changes_a_test,
    licence_of,
    merge_diff_covers,
    merge_parent,
    pull_request_is_eligible,
    repository_is_eligible,
    root_names,
)
from demos1.domain.jsonvalue import (
    JsonObject,
    JsonShapeError,
    as_list,
    as_object,
    optional_child,
    optional_integer,
    optional_text,
    text,
)
from demos1.domain.lists import (
    A_LIST_SIZE,
    B_LIST_SIZE,
    LIST_ORDER,
    AEntry,
    BEntry,
    ListName,
    Lists,
    PathEntry,
    Stratum,
    frame_candidate,
    frame_repository,
)
from demos1.domain.queries import PER_DAY_QUERIES, WINDOW_QUERIES, SearchQuery, pull_request_query
from demos1.domain.window import Interval, days_latest_first, format_utc, in_window

BASE64: Final = "base64"


@dataclass
class SelectionResult:
    lists: Lists = field(default_factory=Lists)
    frame: list[JsonObject] = field(default_factory=list[JsonObject])
    outside_window: list[JsonObject] = field(default_factory=list[JsonObject])


class _Rejected(Exception):
    def __init__(self, failed: Filter, status: int | None = None) -> None:
        super().__init__(failed.value)
        self.failed = failed
        self.status = status


def decoded_content(body: JsonObject) -> str | None:
    """The text of a file the contents API returned, or None when it holds none it can be read from."""
    if optional_text(body, "encoding") != BASE64:
        return None
    content = optional_text(body, "content")
    if content is None:
        return None
    try:
        return base64.b64decode(content).decode("utf-8", errors="replace")
    except (binascii.Error, ValueError):
        return None


class Selector:
    def __init__(self, requests: Requests, searcher: Searcher, pool: frozenset[str]) -> None:
        self._requests = requests
        self._searcher = searcher
        self._exclusions = Exclusions(pool)
        self._repositories: dict[str, ApiResponse] = {}

    def run(self) -> SelectionResult:
        result = SelectionResult()
        for name in LIST_ORDER:
            if name.stratum is Stratum.B:
                result.lists.b[name.ecosystem] = self._stratum_b(name, result)
            else:
                result.lists.a[name.ecosystem] = self._stratum_a(name, result)
            for entry in result.lists.entries(name):
                self._exclusions = self._exclusions.with_listed(entry.full_name)
        return result

    # Stratum B ------------------------------------------------------------------------------------------

    def _search_b(self, name: ListName, result: SelectionResult) -> list[JsonObject]:
        items: list[JsonObject] = []
        label = str(name)
        for day in days_latest_first():
            for query in PER_DAY_QUERIES:
                found = self._searcher.pull_requests(_query_of(query, name.ecosystem), Interval.of_day(day), label)
                items += found.items
                result.frame += [record.to_json() for record in found.intervals]
        for query in WINDOW_QUERIES:
            found = self._searcher.pull_requests(_query_of(query, name.ecosystem), Interval.of_window(), label)
            items += found.items
            result.frame += [record.to_json() for record in found.intervals]
        return items

    def _stratum_b(self, name: ListName, result: SelectionResult) -> list[BEntry]:
        items = self._search_b(name, result)
        by_day: dict[date, list[Candidate]] = defaultdict(list)
        for candidate in merge_items(item for item in items if _merged_at(item) is not None):
            if in_window(candidate.merged_at):
                by_day[candidate.day].append(candidate)
            else:
                result.outside_window.append(_outside(name, candidate.html_url, format_utc(candidate.merged_at)))
        for item in items:
            if _merged_at(item) is None:
                result.outside_window.append(_outside(name, text(item, "html_url"), None))
        entries: list[BEntry] = []
        exclusions = self._exclusions
        for day in days_latest_first():
            for candidate in sorted(by_day.get(day, []), key=Candidate.order_key):
                rank = len(entries) + 1
                try:
                    entry = self._filter_b(name.ecosystem, candidate, exclusions, rank)
                except _Rejected as rejected:
                    result.frame.append(
                        frame_candidate(
                            name,
                            candidate,
                            full_name=self._canonical(candidate.repository),
                            failed=rejected.failed,
                            status=rejected.status,
                        )
                    )
                    continue
                entries.append(entry)
                exclusions = exclusions.with_listed(entry.full_name)
                result.frame.append(
                    frame_candidate(name, candidate, full_name=entry.full_name, rank=rank, cut=rank > B_LIST_SIZE)
                )
            if len(entries) >= B_LIST_SIZE:
                break
        return entries[:B_LIST_SIZE]

    def _canonical(self, repository: str) -> str | None:
        response = self._repositories.get(repository)
        if response is None or not response.ok():
            return None
        return optional_text(as_object(response.json()), "full_name")

    def _repository(self, repository: str) -> ApiResponse:
        if repository not in self._repositories:
            self._repositories[repository] = self._requests.get(queries.repository(repository))
        return self._repositories[repository]

    def _filter_b(self, ecosystem: Ecosystem, candidate: Candidate, exclusions: Exclusions, rank: int) -> BEntry:
        if not candidate.signals:
            raise _Rejected(Filter.SIGNAL)
        repository = self._json_of(self._repository(candidate.repository), Filter.EXCLUDED)
        full_name = text(repository, "full_name")
        if exclusions.excludes(full_name):
            raise _Rejected(Filter.EXCLUDED)
        if not repository_is_eligible(repository, require_stars=True):
            raise _Rejected(Filter.REPOSITORY)
        pull = self._json_of(self._requests.get(queries.pull(full_name, candidate.number)), Filter.PULL_REQUEST)
        if not pull_request_is_eligible(pull):
            raise _Rejected(Filter.PULL_REQUEST)
        merge_commit = text(pull, "merge_commit_sha")
        files_response = self._requests.get(queries.pull_files(full_name, candidate.number))
        if not files_response.ok():
            raise _Rejected(Filter.TESTS_CHANGED, files_response.status)
        files = changed_files([as_object(entry) for entry in as_list(files_response.json(), "files")])
        if not changes_a_test(ecosystem, files):
            raise _Rejected(Filter.TESTS_CHANGED)
        if not changes_a_source(ecosystem, files):
            raise _Rejected(Filter.SOURCE_CHANGED)
        commit = self._json_of(self._requests.get(queries.commit(full_name, merge_commit)), Filter.MERGE_DIFF)
        base = merge_parent(commit)
        if base is None or not merge_diff_covers(files, commit):
            raise _Rejected(Filter.MERGE_DIFF)
        self._check_ecosystem(full_name, merge_commit, ecosystem)
        return BEntry(
            rank=rank,
            full_name=full_name,
            stars=optional_integer(repository, "stargazers_count") or 0,
            licence=licence_of(repository),
            number=candidate.number,
            url=candidate.html_url,
            signals=tuple(ordered_signals(candidate.signals)),
            merged_at=format_utc(candidate.merged_at),
            merge_commit=merge_commit,
            base=base,
            files=tuple(PathEntry.of(ecosystem, changed) for changed in files),
        )

    # Stratum A ------------------------------------------------------------------------------------------

    def _stratum_a(self, name: ListName, result: SelectionResult) -> list[AEntry]:
        found = self._searcher.repositories(lambda page: queries.search_repositories(name.ecosystem, page), str(name))
        result.frame += [record.to_json() for record in found.intervals]
        ordered = sorted(found.items, key=_star_order)
        entries: list[AEntry] = []
        exclusions = self._exclusions
        for item in ordered:
            if len(entries) >= A_LIST_SIZE:
                break
            full_name = text(item, "full_name")
            stars = optional_integer(item, "stargazers_count")
            rank = len(entries) + 1
            try:
                entry = self._filter_a(name.ecosystem, item, exclusions, rank)
            except _Rejected as rejected:
                result.frame.append(
                    frame_repository(name, full_name, stars, failed=rejected.failed, status=rejected.status)
                )
                continue
            entries.append(entry)
            exclusions = exclusions.with_listed(entry.full_name)
            result.frame.append(frame_repository(name, full_name, stars, rank=rank))
        return entries

    def _filter_a(self, ecosystem: Ecosystem, item: JsonObject, exclusions: Exclusions, rank: int) -> AEntry:
        full_name = text(item, "full_name")
        if exclusions.excludes(full_name):
            raise _Rejected(Filter.EXCLUDED)
        if not repository_is_eligible(item, require_stars=False):
            raise _Rejected(Filter.REPOSITORY)
        branch = text(item, "default_branch")
        response = self._requests.get(queries.default_branch_commit(full_name, branch))
        if not response.ok():
            raise _Rejected(Filter.COMMIT, response.status)
        commits = as_list(response.json(), "commits")
        if not commits:
            raise _Rejected(Filter.COMMIT)
        first = as_object(commits[0])
        sha = text(first, "sha")
        commit_detail = optional_child(first, "commit")
        committer = optional_child(commit_detail, "committer") if commit_detail is not None else None
        self._check_ecosystem(full_name, sha, ecosystem)
        return AEntry(
            rank=rank,
            full_name=full_name,
            stars=optional_integer(item, "stargazers_count") or 0,
            licence=licence_of(item),
            default_branch=branch,
            commit=sha,
            committer_date=optional_text(committer, "date") if committer is not None else None,
        )

    # Shared ---------------------------------------------------------------------------------------------

    def _json_of(self, response: ApiResponse, failed: Filter) -> JsonObject:
        if not response.ok():
            raise _Rejected(failed, response.status)
        try:
            return as_object(response.json())
        except (JsonShapeError, ValueError) as error:
            raise _Rejected(failed, response.status) from error

    def _check_ecosystem(self, full_name: str, ref: str, ecosystem: Ecosystem) -> None:
        listing = self._requests.get(queries.root_contents(full_name, ref))
        if not listing.ok():
            raise _Rejected(Filter.ECOSYSTEM, listing.status)
        names = root_names([as_object(entry) for entry in as_list(listing.json(), "contents")])
        statuses: list[int] = []

        def read_config(path: str) -> str | None:
            response = self._requests.get(queries.file_contents(full_name, path, ref))
            statuses.append(response.status)
            if not response.ok():
                return None
            return decoded_content(as_object(response.json()))

        detection = detect(names, read_config)
        if detection.unreadable is not None:
            raise _Rejected(Filter.ECOSYSTEM, statuses[-1] if statuses else None)
        if detection.ecosystem is not ecosystem:
            raise _Rejected(Filter.ECOSYSTEM)


def _query_of(query: SearchQuery, ecosystem: Ecosystem) -> Callable[[Interval], str]:
    return lambda interval: pull_request_query(query, ecosystem, interval)


def _merged_at(item: JsonObject) -> str | None:
    pull_request = optional_child(item, "pull_request")
    return optional_text(pull_request, "merged_at") if pull_request is not None else None


def _outside(name: ListName, html_url: str, merged_at: str | None) -> JsonObject:
    return {"list": str(name), "html_url": html_url, "merged_at": merged_at}


def _star_order(item: JsonObject) -> tuple[int, bytes]:
    return -(optional_integer(item, "stargazers_count") or 0), text(item, "full_name").encode()
