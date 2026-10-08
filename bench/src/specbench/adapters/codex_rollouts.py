"""Codex rollout files: where a thread's record lives under `$CODEX_HOME/sessions`, and what it says.

`codex exec --json` prints neither the arguments of every `spawn_agent` call nor what the spawned threads did; the
rollouts do. A resumed thread appends to its own rollout and a fork copies its parent's history into a new one, so a
session's own spawns are the ones its rollout gained over the rollout it started from.
"""

from __future__ import annotations

from pathlib import Path
from typing import Final

from specbench.adapters.codex_stream import RolloutFile, parse_codex_rollout

SESSIONS_DIRECTORY: Final = "sessions"
ROLLOUT_PATTERN: Final = "*/*/*/rollout-*-{thread_id}.jsonl"
ENCODING: Final = "utf-8"


class CodexRollouts:
    def __init__(self, codex_home: Path) -> None:
        self._sessions = codex_home / SESSIONS_DIRECTORY

    def locate(self, thread_id: str) -> Path | None:
        found = sorted(self._sessions.glob(ROLLOUT_PATTERN.format(thread_id=thread_id)))
        return found[0] if found else None

    def read(self, thread_id: str) -> RolloutFile | None:
        path = self.locate(thread_id)
        return parse_codex_rollout(path.read_text(encoding=ENCODING)) if path is not None else None

    def own_spawns(self, thread_id: str) -> int:
        """The `spawn_agent` calls the thread's own rollout holds; a thread with no rollout holds none."""
        rollout = self.read(thread_id)
        return len(rollout.spawns) if rollout is not None else 0
