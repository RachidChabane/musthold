"""Takes the trust table a Codex session persisted for its workspace out of the owner's `config.toml`.

Codex writes `[projects."<workspace>"]` into `$CODEX_HOME/config.toml` when a session trusts its project. The bench
uses the owner's `CODEX_HOME`, so after every session it removes exactly that table and reads the file again to check
that the table is gone and that the rest is what the removal left.
"""

from __future__ import annotations

import os
import shutil
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.domain.codex_config import names_project, remove_project_table

CONFIG_FILE_NAME: Final = "config.toml"
ENCODING: Final = "utf-8"


class CodexConfigNotRestored(RuntimeError):
    """The owner's Codex configuration still names a bench workspace after the cleanup."""


@dataclass(frozen=True)
class TrustRestoration:
    """`table` is the text that was removed, empty when the session persisted nothing."""

    removed: bool
    table: str = ""


class CodexTrustTable:
    def __init__(self, codex_home: Path) -> None:
        self._path = codex_home / CONFIG_FILE_NAME

    @property
    def config_path(self) -> Path:
        return self._path

    def restore(self, workspace: Path) -> TrustRestoration:
        project = str(workspace)
        if not self._path.is_file():
            return TrustRestoration(False)
        content = self._path.read_bytes().decode(ENCODING)
        if not names_project(content, project):
            return TrustRestoration(False)
        removal = remove_project_table(content, project)
        if removal.found:
            self._replace(removal.cleaned.encode(ENCODING))
        after = self._path.read_bytes().decode(ENCODING)
        if names_project(after, project) or (removal.found and after != removal.cleaned):
            raise CodexConfigNotRestored(f"{self._path} still names {project} after the cleanup")
        return TrustRestoration(removal.found, removal.table)

    def _replace(self, content: bytes) -> None:
        pending = self._path.with_name(f".{self._path.name}.pending")
        pending.write_bytes(content)
        shutil.copymode(self._path, pending)
        os.replace(pending, self._path)
