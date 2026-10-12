"""The four lists of PREREG 6.5 and 7.2 and the frame records of 11, as JSON documents."""

from __future__ import annotations

from collections.abc import Sequence
from dataclasses import dataclass, field
from enum import StrEnum
from typing import Final

from demos1.domain.candidates import Candidate, ordered_signals
from demos1.domain.ecosystem import ECOSYSTEM_ORDER, ChangedFile, Ecosystem, is_test_path
from demos1.domain.filters import Filter
from demos1.domain.jsonvalue import (
    JsonObject,
    JsonValue,
    as_object,
    children,
    integer,
    optional_text,
    text,
)
from demos1.domain.window import format_utc

B_LIST_SIZE: Final = 10
A_LIST_SIZE: Final = 5


class Stratum(StrEnum):
    B = "B"
    A = "A"


@dataclass(frozen=True)
class ListName:
    stratum: Stratum
    ecosystem: Ecosystem

    def __str__(self) -> str:
        return f"{self.stratum.value}-{self.ecosystem.value}"

    @classmethod
    def parse(cls, value: str) -> ListName:
        stratum, _, ecosystem = value.partition("-")
        return cls(Stratum(stratum), Ecosystem(ecosystem))


LIST_ORDER: Final = (
    ListName(Stratum.B, Ecosystem.PYTEST),
    ListName(Stratum.B, Ecosystem.GO),
    ListName(Stratum.A, Ecosystem.PYTEST),
    ListName(Stratum.A, Ecosystem.GO),
)
"""PREREG 6: the order `select` builds the lists in; the exclusions of a list read the lists built before."""


def attempt_lists(ecosystem: Ecosystem) -> tuple[ListName, ListName]:
    """PREREG 8.1: within an ecosystem, the B list first, then the A list."""
    return ListName(Stratum.B, ecosystem), ListName(Stratum.A, ecosystem)


@dataclass(frozen=True)
class PathEntry:
    path: str
    status: str
    test: bool

    def to_json(self) -> JsonObject:
        return {"path": self.path, "status": self.status, "test": self.test}

    @classmethod
    def of(cls, ecosystem: Ecosystem, changed: ChangedFile) -> PathEntry:
        return cls(changed.path, changed.status, is_test_path(ecosystem, changed.path))

    @classmethod
    def from_json(cls, document: JsonObject) -> PathEntry:
        test = document.get("test")
        return cls(text(document, "path"), text(document, "status"), test is True)


@dataclass(frozen=True)
class BEntry:
    rank: int
    full_name: str
    stars: int
    licence: str | None
    number: int
    url: str
    signals: tuple[str, ...]
    merged_at: str
    merge_commit: str
    base: str
    files: tuple[PathEntry, ...]

    def to_json(self) -> JsonObject:
        return {
            "rank": self.rank,
            "full_name": self.full_name,
            "stars": self.stars,
            "licence": self.licence,
            "pull_request": self.number,
            "url": self.url,
            "signals": list(self.signals),
            "merged_at": self.merged_at,
            "merge_commit": self.merge_commit,
            "base": self.base,
            "files": [entry.to_json() for entry in self.files],
        }

    @classmethod
    def from_json(cls, document: JsonObject) -> BEntry:
        signals = document.get("signals")
        return cls(
            rank=integer(document, "rank"),
            full_name=text(document, "full_name"),
            stars=integer(document, "stars"),
            licence=optional_text(document, "licence"),
            number=integer(document, "pull_request"),
            url=text(document, "url"),
            signals=tuple(str(value) for value in signals) if isinstance(signals, list) else (),
            merged_at=text(document, "merged_at"),
            merge_commit=text(document, "merge_commit"),
            base=text(document, "base"),
            files=tuple(PathEntry.from_json(entry) for entry in children(document, "files")),
        )


@dataclass(frozen=True)
class AEntry:
    rank: int
    full_name: str
    stars: int
    licence: str | None
    default_branch: str
    commit: str
    committer_date: str | None

    def to_json(self) -> JsonObject:
        return {
            "rank": self.rank,
            "full_name": self.full_name,
            "stars": self.stars,
            "licence": self.licence,
            "default_branch": self.default_branch,
            "commit": self.commit,
            "committer_date": self.committer_date,
        }

    @classmethod
    def from_json(cls, document: JsonObject) -> AEntry:
        return cls(
            rank=integer(document, "rank"),
            full_name=text(document, "full_name"),
            stars=integer(document, "stars"),
            licence=optional_text(document, "licence"),
            default_branch=text(document, "default_branch"),
            commit=text(document, "commit"),
            committer_date=optional_text(document, "committer_date"),
        )


type Entry = BEntry | AEntry


@dataclass
class Lists:
    b: dict[Ecosystem, list[BEntry]] = field(default_factory=dict[Ecosystem, list[BEntry]])
    a: dict[Ecosystem, list[AEntry]] = field(default_factory=dict[Ecosystem, list[AEntry]])

    def entries(self, name: ListName) -> Sequence[Entry]:
        if name.stratum is Stratum.B:
            return self.b.get(name.ecosystem, [])
        return self.a.get(name.ecosystem, [])

    def to_json(self) -> JsonObject:
        document: dict[str, JsonValue] = {}
        for ecosystem in ECOSYSTEM_ORDER:
            document[str(ListName(Stratum.B, ecosystem))] = [entry.to_json() for entry in self.b.get(ecosystem, [])]
        for ecosystem in ECOSYSTEM_ORDER:
            document[str(ListName(Stratum.A, ecosystem))] = [entry.to_json() for entry in self.a.get(ecosystem, [])]
        return document

    @classmethod
    def from_json(cls, document: JsonObject) -> Lists:
        lists = cls()
        for ecosystem in ECOSYSTEM_ORDER:
            lists.b[ecosystem] = [BEntry.from_json(as_object(entry)) for entry in children(document, f"B-{ecosystem}")]
            lists.a[ecosystem] = [AEntry.from_json(as_object(entry)) for entry in children(document, f"A-{ecosystem}")]
        return lists


def frame_candidate(
    name: ListName,
    candidate: Candidate,
    *,
    full_name: str | None = None,
    failed: Filter | None = None,
    status: int | None = None,
    rank: int | None = None,
    cut: bool = False,
) -> JsonObject:
    """One B candidate examined: its first failing filter (with the HTTP status when a request failed) or its rank."""
    return {
        "kind": "candidate",
        "list": str(name),
        "html_url": candidate.html_url,
        "repository": candidate.repository,
        "full_name": full_name,
        "pull_request": candidate.number,
        "merged_at": format_utc(candidate.merged_at),
        "day": candidate.day.isoformat(),
        "signals": ordered_signals(candidate.signals),
        "failed": failed.value if failed is not None else None,
        "status": status,
        "rank": rank,
        "cut": cut,
    }


def frame_repository(
    name: ListName,
    full_name: str,
    stars: int | None,
    *,
    failed: Filter | None = None,
    status: int | None = None,
    rank: int | None = None,
) -> JsonObject:
    """One A candidate examined."""
    return {
        "kind": "candidate",
        "list": str(name),
        "full_name": full_name,
        "stars": stars,
        "failed": failed.value if failed is not None else None,
        "status": status,
        "rank": rank,
    }
