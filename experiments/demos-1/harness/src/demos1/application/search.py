"""PREREG 5, search hygiene: rate-limit retries, incomplete retries, splitting at the middle second, pagination."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Final

from demos1.application.ports import Clock, GitHubApi
from demos1.domain.api import ApiResponse
from demos1.domain.jsonvalue import JsonObject, JsonValue, as_list, as_object, integer, optional_boolean
from demos1.domain.queries import REPOSITORY_SEARCH_PAGES, ApiRequest, search_issues
from demos1.domain.window import Interval, format_utc

RATE_LIMIT_RETRIES: Final = 3
INCOMPLETE_RETRIES: Final = 3
INCOMPLETE_RETRY_SECONDS: Final = 60.0
SEARCH_SPACING_SECONDS: Final = 3.0
SEARCH_CEILING: Final = 1000


class SearchFailed(RuntimeError):
    """A search request answered non-2xx after its rate-limit retries: `select` stops."""

    def __init__(self, request: ApiRequest, response: ApiResponse) -> None:
        super().__init__(f"search {request.params.get('q', request.path)} answered HTTP {response.status}")
        self.request = request
        self.response = response


class Requests:
    """Every request of the selection: searches spaced 3 s apart (the rule asks at least 2.1 s), rate limits waited out up to 3 times."""

    def __init__(self, api: GitHubApi, clock: Clock) -> None:
        self._api = api
        self._clock = clock
        self._last_search: float | None = None

    def get(self, request: ApiRequest) -> ApiResponse:
        for attempt in range(RATE_LIMIT_RETRIES + 1):
            if request.is_search():
                self._space_search()
            response = self._api.get(request)
            if not response.is_rate_limited() or attempt == RATE_LIMIT_RETRIES:
                return response
            self._clock.sleep(response.retry_wait_seconds(self._clock.now()))
        raise AssertionError("unreachable")

    def _space_search(self) -> None:
        if self._last_search is not None:
            waited = self._clock.monotonic() - self._last_search
            if waited < SEARCH_SPACING_SECONDS:
                self._clock.sleep(SEARCH_SPACING_SECONDS - waited)
        self._last_search = self._clock.monotonic()


@dataclass(frozen=True)
class SearchPage:
    total_count: int
    items: list[JsonObject]
    complete: bool


@dataclass
class IntervalRecord:
    """One searched interval, as the frame records it."""

    label: str
    interval: Interval | None
    total_count: int
    items: int
    incomplete: bool
    split: bool

    def to_json(self) -> JsonObject:
        return {
            "kind": "search-interval",
            "list": self.label,
            "start": format_utc(self.interval.start) if self.interval else None,
            "end": format_utc(self.interval.end) if self.interval else None,
            "total_count": self.total_count,
            "items": self.items,
            "incomplete": self.incomplete,
            "split": self.split,
        }


@dataclass
class SearchResult:
    items: list[JsonObject] = field(default_factory=list[JsonObject])
    intervals: list[IntervalRecord] = field(default_factory=list[IntervalRecord])


def _page_of(body: JsonValue) -> SearchPage:
    document = as_object(body, "search response")
    items = [as_object(item) for item in as_list(document.get("items") or [], "items")]
    complete = optional_boolean(document, "incomplete_results") is not True
    return SearchPage(integer(document, "total_count"), items, complete)


class Searcher:
    def __init__(self, requests: Requests, clock: Clock) -> None:
        self._requests = requests
        self._clock = clock

    def page(self, request: ApiRequest) -> SearchPage:
        """One search page, retried up to 3 times, 60 s apart, while its results are incomplete."""
        page = self._fetch(request)
        retries = 0
        while not page.complete and retries < INCOMPLETE_RETRIES:
            self._clock.sleep(INCOMPLETE_RETRY_SECONDS)
            retries += 1
            page = self._fetch(request)
        return page

    def _fetch(self, request: ApiRequest) -> SearchPage:
        response = self._requests.get(request)
        if not response.ok():
            raise SearchFailed(request, response)
        return _page_of(response.json())

    def pull_requests(self, query: Callable[[Interval], str], interval: Interval, label: str) -> SearchResult:
        """Every item of a pull request search interval, split until each part is under 1000 and complete."""
        result = SearchResult()
        self._collect(query, interval, label, result)
        return result

    def _collect(self, query: Callable[[Interval], str], interval: Interval, label: str, result: SearchResult) -> None:
        q = query(interval)
        first = self.page(search_issues(q, 1))
        splittable = not interval.is_one_second()
        if (first.total_count >= SEARCH_CEILING or not first.complete) and splittable:
            self._split(query, interval, label, result, first.total_count)
            return
        items = list(first.items)
        incomplete = not first.complete or first.total_count >= SEARCH_CEILING
        page_number, last = 1, first
        while len(items) < first.total_count and last.items:
            page_number += 1
            last = self.page(search_issues(q, page_number))
            if not last.complete:
                if splittable:
                    self._split(query, interval, label, result, first.total_count)
                    return
                incomplete = True
            items.extend(last.items)
        result.items.extend(items)
        result.intervals.append(IntervalRecord(label, interval, first.total_count, len(items), incomplete, split=False))

    def _split(
        self, query: Callable[[Interval], str], interval: Interval, label: str, result: SearchResult, total: int
    ) -> None:
        result.intervals.append(IntervalRecord(label, interval, total, 0, incomplete=False, split=True))
        for half in interval.halves():
            self._collect(query, half, label, result)

    def repositories(self, request_of_page: Callable[[int], ApiRequest], label: str) -> SearchResult:
        """PREREG 7.1: pages 1 to 10, all fetched, never split."""
        result = SearchResult()
        for page_number in range(1, REPOSITORY_SEARCH_PAGES + 1):
            page = self.page(request_of_page(page_number))
            result.items.extend(page.items)
            result.intervals.append(
                IntervalRecord(label, None, page.total_count, len(page.items), not page.complete, split=False)
            )
        return result
