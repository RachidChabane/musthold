"""Run directory on disk: sorted-key JSON, sorted jsonl, atomic writes (RUN_RECORDS)."""

from __future__ import annotations

import json
import os
import tempfile
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Any

JSON_INDENT = 2


def _sort_value(value: Any) -> tuple[bool, Any]:
    return (value is None, "" if value is None else value)


class RunDirectory:
    """Implements the `RunStore` port over `runs/<run-id>/`."""

    def __init__(self, root: Path) -> None:
        self._root = root

    def exists(self, relative: str) -> bool:
        return (self._root / relative).exists()

    def read_text(self, relative: str) -> str:
        return (self._root / relative).read_text(encoding="utf-8")

    def write_text(self, relative: str, text: str) -> None:
        path = self._root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        descriptor, temporary = tempfile.mkstemp(dir=path.parent, prefix=".tmp-")
        with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
            handle.write(text)
        os.replace(temporary, path)

    def append_text(self, relative: str, text: str) -> None:
        path = self._root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open("a", encoding="utf-8") as handle:
            handle.write(text)

    def read_json(self, relative: str) -> Any:
        return json.loads(self.read_text(relative))

    def write_json(self, relative: str, value: Any) -> None:
        self.write_text(relative, json.dumps(value, indent=JSON_INDENT, sort_keys=True) + "\n")

    def read_jsonl(self, relative: str) -> list[dict[str, Any]]:
        return [json.loads(line) for line in self.read_text(relative).splitlines() if line.strip()]

    def write_jsonl(self, relative: str, rows: Sequence[Mapping[str, Any]], key: Sequence[str]) -> None:
        ordered = sorted(rows, key=lambda row: tuple(_sort_value(row[field]) for field in key))
        self.write_text(relative, "".join(json.dumps(row, sort_keys=True) + "\n" for row in ordered))

    def list_files(self, relative: str) -> list[str]:
        directory = self._root / relative
        if not directory.is_dir():
            return []
        return sorted(p.name for p in directory.iterdir() if p.is_file())
