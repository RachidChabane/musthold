"""The deterministic facts construction reads: the pull request text, the linked issues, the non-test diff, the public
API at merge, and the human tests the probe found. Every GitHub response goes through the archiving client.

The spec author's view is built from the non-test side only: `author_view()` has no field that could hold a test, and
this module never passes a test path into the diff, the API listing or the reference texts it builds for the author.
"""

from __future__ import annotations

import json
import re
from collections.abc import Callable
from pathlib import Path
from typing import Final

from specbench.adapters.gh_miner import GhNotFound
from specbench.adapters.git_history import GitHistory, GitMirrors
from specbench.application.construct import ConstructionInputs, HumanTests
from specbench.application.ports import GhClient
from specbench.domain.changed_tests import function_sources, without_parameters
from specbench.domain.jsonread import mapping, optional_text, sequence, text
from specbench.domain.languages import Language, is_documentation_path, is_non_test_source_path, is_test_path
from specbench.domain.public_api import public_api
from specbench.domain.selection import ProbedTask

MAX_DIFF_CHARS: Final = 60_000
MAX_REFERENCE_CHARS: Final = 40_000
MAX_LINKED_ISSUES: Final = 2
DIFF_CONTEXT_LINES: Final = 3
README_NAMES: Final = ("README.md", "README.rst", "README.txt", "README")
GO_MODULE_FILE: Final = "go.mod"
GO_TEST_SUFFIX: Final = "_test.go"
PROBE_DIRECTORY: Final = "probe"
TESTS_SUFFIX: Final = ".tests.json"
SOURCE_UNAVAILABLE: Final = "(source unavailable)"
ISSUE_REFERENCE: Final = re.compile(
    r"(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s*:?\s+"
    r"(?:https://github\.com/[\w.-]+/[\w.-]+/issues/|#)(\d+)"
)
ENCODING: Final = "utf-8"


def linked_issue_numbers(*texts: str) -> list[int]:
    """Issue numbers a pull request says it closes, in first-mention order, without repeats."""
    found: list[int] = []
    for body in texts:
        for match in ISSUE_REFERENCE.finditer(body):
            number = int(match.group(1))
            if number not in found:
                found.append(number)
    return found[:MAX_LINKED_ISSUES]


def go_module_of(go_mod: str | None) -> str:
    for line in (go_mod or "").splitlines():
        if line.startswith("module "):
            return line.removeprefix("module ").strip()
    return ""


Stabiliser = Callable[[ProbedTask, tuple[str, ...], tuple[str, ...]], tuple[tuple[str, ...], tuple[str, ...]]]


class MirrorConstructionSources:
    def __init__(
        self,
        mirrors: GitMirrors,
        history_of: Callable[[Path], GitHistory],
        gh: GhClient,
        work: Path,
        stabilise: Stabiliser,
    ) -> None:
        self._mirrors = mirrors
        self._history_of = history_of
        self._gh = gh
        self._work = work
        self._stabilise = stabilise

    def gather(self, task: ProbedTask) -> ConstructionInputs:
        candidate = task.candidate
        language = candidate.language
        history = self._history_of(self._mirrors.path_of(candidate.repository))
        changed = history.changed_paths(task.base_sha, candidate.merge_sha)
        non_test = sorted(p for p in changed if not is_test_path(language, p))
        source_paths = [p for p in non_test if is_non_test_source_path(language, p)]
        merge_files = {p: text_ for p in source_paths if (text_ := history.file_at(candidate.merge_sha, p)) is not None}
        module = go_module_of(history.file_at(candidate.merge_sha, GO_MODULE_FILE)) if language is Language.GO else ""
        title, body = self._pull(candidate.repository, candidate.pull_number)
        issues = self._issues(candidate.repository, title, body)
        diff = history.unified_diff(task.base_sha, candidate.merge_sha, non_test, DIFF_CONTEXT_LINES)
        return ConstructionInputs(
            task=task,
            module=module,
            issue="\n\n".join(issues),
            pull_title=title,
            pull_body=body,
            diff=diff[:MAX_DIFF_CHARS],
            public_api=public_api(language, module, merge_files),
            reference_texts=self._references(history, candidate.merge_sha, non_test, issues, body),
            merge_files=merge_files,
            human=self._human(task, history, sorted(changed)),
        )

    def _pull(self, repository: str, number: int) -> tuple[str, str]:
        detail = mapping(self._gh.get(f"repos/{repository}/pulls/{number}"), "pull request")
        return text(detail.get("title"), "title"), optional_text(detail.get("body"), "body") or ""

    def _issues(self, repository: str, title: str, body: str) -> list[str]:
        found: list[str] = []
        for number in linked_issue_numbers(title, body):
            try:
                issue = mapping(self._gh.get(f"repos/{repository}/issues/{number}"), "issue")
            except GhNotFound:
                continue
            found.append(f"{text(issue.get('title'), 'title')}\n\n{optional_text(issue.get('body'), 'body') or ''}")
        return found

    @staticmethod
    def _references(
        history: GitHistory, merge: str, non_test: list[str], issues: list[str], pull_body: str
    ) -> tuple[str, ...]:
        """Public texts a literal may legitimately come from: the issues, the description, the docs and the README."""
        documents = [p for p in non_test if is_documentation_path(p)]
        readme = next((t for name in README_NAMES if (t := history.file_at(merge, name)) is not None), "")
        files = [history.file_at(merge, p) or "" for p in documents]
        return tuple(t[:MAX_REFERENCE_CHARS] for t in (*issues, pull_body, readme, *files) if t)

    def _human(self, task: ProbedTask, history: GitHistory, changed: list[str]) -> HumanTests:
        candidate = task.candidate
        language = candidate.language
        recorded = mapping(
            json.loads((self._work / PROBE_DIRECTORY / f"{task.task_id}{TESTS_SUFFIX}").read_text(encoding=ENCODING)),
            "tests",
        )
        u_ids, r_ids = self._stabilise(
            task,
            tuple(text(t, "u") for t in sequence(recorded.get("u"), "u")),
            tuple(text(t, "r") for t in sequence(recorded.get("r"), "r")),
        )
        test_paths = [p for p in changed if is_test_path(language, p)]
        overlay = {
            p: content.encode(ENCODING) for p in test_paths if (content := history.file_at(candidate.merge_sha, p))
        }
        wanted = frozenset(without_parameters(t) for t in u_ids)
        sources: dict[str, str] = {}
        suffix = ".py" if language is Language.PYTHON else GO_TEST_SUFFIX
        for path in test_paths:
            if path.endswith(suffix) and (content := history.file_at(candidate.merge_sha, path)):
                sources |= function_sources(language, path, content, wanted)
        return HumanTests(u_ids, r_ids, {t: sources.get(t, SOURCE_UNAVAILABLE) for t in sorted(wanted)}, overlay)
