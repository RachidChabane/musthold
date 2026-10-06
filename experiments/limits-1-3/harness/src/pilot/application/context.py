"""Shared dependencies of the pipeline stages."""

from __future__ import annotations

from dataclasses import dataclass

from pilot.application.model_caller import Log, ModelCaller
from pilot.application.ports import FixtureSources, PromptLibrary, RunStore
from pilot.domain.spec import Criterion, parse_spec


@dataclass(frozen=True)
class PipelineContext:
    run_id: str
    store: RunStore
    sources: FixtureSources
    prompts: PromptLibrary
    caller: ModelCaller
    log: Log

    def criteria(self) -> tuple[Criterion, ...]:
        return parse_spec(self.sources.spec_text())
