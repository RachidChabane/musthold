"""The provenance manifest of a run (`manifest.json`) and the layout of a run directory."""

from __future__ import annotations

import re
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from specbench.domain.arms import FROZEN_MODEL, Arm, Harness
from specbench.domain.canonical import JsonValue, canonical_json, parse_canonical
from specbench.domain.deviations import DEVIATIONS_FILE_NAME
from specbench.domain.jsonread import RecordError, enum_of, mapping, sequence, text
from specbench.domain.pacing import CALL_LEDGER_FILE_NAME
from specbench.domain.reservation import EXECUTION_HISTORY_FILE_NAME
from specbench.domain.task import DIGEST_PATTERN, SHA_PATTERN

MANIFEST_SCHEMA: Final = "specbench.manifest/3"
BUDGET_UNIT: Final = "model-execution"
BUDGET_UNIT_KEY: Final = "budget_unit"
MANIFEST_FILE_NAME: Final = "manifest.json"
RESULTS_FILE_NAME: Final = "results.json"
REPORT_FILE_NAME: Final = "REPORT.md"
TRUNKS_DIRECTORY: Final = "trunks"
CALLS_DIRECTORY: Final = "calls"
CALL_ARGV: Final = "argv.json"
CALL_STDIN: Final = "stdin.txt"
CALL_STDOUT: Final = "stdout.txt"
CALL_EVENTS: Final = "events.jsonl"
CALL_META: Final = "meta.json"
CALL_FILES: Final = (CALL_ARGV, CALL_STDIN, CALL_STDOUT, CALL_EVENTS, CALL_META)
RUN_ID_PATTERN: Final = re.compile(r"^[a-z0-9][a-z0-9-]{2,63}$")
SHALLNOT_BINARY: Final = "shallnot"
PRODUCT_BINARIES: Final = (
    SHALLNOT_BINARY,
    "spec-proof-alignment",
    "spec-proof-completeness",
    "spec-proof-dp",
    "spec-proof-ledger",
    "spec-proof-s1",
    "spec-proof-verify",
)
"""The executables of `.tools/bin` the benchmark drives; `script/product-build` builds them."""
UMBRELLA_PLUGIN: Final = "umbrella-plugin"
PRODUCT_ARTIFACTS: Final = (*PRODUCT_BINARIES, UMBRELLA_PLUGIN)
CODING_AGENT_CLIS: Final = ("claude", "codex")


@dataclass(frozen=True)
class RunLayout:
    """Where every file of `bench/runs/<run-id>/` lives."""

    root: Path

    @property
    def manifest(self) -> Path:
        return self.root / MANIFEST_FILE_NAME

    @property
    def deviations(self) -> Path:
        return self.root / DEVIATIONS_FILE_NAME

    @property
    def call_ledger(self) -> Path:
        return self.root / CALL_LEDGER_FILE_NAME

    @property
    def execution_history(self) -> Path:
        return self.root / EXECUTION_HISTORY_FILE_NAME

    @property
    def results(self) -> Path:
        return self.root / RESULTS_FILE_NAME

    @property
    def report(self) -> Path:
        return self.root / REPORT_FILE_NAME

    def trunk(self, task_id: str, harness: Harness) -> Path:
        return self.root / TRUNKS_DIRECTORY / task_id / f"{harness.value}.json"

    @property
    def trunks(self) -> Path:
        return self.root / TRUNKS_DIRECTORY

    def call(self, call_id: str) -> Path:
        return self.root / CALLS_DIRECTORY / call_id

    def stop_diff(self, task_id: str, harness: Harness, arm: Arm, stop: int) -> Path:
        return self.root / TRUNKS_DIRECTORY / task_id / harness.value / arm.value / f"stop-{stop}.diff"


@dataclass(frozen=True)
class Provenance:
    """What a run executed: the digest of each product artifact, the Shallnot commit and the coding-agent CLI versions."""

    product_digests: Mapping[str, str]
    shallnot_commit: str
    cli_versions: Mapping[str, str]

    def __post_init__(self) -> None:
        if set(self.product_digests) != set(PRODUCT_ARTIFACTS):
            raise RecordError(f"product_digests must name exactly {', '.join(PRODUCT_ARTIFACTS)}")
        for name, digest in self.product_digests.items():
            if DIGEST_PATTERN.match(digest) is None:
                raise RecordError(f"product digest of {name} is not a SHA-256 digest")
        if SHA_PATTERN.match(self.shallnot_commit) is None:
            raise RecordError("shallnot_commit is not a commit SHA")
        if set(self.cli_versions) != set(CODING_AGENT_CLIS):
            raise RecordError(f"cli_versions must name exactly {', '.join(CODING_AGENT_CLIS)}")
        if not all(self.cli_versions.values()):
            raise RecordError("a CLI version is empty")


@dataclass(frozen=True)
class Manifest:
    run_id: str
    seed: int
    prereg_tag: str
    provenance: Provenance
    corpus_lock_digest: str
    prices_lock_digest: str
    os_name: str
    toolchains: Mapping[str, str]
    harness_models: Mapping[Harness, str]
    sample_tasks: tuple[str, ...]

    def __post_init__(self) -> None:
        if RUN_ID_PATTERN.match(self.run_id) is None:
            raise RecordError(f"run id {self.run_id!r} is malformed")
        for name, digest in (
            ("corpus_lock_digest", self.corpus_lock_digest),
            ("prices_lock_digest", self.prices_lock_digest),
        ):
            if DIGEST_PATTERN.match(digest) is None:
                raise RecordError(f"{name} is not a SHA-256 digest")
        for harness, model in self.harness_models.items():
            if model != FROZEN_MODEL[harness]:
                raise RecordError(f"{harness.value} must run {FROZEN_MODEL[harness]}, not {model}")

    def to_record(self) -> dict[str, JsonValue]:
        return {
            BUDGET_UNIT_KEY: BUDGET_UNIT,
            "cli_versions": dict(sorted(self.provenance.cli_versions.items())),
            "corpus_lock_digest": self.corpus_lock_digest,
            "harness_models": {h.value: m for h, m in sorted(self.harness_models.items(), key=lambda kv: kv[0].value)},
            "os": self.os_name,
            "prereg_tag": self.prereg_tag,
            "prices_lock_digest": self.prices_lock_digest,
            "product_digests": dict(sorted(self.provenance.product_digests.items())),
            "run_id": self.run_id,
            "sample_tasks": list(self.sample_tasks),
            "schema": MANIFEST_SCHEMA,
            "seed": self.seed,
            "shallnot_commit": self.provenance.shallnot_commit,
            "toolchains": dict(sorted(self.toolchains.items())),
        }

    def to_canonical_json(self) -> str:
        return canonical_json(self.to_record())


def _texts(value: object, where: str) -> dict[str, str]:
    return {key: text(item, f"{where}.{key}") for key, item in mapping(value, where).items()}


def parse_manifest(content: str) -> Manifest:
    body = mapping(parse_canonical(content), "manifest")
    if body.get("schema") != MANIFEST_SCHEMA:
        raise RecordError(f"schema must be {MANIFEST_SCHEMA}")
    if body.get(BUDGET_UNIT_KEY) != BUDGET_UNIT:
        raise RecordError(f"{BUDGET_UNIT_KEY} must be {BUDGET_UNIT}")
    seed = body.get("seed")
    if isinstance(seed, bool) or not isinstance(seed, int):
        raise RecordError("seed must be an integer")
    tasks = sequence(body.get("sample_tasks"), "sample_tasks")
    return Manifest(
        run_id=text(body.get("run_id"), "run_id"),
        seed=seed,
        prereg_tag=text(body.get("prereg_tag"), "prereg_tag"),
        provenance=Provenance(
            product_digests=_texts(body.get("product_digests"), "product_digests"),
            shallnot_commit=text(body.get("shallnot_commit"), "shallnot_commit"),
            cli_versions=_texts(body.get("cli_versions"), "cli_versions"),
        ),
        corpus_lock_digest=text(body.get("corpus_lock_digest"), "corpus_lock_digest"),
        prices_lock_digest=text(body.get("prices_lock_digest"), "prices_lock_digest"),
        os_name=text(body.get("os"), "os"),
        toolchains=_texts(body.get("toolchains"), "toolchains"),
        harness_models={
            enum_of(Harness, k, "harness_models key"): text(v, f"harness_models.{k}")
            for k, v in mapping(body.get("harness_models"), "harness_models").items()
        },
        sample_tasks=tuple(text(t, "sample_tasks item") for t in tasks),
    )
