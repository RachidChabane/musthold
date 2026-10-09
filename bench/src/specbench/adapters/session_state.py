"""What a clean-room session area remembers about its gate-host session: its id, and whether it ran to its result.

The file lives beside the area's tree, outside every directory a session can read as its project. A re-entered gate
reads it to skip a session that already finished and to resume that session for each CI round trip.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Final

from specbench.domain.clean_room import RoomArea
from specbench.domain.jsonread import boolean, mapping, optional_text

STATE_FILE_NAME: Final = "session-state.json"


@dataclass(frozen=True)
class SessionState:
    session_id: str | None = None
    finished: bool = False


class SessionStateFile:
    def __init__(self, area: RoomArea) -> None:
        self._path = area.root / STATE_FILE_NAME

    def read(self) -> SessionState:
        if not self._path.is_file():
            return SessionState()
        body = mapping(json.loads(self._path.read_text(encoding="utf-8")), STATE_FILE_NAME)
        return SessionState(
            optional_text(body.get("session_id"), "session_id"), boolean(body.get("finished"), "finished")
        )

    def write(self, state: SessionState) -> None:
        self._path.parent.mkdir(parents=True, exist_ok=True)
        record = {"finished": state.finished, "session_id": state.session_id}
        self._path.write_text(json.dumps(record, sort_keys=True), encoding="utf-8")
