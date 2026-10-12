"""`select` on recorded API responses: the lists and the frame are a function of the saved responses."""

import json
from collections import defaultdict, deque
from pathlib import Path, PurePosixPath

from demos1.adapters.github import RecordingApi, ReplayApi
from demos1.application.select_run import run_selection
from demos1.domain.api import ApiResponse
from demos1.domain.ecosystem import Ecosystem
from demos1.domain.queries import ApiRequest
from support.fakes import START, FakeClock

FIXTURE = Path(__file__).parent.parent / "fixtures" / "selection" / "responses.json"
EMPTY_SEARCH = json.dumps({"total_count": 0, "incomplete_results": False, "items": []})
POOL = frozenset({"quarry-lab/sample"})


class RecordedApi:
    """The recorded responses; a search no recording answers had no result on the day it ran."""

    def __init__(self) -> None:
        self.queues: dict[tuple[str, tuple[tuple[str, str], ...]], deque[ApiResponse]] = defaultdict(deque)
        for entry in json.loads(FIXTURE.read_text()):
            request = ApiRequest(entry["request"]["path"], entry["request"]["params"])
            self.queues[request.key()].append(ApiResponse(entry["status"], json.dumps(entry["body"]), START))
        self.unanswered: list[ApiRequest] = []

    def get(self, request: ApiRequest) -> ApiResponse:
        queue = self.queues.get(request.key())
        if queue:
            return queue.popleft()
        if request.is_search():
            return ApiResponse(200, EMPTY_SEARCH, START)
        self.unanswered.append(request)
        return ApiResponse(404, '{"message": "Not Found"}', START)


def run(api: object) -> object:
    return run_selection(api, FakeClock(), POOL)  # type: ignore[arg-type]


def test_the_four_lists_from_the_recorded_responses():
    api = RecordedApi()
    result = run_selection(api, FakeClock(), POOL)
    assert api.unanswered == []
    (pytest_entry,) = result.lists.b[Ecosystem.PYTEST]
    assert (pytest_entry.full_name, pytest_entry.number, pytest_entry.rank) == ("northwind/ledger-py", 41, 1)
    assert pytest_entry.merge_commit == "4f1c2e8a9b0d3c5e7f6a1b2c3d4e5f60718293a4"
    assert pytest_entry.base == "a0b1c2d3e4f5061728394a5b6c7d8e9f00112233"
    assert pytest_entry.signals == ("copilot",)
    assert [(path.path, path.status, path.test) for path in pytest_entry.files] == [
        ("ledger/totals.py", "modified", False),
        ("tests/test_totals.py", "added", True),
        ("README.md", "modified", False),
    ]
    (go_entry,) = result.lists.b[Ecosystem.GO]
    assert (go_entry.full_name, go_entry.number) == ("pebble-sys/queue", 88)
    (a_pytest,) = result.lists.a[Ecosystem.PYTEST]
    assert (a_pytest.full_name, a_pytest.commit, a_pytest.committer_date) == (
        "birch-dev/forms",
        "5d4c3b2a19087f6e5d4c3b2a1908f7e6d5c4b3a2",
        "2026-09-30T21:14:03Z",
    )
    (a_go,) = result.lists.a[Ecosystem.GO]
    assert a_go.full_name == "cedar-run/log"


def test_the_frame_keeps_each_candidate_first_failing_filter_or_rank():
    result = run_selection(RecordedApi(), FakeClock(), POOL)
    candidates = {
        (record["list"], record.get("html_url") or record["full_name"]): (record["failed"], record["rank"])
        for record in result.frame
        if record["kind"] == "candidate"
    }
    assert candidates == {
        ("B-pytest", "https://github.com/northwind/ledger-py/pull/41"): (None, 1),
        ("B-pytest", "https://github.com/northwind/ledger-py/pull/42"): ("excluded", None),
        ("B-pytest", "https://github.com/harbor-dev/tide/pull/7"): ("signal", None),
        ("B-pytest", "https://github.com/acme-labs/forms/pull/5"): ("repository", None),
        ("B-pytest", "https://github.com/kiln-io/glaze/pull/9"): ("tests-changed", None),
        ("B-go", "https://github.com/northwind/ledger-py/pull/43"): ("excluded", None),
        ("B-go", "https://github.com/pebble-sys/queue/pull/88"): (None, 1),
        ("A-pytest", "oak-org/sheets"): ("repository", None),
        ("A-pytest", "birch-dev/forms"): (None, 1),
        ("A-pytest", "northwind/ledger-py"): ("excluded", None),
        ("A-go", "willow-hq/cli"): ("ecosystem", None),
        ("A-go", "cedar-run/log"): (None, 1),
        ("A-go", "pebble-sys/queue"): ("excluded", None),
    }


def test_the_pool_excludes_by_lowercased_canonical_name():
    result = run_selection(RecordedApi(), FakeClock(), frozenset({"northwind/ledger-py"}))
    assert result.lists.b[Ecosystem.PYTEST] == []
    assert [entry.full_name for entry in result.lists.a[Ecosystem.PYTEST]] == ["birch-dev/forms"]


def test_saved_responses_give_the_same_lists_again(tmp_path: Path):
    responses = PurePosixPath(tmp_path / "responses")
    first = run_selection(RecordingApi(RecordedApi(), responses), FakeClock(), POOL)
    saved = sorted((tmp_path / "responses").iterdir())
    document = json.loads(saved[0].read_text())
    assert set(document) >= {"request", "fetched_at", "status", "body", "body_sha256"}
    replayed = run_selection(ReplayApi.from_directory(responses), FakeClock(), POOL)
    assert replayed.lists.to_json() == first.lists.to_json()
    assert replayed.frame == first.frame
