"""`specbench site`: build the results site from a sealed run or from the synthetic fixtures, check it, write it.

A run is accepted as sealed only when:

- its manifest names the corpus lock committed in the repository, by digest, and every sample task is in it;
- every trunk record is of a sample task;
- its `results.json` is exactly what the analysis writes from those trunk records;
- it is not the synthetic fixtures.

So every figure of a sealed site traces back to trunk records of the sealed corpus.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from importlib import metadata
from pathlib import Path
from typing import Final

from specbench.adapters.file_corpus import FileCorpus
from specbench.adapters.run_store import RunStore
from specbench.adapters.site_inputs import is_fixture_run, read_fixture_inputs, read_run_inputs
from specbench.adapters.site_output import write_site
from specbench.analysis.estimate import AnalysisConfig
from specbench.analysis.results import analyze
from specbench.cli.commands import ENCODING, EXIT_FAILED, EXIT_OK, Console
from specbench.domain.canonical import canonical_results_json
from specbench.domain.jsonread import RecordError, integer, mapping
from specbench.domain.manifest import RunLayout
from specbench.domain.sealing import LOCK_FILE_NAME
from specbench.site.build import build_site
from specbench.site.check import check_site
from specbench.site.model import SiteInputs

PACKAGE: Final = "specbench"
CORPUS_DIRECTORY: Final = Path("bench/corpus")


class NotASealedRun(RecordError):
    """A run directory that cannot stand behind a public site."""


@dataclass(frozen=True)
class SiteRequest:
    run: Path | None
    root: Path
    out: Path
    commit: str
    check_only: bool


def generator_version() -> str:
    return f"{PACKAGE} {metadata.version(PACKAGE)}"


def _reproduces_analysis(run_dir: Path) -> bool:
    layout = RunLayout(run_dir)
    written = layout.results.read_text(encoding=ENCODING)
    method = mapping(mapping(json.loads(written), "results").get("method"), "method")
    config = AnalysisConfig(
        resamples=integer(method.get("resamples"), "method.resamples"), seed=integer(method.get("seed"), "method.seed")
    )
    permutations = integer(method.get("permutations"), "method.permutations")
    return canonical_results_json(analyze(RunStore(layout).read_trunks(), config, permutations)) == written


def _check_sealed_corpus(run_dir: Path, root: Path) -> None:
    corpus = root / CORPUS_DIRECTORY
    if not (corpus / LOCK_FILE_NAME).is_file():
        raise NotASealedRun(f"there is no sealed corpus: {corpus / LOCK_FILE_NAME} does not exist")
    lock = FileCorpus(corpus).read_lock()
    store = RunStore(RunLayout(run_dir))
    manifest = store.read_manifest()
    if manifest.corpus_lock_digest != lock.digest:
        raise NotASealedRun(
            f"{run_dir} names corpus lock {manifest.corpus_lock_digest}, the repository holds {lock.digest}"
        )
    sealed = {entry.task_id for entry in lock.entries}
    outside = sorted(set(manifest.sample_tasks) - sealed)
    if outside:
        raise NotASealedRun(f"sample tasks {', '.join(outside)} are not in the sealed corpus")
    strays = sorted({t.task_id for t in store.read_trunks()} - set(manifest.sample_tasks))
    if strays:
        raise NotASealedRun(f"trunk records of {', '.join(strays)} are not of a sample task")


def _inputs(request: SiteRequest) -> SiteInputs:
    if request.run is None:
        return read_fixture_inputs(request.root, request.commit, generator_version())
    if is_fixture_run(request.run, request.root):
        raise NotASealedRun(f"{request.run} carries the synthetic fixtures; build them with --fixtures")
    _check_sealed_corpus(request.run, request.root)
    if not _reproduces_analysis(request.run):
        raise NotASealedRun(f"{request.run}/results.json is not what the analysis writes from its trunk records")
    return read_run_inputs(request.run, request.commit, generator_version())


def run_site(request: SiteRequest, console: Console) -> int:
    inputs = _inputs(request)
    files = build_site(inputs)
    findings = check_site(files, inputs.mode, inputs.results)
    for finding in findings:
        console.say(finding)
    if findings:
        console.say(f"site check failed: {len(findings)} problems")
        return EXIT_FAILED
    pages = sum(1 for path in files if path.endswith(".html"))
    if request.check_only:
        console.say(f"site check passed: {pages} pages, {inputs.mode.value} build")
        return EXIT_OK
    write_site(files, request.out)
    console.say(f"site written to {request.out}: {pages} pages, {inputs.mode.value} build")
    return EXIT_OK
