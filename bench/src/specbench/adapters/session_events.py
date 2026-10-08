"""The events of a session, written to `calls/<call-id>/events.jsonl` as they arrive.

A session killed by the window, a deadline or the machine leaves its events behind, and the execution counter settles
the night's ledger from them. An attempt appends to the file of the attempts before it; a last line that an earlier
kill cut in half is dropped first, so the file stays a stream of whole events.
"""

from __future__ import annotations

import json
from pathlib import Path
from types import TracebackType
from typing import Final, Self, TextIO

from specbench.domain.manifest import CALL_EVENTS

ENCODING: Final = "utf-8"


class SessionEventLog:
    def __init__(self, call_directory: Path) -> None:
        self._path = call_directory / CALL_EVENTS
        self._handle: TextIO | None = None
        self._lines: list[str] = []

    @property
    def path(self) -> Path:
        return self._path

    @property
    def lines(self) -> tuple[str, ...]:
        """The events of this attempt only."""
        return tuple(self._lines)

    def __enter__(self) -> Self:
        self._path.parent.mkdir(parents=True, exist_ok=True)
        _drop_cut_tail(self._path)
        self._handle = self._path.open("a", encoding=ENCODING)
        return self

    def __exit__(
        self, kind: type[BaseException] | None, error: BaseException | None, trace: TracebackType | None
    ) -> None:
        if self._handle is not None:
            self._handle.close()
            self._handle = None

    def append(self, line: str) -> None:
        if self._handle is None:
            raise RuntimeError("the event log is not open")
        self._handle.write(line + "\n")
        self._handle.flush()
        self._lines.append(line)


def _drop_cut_tail(path: Path) -> None:
    if not path.is_file():
        return
    lines = path.read_text(encoding=ENCODING).splitlines()
    while lines and not lines[-1].strip():
        lines.pop()
    if not lines:
        return
    try:
        json.loads(lines[-1])
    except json.JSONDecodeError:
        lines.pop()
    path.write_text("".join(f"{line}\n" for line in lines), encoding=ENCODING)
