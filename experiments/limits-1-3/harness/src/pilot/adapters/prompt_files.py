"""The pre-registered prompt templates, schemas and codebook on disk."""

from __future__ import annotations

from pathlib import Path

from pilot.domain.call_type import SCHEMA_STEM, TEMPLATE_STEM, TEMPLATE_SUFFIX, CallType


class DirectoryPromptLibrary:
    """Implements the `PromptLibrary` port."""

    def __init__(self, prompts_dir: Path, schemas_dir: Path, system_prompt: Path, codebook: Path) -> None:
        self._prompts_dir = prompts_dir
        self._schemas_dir = schemas_dir
        self._system_prompt = system_prompt
        self._codebook = codebook

    def template(self, call_type: CallType) -> str:
        return (self._prompts_dir / f"{TEMPLATE_STEM[call_type]}{TEMPLATE_SUFFIX}").read_text(encoding="utf-8")

    def schema(self, call_type: CallType) -> str:
        return (self._schemas_dir / SCHEMA_STEM[call_type]).read_text(encoding="utf-8")

    def system_prompt(self) -> str:
        return self._system_prompt.read_text(encoding="utf-8")

    def codebook(self) -> str:
        return self._codebook.read_text(encoding="utf-8")
