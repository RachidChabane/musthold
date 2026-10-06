"""Composition of the pipeline: adapters wired into the use cases (no ground truth is reachable from here)."""

from __future__ import annotations

import sys
from collections.abc import Iterator
from contextlib import contextmanager
from dataclasses import dataclass
from pathlib import Path

from pilot.adapters.arm_p_items import DirectoryArmPItems
from pilot.adapters.ast_edits import AstEditApplier
from pilot.adapters.differential import HypothesisDifferential
from pilot.adapters.fixture_sources import DirectoryFixtureSources
from pilot.adapters.import_smoke import ProjectImportSmoke
from pilot.adapters.model_cli import ClaudeCli, CodexCli
from pilot.adapters.models_lock import (
    InstalledCliVersions,
    JsonSchemaValidator,
    ModelsLock,
    load_baselines,
    load_models_lock,
)
from pilot.adapters.process import SubprocessRunner, local_process_runner
from pilot.adapters.project_copy import ProjectWorkspace, temporary_work_root
from pilot.adapters.prompt_files import DirectoryPromptLibrary
from pilot.adapters.provenance import GitProvenance
from pilot.adapters.pytest_runner import PytestJunitRunner, WorkspaceVariantRunner
from pilot.adapters.run_directory import RunDirectory
from pilot.adapters.shallnot import ShallnotTraceability
from pilot.adapters.witness_runner import SubprocessWitnessExecutor
from pilot.application.context import PipelineContext
from pilot.application.core_run import CoreDependencies
from pilot.application.model_router import RoutingModelPort
from pilot.application.model_caller import ModelCaller
from pilot.application.process_mutants import MutantProcessor
from pilot.application.provenance import ManifestInputs
from pilot.application.stage_timer import StageTimer
from pilot.cli.pipeline_layout import PipelineLayout
from pilot.domain.constants import MAX_CONCURRENT_MODEL_CALLS
from pilot.domain.model_call import Cli


def log_to_stderr(message: str) -> None:
    print(message, file=sys.stderr, flush=True)


@dataclass(frozen=True)
class ModelStack:
    router: RoutingModelPort
    versions: InstalledCliVersions
    lock: ModelsLock


def build_model_stack(layout: PipelineLayout, raw_dir: Path, process: SubprocessRunner) -> ModelStack:
    lock = load_models_lock(layout.models_lock)
    versions = InstalledCliVersions(process, layout.root)
    validator = JsonSchemaValidator()
    router = RoutingModelPort(
        {
            Cli.CLAUDE: ClaudeCli(process, raw_dir, cli_version=versions.installed_version(Cli.CLAUDE), schema_validator=validator),
            Cli.CODEX: CodexCli(process, raw_dir, cli_version=versions.installed_version(Cli.CODEX), schema_validator=validator),
        }
    )
    return ModelStack(router, versions, lock)


def prompt_library(layout: PipelineLayout) -> DirectoryPromptLibrary:
    return DirectoryPromptLibrary(layout.prompts_dir, layout.schemas_dir, layout.system_prompt, layout.codebook)


def fixture_sources(layout: PipelineLayout) -> DirectoryFixtureSources:
    return DirectoryFixtureSources(layout.project, layout.spec_dir)


@contextmanager
def core_dependencies(
    layout: PipelineLayout, run_id: str, expected_tests: int, expected_criteria: int
) -> Iterator[CoreDependencies]:
    process = SubprocessRunner()
    local = local_process_runner()
    stack = build_model_stack(layout, layout.raw_dir(run_id), process)
    prompts = prompt_library(layout)
    store = RunDirectory(layout.run_dir(run_id))
    caller = ModelCaller(
        stack.router,
        load_baselines(layout.canary_dir(run_id)),
        prompts.system_prompt(),
        log_to_stderr,
        MAX_CONCURRENT_MODEL_CALLS,
    )
    ctx = PipelineContext(run_id, store, fixture_sources(layout), prompts, caller, log_to_stderr)
    with temporary_work_root() as work_root:
        applier = AstEditApplier()
        workspace = ProjectWorkspace(layout.project, work_root, applier)
        pytest_runner = PytestJunitRunner(local)
        runner = WorkspaceVariantRunner(workspace, pytest_runner.run)
        processor = MutantProcessor(
            runner, applier, layout.project, ProjectImportSmoke(workspace, local),
            HypothesisDifferential(workspace, local, layout.project),
        )
        yield CoreDependencies(
            ctx=ctx,
            timer=StageTimer(store),
            provenance=GitProvenance(process, layout.root, layout.harness_source, layout.project, layout.run_dir(run_id)),
            versions=stack.versions,
            manifest_inputs=ManifestInputs(run_id, sorted(stack.lock.pins, key=lambda p: p.key), stack.lock.cli_versions),
            traceability=ShallnotTraceability(workspace, pytest_runner, local),
            runner=runner,
            processor=processor,
            witnesses=SubprocessWitnessExecutor(workspace, local),
            arm_p=DirectoryArmPItems(layout.arm_p_items),
            pins={pin.model: pin for pin in stack.lock.pins},
            expected_tests=expected_tests,
            expected_criteria=expected_criteria,
        )
