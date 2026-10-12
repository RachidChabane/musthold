"""Stratum B candidates: search items, their signals, their day and their processing order (PREREG 6.1-6.3)."""

from __future__ import annotations

from collections.abc import Iterable
from dataclasses import dataclass
from datetime import date, datetime
from enum import StrEnum
from typing import Final

from demos1.domain.jsonvalue import JsonObject, child, integer, optional_child, optional_text, text
from demos1.domain.window import parse_utc

BOT_TYPE: Final = "Bot"
COPILOT_LOGIN: Final = "Copilot"
CLAUDE_LOGIN: Final = "claude[bot]"
CODEX_TASK_URL: Final = "https://chatgpt.com/codex/tasks/"
REPOSITORY_URL_MARKER: Final = "/repos/"


class Signal(StrEnum):
    COPILOT = "copilot"
    CLAUDE = "claude"
    CODEX = "codex"


SIGNAL_ORDER: Final = (Signal.COPILOT, Signal.CLAUDE, Signal.CODEX)


def signals_of(item: JsonObject) -> frozenset[Signal]:
    """Every signal of PREREG 6.2 the search item meets."""
    found: set[Signal] = set()
    user = optional_child(item, "user")
    if user is not None and optional_text(user, "type") == BOT_TYPE:
        login = optional_text(user, "login")
        if login == COPILOT_LOGIN:
            found.add(Signal.COPILOT)
        if login == CLAUDE_LOGIN:
            found.add(Signal.CLAUDE)
    body = optional_text(item, "body")
    if body is not None and CODEX_TASK_URL.encode() in body.encode():
        found.add(Signal.CODEX)
    return frozenset(found)


def ordered_signals(signals: Iterable[Signal]) -> list[str]:
    present = set(signals)
    return [signal.value for signal in SIGNAL_ORDER if signal in present]


def repository_of(item: JsonObject) -> str:
    """The `owner/name` the search item's `repository_url` names, as the API returned it."""
    url = text(item, "repository_url")
    _, marker, tail = url.partition(REPOSITORY_URL_MARKER)
    if not marker or tail.count("/") != 1:
        raise ValueError(f"unexpected repository_url {url!r}")
    return tail


@dataclass(frozen=True)
class Candidate:
    """One merged pull request a search proposed, with the signals of its item."""

    html_url: str
    repository: str
    number: int
    merged_at: datetime
    signals: frozenset[Signal]

    @property
    def day(self) -> date:
        return self.merged_at.date()

    def order_key(self) -> tuple[float, bytes, int]:
        """`merged_at` latest first, then `full_name` byte-wise ascending, then the number ascending."""
        return -self.merged_at.timestamp(), self.repository.encode(), self.number


def candidate_of(item: JsonObject, signals: frozenset[Signal]) -> Candidate:
    pull_request = child(item, "pull_request")
    return Candidate(
        html_url=text(item, "html_url"),
        repository=repository_of(item),
        number=integer(item, "number"),
        merged_at=parse_utc(text(pull_request, "merged_at")),
        signals=signals,
    )


def merge_items(items: Iterable[JsonObject]) -> list[Candidate]:
    """The items of the three queries, deduplicated by `html_url`; a candidate names every signal its copies meet."""
    first: dict[str, JsonObject] = {}
    signals: dict[str, set[Signal]] = {}
    for item in items:
        url = text(item, "html_url")
        first.setdefault(url, item)
        signals.setdefault(url, set()).update(signals_of(item))
    return [candidate_of(item, frozenset(signals[url])) for url, item in first.items()]


def in_processing_order(candidates: Iterable[Candidate]) -> list[Candidate]:
    return sorted(candidates, key=Candidate.order_key)
