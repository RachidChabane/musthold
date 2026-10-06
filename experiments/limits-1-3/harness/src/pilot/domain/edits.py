"""Mutant edit format (PREREG 6.1): value objects shared by every arm."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from enum import Enum
from typing import Any


class EditFailure(Enum):
    UNPARSEABLE = "unparseable"
    ABSENT_QUALNAME = "absent_qualname"
    NOT_A_SINGLE_DEFINITION = "not_a_single_definition"
    UNSAFE_PATH = "unsafe_path"
    MISSING_FILE = "missing_file"
    TOO_MANY_CHANGED_LINES = "too_many_changed_lines"
    MALFORMED = "malformed"


class EditError(Exception):
    def __init__(self, failure: EditFailure, detail: str) -> None:
        super().__init__(f"{failure.value}: {detail}")
        self.failure = failure
        self.detail = detail


@dataclass(frozen=True)
class Edit:
    file: str
    qualname: str
    new_source: str


@dataclass(frozen=True)
class EditSet:
    edits: tuple[Edit, ...]

    @classmethod
    def from_mapping(cls, raw: Mapping[str, Any]) -> EditSet:
        entries = raw.get("edits")
        if not isinstance(entries, Sequence) or isinstance(entries, str) or not entries:
            raise EditError(EditFailure.MALFORMED, "'edits' must be a non-empty list")
        return cls(tuple(_edit_from_mapping(entry) for entry in entries))

    def to_mapping(self) -> dict[str, Any]:
        return {
            "edits": [
                {"file": e.file, "qualname": e.qualname, "new_source": e.new_source}
                for e in self.edits
            ]
        }

    @property
    def files(self) -> tuple[str, ...]:
        return tuple(dict.fromkeys(edit.file for edit in self.edits))


def _edit_from_mapping(entry: Any) -> Edit:
    required = ("file", "qualname", "new_source")
    if not isinstance(entry, Mapping) or any(
        not isinstance(entry.get(key), str) for key in required
    ):
        raise EditError(EditFailure.MALFORMED, f"each edit needs string keys {required}")
    return Edit(entry["file"], entry["qualname"], entry["new_source"])
