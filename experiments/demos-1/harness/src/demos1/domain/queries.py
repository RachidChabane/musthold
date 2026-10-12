"""The GitHub API requests of the selection, exactly as PREREG 5, 6.1, 6.4 and 7.1 write them."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass, field
from enum import StrEnum
from typing import Final

from demos1.domain.ecosystem import SEARCH_LANGUAGE, Ecosystem
from demos1.domain.window import DEFAULT_BRANCH_UNTIL, Interval

PER_PAGE: Final = 100
REPOSITORY_SEARCH_PAGES: Final = 10
SEARCH_ISSUES_PATH: Final = "search/issues"
SEARCH_REPOSITORIES_PATH: Final = "search/repositories"
SEARCH_PATH_PREFIX: Final = "search/"


class SearchQuery(StrEnum):
    """The three pull request searches of stratum B."""

    COPILOT = "copilot"
    CLAUDE = "claude"
    CODEX = "codex"


PER_DAY_QUERIES: Final = (SearchQuery.COPILOT, SearchQuery.CLAUDE)
WINDOW_QUERIES: Final = (SearchQuery.CODEX,)

_QUERY_HEAD: Final = {
    SearchQuery.COPILOT: "is:pr is:merged is:public author:app/copilot-swe-agent",
    SearchQuery.CLAUDE: "is:pr is:merged is:public author:app/claude",
    SearchQuery.CODEX: 'is:pr is:merged is:public "chatgpt.com/codex/tasks" in:body',
}


def pull_request_query(query: SearchQuery, ecosystem: Ecosystem, interval: Interval) -> str:
    return f"{_QUERY_HEAD[query]} language:{SEARCH_LANGUAGE[ecosystem]} {interval.merged_qualifier()}"


def repository_query(ecosystem: Ecosystem) -> str:
    return f"language:{SEARCH_LANGUAGE[ecosystem]} stars:>=1000 archived:false fork:false is:public"


@dataclass(frozen=True)
class ApiRequest:
    """One GET request: a path and its parameters, which `gh api -X GET -f` sends as the query string."""

    path: str
    params: Mapping[str, str] = field(default_factory=dict[str, str])

    def is_search(self) -> bool:
        return self.path.startswith(SEARCH_PATH_PREFIX)

    def key(self) -> tuple[str, tuple[tuple[str, str], ...]]:
        return self.path, tuple(sorted(self.params.items()))


def search_issues(q: str, page: int) -> ApiRequest:
    return ApiRequest(SEARCH_ISSUES_PATH, {"per_page": str(PER_PAGE), "page": str(page), "q": q})


def search_repositories(ecosystem: Ecosystem, page: int) -> ApiRequest:
    return ApiRequest(
        SEARCH_REPOSITORIES_PATH,
        {
            "sort": "stars",
            "order": "desc",
            "per_page": str(PER_PAGE),
            "page": str(page),
            "q": repository_query(ecosystem),
        },
    )


def repository(full_name: str) -> ApiRequest:
    return ApiRequest(f"repos/{full_name}")


def pull(full_name: str, number: int) -> ApiRequest:
    return ApiRequest(f"repos/{full_name}/pulls/{number}")


def pull_files(full_name: str, number: int) -> ApiRequest:
    return ApiRequest(f"repos/{full_name}/pulls/{number}/files", {"per_page": str(PER_PAGE)})


def commit(full_name: str, sha: str) -> ApiRequest:
    return ApiRequest(f"repos/{full_name}/commits/{sha}")


def root_contents(full_name: str, ref: str) -> ApiRequest:
    return ApiRequest(f"repos/{full_name}/contents", {"ref": ref})


def file_contents(full_name: str, path: str, ref: str) -> ApiRequest:
    return ApiRequest(f"repos/{full_name}/contents/{path}", {"ref": ref})


def default_branch_commit(full_name: str, branch: str) -> ApiRequest:
    return ApiRequest(f"repos/{full_name}/commits", {"sha": branch, "until": DEFAULT_BRANCH_UNTIL, "per_page": "1"})
