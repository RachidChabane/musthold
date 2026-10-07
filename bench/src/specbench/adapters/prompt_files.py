"""Loads the committed prompts of `specbench/prompts/` and the JSON schemas of the construction roles."""

from __future__ import annotations

from importlib import resources

from specbench.adapters.schemas import SchemaName, load_schema
from specbench.domain.canonical import JsonValue
from specbench.domain.prompts import Prompt, PromptLibrary

PROMPT_SUFFIX = ".md"
PROMPT_PACKAGE = "specbench.prompts"


def load_prompts() -> PromptLibrary:
    package = resources.files(PROMPT_PACKAGE)
    return {prompt: package.joinpath(prompt.value + PROMPT_SUFFIX).read_text(encoding="utf-8") for prompt in Prompt}


def construction_schema(name: SchemaName) -> JsonValue:
    return load_schema(name)
