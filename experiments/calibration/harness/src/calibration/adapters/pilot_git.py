"""`PilotGit`: the pilot's tags and the commit of its analysis, read from the history of the repository."""

from __future__ import annotations

import json
from typing import Final

from calibration.domain.constants import PILOT_DECISIONS_FILE
from calibration.domain.ports import VersionControl

HEAD: Final = "HEAD"


def names_value(document: object, value: str) -> bool:
    """Whether any string at any depth of a decoded JSON document equals `value`."""
    if isinstance(document, str):
        return document == value
    if isinstance(document, dict):
        return any(names_value(item, value) for item in document.values())  # pyright: ignore[reportUnknownVariableType, reportUnknownArgumentType]
    if isinstance(document, list):
        return any(names_value(item, value) for item in document)  # pyright: ignore[reportUnknownVariableType, reportUnknownArgumentType]
    return False


class GitPilotHistory:
    """Implements `PilotGit` over a `VersionControl`."""

    def __init__(self, vcs: VersionControl) -> None:
        self._vcs = vcs

    def tag_commit(self, tag: str) -> str | None:
        return self._vcs.peel_tag(tag)

    def analysis_commit(self, run_id: str, freeze_commit: str) -> str | None:
        for commit in self._vcs.commits_touching(HEAD, PILOT_DECISIONS_FILE):
            if self._vcs.is_ancestor(freeze_commit, commit) and self._decisions_name(commit, run_id):
                return commit
        return None

    def _decisions_name(self, commit: str, run_id: str) -> bool:
        content = self._vcs.blob(commit, PILOT_DECISIONS_FILE)
        if content is None:
            return False
        try:
            return names_value(json.loads(content), run_id)
        except ValueError:
            return False
