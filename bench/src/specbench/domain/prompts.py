"""The committed prompts of the construction roles: their names and how a template is filled."""

from __future__ import annotations

from collections.abc import Mapping
from enum import Enum
from string import Template


class Prompt(Enum):
    """Each member's value is the file name, without extension, under `specbench/prompts/`."""

    SPEC_AUTHOR_SYSTEM = "spec-author.system"
    SPEC_AUTHOR_USER = "spec-author.user"
    SPEC_REVISE_USER = "spec-revise.user"
    REVIEWER_SYSTEM = "reviewer.system"
    REVIEWER_USER = "reviewer.user"
    H_WRITER_SYSTEM = "h-writer.system"
    H_WRITER_USER = "h-writer.user"
    H_LAYOUT_PYTHON = "h-layout-python"
    H_LAYOUT_GO = "h-layout-go"
    VIOLATION_AUTHOR_SYSTEM = "violation-author.system"
    VIOLATION_AUTHOR_USER = "violation-author.user"
    PLANTED_AUTHOR_SYSTEM = "planted-author.system"
    PLANTED_AUTHOR_USER = "planted-author.user"
    ASSIGNER_SYSTEM = "assigner.system"
    ASSIGNER_USER = "assigner.user"
    LEAK_PROBE_SYSTEM = "leak-probe.system"
    LEAK_PROBE_USER = "leak-probe.user"


PromptLibrary = Mapping[Prompt, str]


def fill(template: str, **fields: str) -> str:
    """Substitute `$name` placeholders; a placeholder without a value is an error, never silently left in."""
    return Template(template).substitute(fields).rstrip() + "\n"
