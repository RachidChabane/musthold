import json
from collections.abc import Callable
from datetime import UTC, date, datetime

import pytest

from demos1.application.search import Requests, Searcher, SearchFailed
from demos1.domain.api import ApiResponse
from demos1.domain.queries import ApiRequest, search_issues
from demos1.domain.window import Interval
from support.fakes import START, FakeClock

type Answer = Callable[[ApiRequest], ApiResponse]


class ScriptedApi:
    def __init__(self, answer: Answer) -> None:
        self.answer = answer
        self.requests: list[ApiRequest] = []

    def get(self, request: ApiRequest) -> ApiResponse:
        self.requests.append(request)
        return self.answer(request)


def page(total: int, items: int, incomplete: bool = False, start: int = 0) -> ApiResponse:
    body = {
        "total_count": total,
        "incomplete_results": incomplete,
        "items": [{"html_url": f"https://github.com/north/ledger/pull/{start + index}"} for index in range(items)],
    }
    return ApiResponse(200, json.dumps(body), START)


def query(interval: Interval) -> str:
    return f"is:pr {interval.merged_qualifier()}"


def test_searches_are_spaced_3_seconds_apart_at_least_the_2_1_the_rule_asks():
    clock = FakeClock()
    api = ScriptedApi(lambda request: page(0, 0))
    requests = Requests(api, clock)
    requests.get(search_issues("q", 1))
    requests.get(search_issues("q", 2))
    requests.get(ApiRequest("repos/north/ledger"))
    assert clock.sleeps == [pytest.approx(3.0)]


def test_a_rate_limited_request_waits_for_the_reset_three_times_at_most():
    clock = FakeClock()
    reset = str(int(START.timestamp()) + 90)
    limited = ApiResponse(403, "{}", START, {"x-ratelimit-remaining": "0", "x-ratelimit-reset": reset})
    api = ScriptedApi(lambda request: limited)
    response = Requests(api, clock).get(ApiRequest("repos/north/ledger"))
    assert response.status == 403
    assert len(api.requests) == 4
    assert clock.sleeps[0] == pytest.approx(90)


def test_a_retry_after_header_names_the_wait_when_longer_than_a_minute():
    clock = FakeClock()
    answers = iter([ApiResponse(429, "{}", START, {"retry-after": "75"}), ApiResponse(200, "{}", START)])
    response = Requests(ScriptedApi(lambda request: next(answers)), clock).get(ApiRequest("repos/a/b"))
    assert response.status == 200
    assert clock.sleeps == [75.0]


def test_a_reset_that_comes_too_soon_still_waits_a_minute():
    clock = FakeClock()
    soon = str(int(START.timestamp()) + 5)
    answers = iter(
        [
            ApiResponse(403, "{}", START, {"x-ratelimit-remaining": "6", "x-ratelimit-reset": soon}),
            ApiResponse(200, "{}", START),
        ]
    )
    response = Requests(ScriptedApi(lambda request: next(answers)), clock).get(ApiRequest("repos/a/b"))
    assert response.status == 200
    assert clock.sleeps == [60.0]


def test_an_incomplete_page_is_retried_60_seconds_apart():
    clock = FakeClock()
    answers = iter([page(3, 3, incomplete=True), page(3, 3, incomplete=True), page(3, 3)])
    searcher = Searcher(Requests(ScriptedApi(lambda request: next(answers)), clock), clock)
    assert searcher.page(search_issues("q", 1)).complete
    assert [sleep for sleep in clock.sleeps if sleep == 60.0] == [60.0, 60.0]


def test_a_search_answering_non_2xx_stops_the_selection():
    clock = FakeClock()
    searcher = Searcher(Requests(ScriptedApi(lambda request: ApiResponse(422, "{}", START)), clock), clock)
    with pytest.raises(SearchFailed):
        searcher.pull_requests(query, Interval.of_day(date(2026, 9, 30)), "B-go")


def test_an_interval_of_1000_or_more_splits_at_the_middle_second_until_each_part_is_under():
    day = Interval.of_day(date(2026, 9, 30))
    first_half, second_half = day.halves()

    def answer(request: ApiRequest) -> ApiResponse:
        q = request.params["q"]
        if q == query(day):
            return page(1500, 100)
        if q == query(first_half):
            return page(150, 100, start=0) if request.params["page"] == "1" else page(150, 50, start=100)
        if q == query(second_half):
            return page(20, 20, start=500)
        raise AssertionError(q)

    clock = FakeClock()
    result = Searcher(Requests(ScriptedApi(answer), clock), clock).pull_requests(query, day, "B-go")
    assert len(result.items) == 170
    assert [(record.split, record.total_count) for record in result.intervals] == [
        (True, 1500),
        (False, 150),
        (False, 20),
    ]


def test_an_incomplete_one_second_interval_is_used_as_returned_and_recorded():
    moment = datetime(2026, 9, 30, 12, 0, 0, tzinfo=UTC)
    second = Interval(moment, moment)
    clock = FakeClock()
    searcher = Searcher(Requests(ScriptedApi(lambda request: page(2, 2, incomplete=True)), clock), clock)
    result = searcher.pull_requests(query, second, "B-go")
    assert len(result.items) == 2
    assert result.intervals[0].incomplete


def test_pages_stop_at_total_count_or_an_empty_page():
    answers = {"1": page(250, 100), "2": page(250, 100, start=100), "3": page(250, 0)}
    clock = FakeClock()
    api = ScriptedApi(lambda request: answers[request.params["page"]])
    result = Searcher(Requests(api, clock), clock).pull_requests(query, Interval.of_day(date(2026, 9, 1)), "B-go")
    assert len(result.items) == 200
    assert [request.params["page"] for request in api.requests] == ["1", "2", "3"]


def test_the_repository_search_fetches_all_ten_pages_and_never_splits():
    clock = FakeClock()
    api = ScriptedApi(lambda request: page(11014, 100 if request.params["page"] == "1" else 0))
    result = Searcher(Requests(api, clock), clock).repositories(
        lambda number: ApiRequest("search/repositories", {"page": str(number)}), "A-pytest"
    )
    assert len(api.requests) == 10
    assert len(result.items) == 100
