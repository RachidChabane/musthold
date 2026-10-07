"""`FeasibilityProbe` over a local mirror: build the environment, run the suite offline at merge and at base.

For one candidate it establishes, with no model call:

- the true base, the first parent of the merge commit, and that the merge's diff holds the pull request's files;
- a native environment built from the base tree's lockfiles;
- the whole suite at the merge commit and at the base tree overlaid with the merge's test files, both with outbound
  network denied, and from them U (fail to pass) and R (pass to pass).

Verdicts are cached in the work directory, so a rerun resumes where it stopped.
"""

from __future__ import annotations

import json
import shutil
import subprocess
from collections.abc import Callable, Mapping
from pathlib import Path
from typing import Final

from specbench.adapters.env_builder import (
    PYTHON_ORACLE_TOOLING,
    EnvironmentBuildError,
    go_offline_environment,
    python_executable,
)
from specbench.adapters.git_history import GitError, GitHistory, GitMirrors
from specbench.adapters.git_trees import GitTreeFactory, TreeBuildError
from specbench.adapters.oracle_runners import GoSuiteRunner, PytestSuiteRunner
from specbench.application.ports import EnvBuilder, EnvRequest, ProcessRunner, SuiteRunner, TreeSpec
from specbench.domain.candidates import Candidate, task_id_for
from specbench.domain.canonical import JsonValue, canonical_json, sha256_hex
from specbench.domain.changed_tests import added_lines, touched_by_diff
from specbench.domain.ground_tests import MIN_SUITE_PASS_PERCENT, GroundTests, partition_ground_tests
from specbench.domain.jsonread import mapping
from specbench.domain.languages import Language, is_test_path
from specbench.domain.mining import Rejection, Stage, has_lockfiles, heavy_dependency, lockfiles_for
from specbench.domain.selection import ProbedTask
from specbench.domain.task import Toolchain
from specbench.domain.toolchain import PYTHON_VERSION_FILE, declared_requirement, format_version, python_for

PROBE_DIRECTORY: Final = "probe"
ENVIRONMENT_DIRECTORY: Final = "envs"
VERDICT_SUFFIX: Final = ".verdict.json"
TESTS_SUFFIX: Final = ".tests.json"
MAX_REASON_LENGTH: Final = 300
TOOLCHAIN_NAME: Final = {Language.PYTHON: "python", Language.GO: "go"}
SUITE_SOURCE_SUFFIX: Final = {Language.PYTHON: ".py", Language.GO: "_test.go"}
ENCODING: Final = "utf-8"

SuiteRunnerFactory = Callable[[Language, Path], SuiteRunner]
InterpreterFactory = Callable[[str], Path]
"""The interpreter of a `major.minor` Python version, installed on demand."""


def suite_runner_for(language: Language, env_dir: Path, runner: ProcessRunner) -> SuiteRunner:
    """The whole-suite runner of a language, bound to the task environment and an offline process runner."""
    if language is Language.PYTHON:
        return PytestSuiteRunner(python_executable(env_dir), runner)
    return GoSuiteRunner(runner, go_offline_environment(env_dir))


def verdict_path(work: Path, candidate: Candidate) -> Path:
    return work / PROBE_DIRECTORY / f"{task_id_for(candidate)}{VERDICT_SUFFIX}"


def environment_directory(work: Path, task_id: str) -> Path:
    return work / ENVIRONMENT_DIRECTORY / task_id


def _short(message: str) -> str:
    """The end of the message, where tools state the cause after any warnings."""
    flat = " ".join(message.split())
    return flat if len(flat) <= MAX_REASON_LENGTH else "..." + flat[-(MAX_REASON_LENGTH - 3) :]


class OfflineProbe:
    def __init__(
        self,
        mirrors: GitMirrors,
        history_of: Callable[[Path], GitHistory],
        builder: EnvBuilder,
        suites: SuiteRunnerFactory,
        work: Path,
        interpreters: InterpreterFactory,
    ) -> None:
        self._mirrors = mirrors
        self._history_of = history_of
        self._builder = builder
        self._suites = suites
        self._work = work
        self._interpreters = interpreters

    def probe(self, candidate: Candidate) -> ProbedTask | Rejection:
        path = verdict_path(self._work, candidate)
        if path.is_file():
            return read_verdict(candidate, json.loads(path.read_text(encoding=ENCODING)))
        outcome = self._probe(candidate)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(canonical_json(_verdict_record(outcome)), encoding=ENCODING)
        if isinstance(outcome, Rejection):
            shutil.rmtree(self._environment_dir(candidate), ignore_errors=True)
        return outcome

    def _environment_dir(self, candidate: Candidate) -> Path:
        return environment_directory(self._work, task_id_for(candidate))

    def _probe(self, candidate: Candidate) -> ProbedTask | Rejection:
        try:
            clone = self._mirrors.ensure(candidate.repository)
        except GitError as error:
            return Rejection(Stage.ENVIRONMENT_BUILD, _short(str(error)))
        history = self._history_of(clone)
        trees = GitTreeFactory(clone)
        located = self._locate(candidate, history)
        if isinstance(located, Rejection):
            return located
        base, changed = located
        test_paths = sorted(p for p in changed if is_test_path(candidate.language, p))
        overlay = {
            p: text.encode() for p in test_paths if (text := history.file_at(candidate.merge_sha, p)) is not None
        }
        try:
            return self._run(candidate, history, trees, base, test_paths, overlay)
        except (EnvironmentBuildError, TreeBuildError, GitError) as error:
            return Rejection(Stage.ENVIRONMENT_BUILD, _short(str(error)))
        except subprocess.TimeoutExpired:
            return Rejection(Stage.OFFLINE_TESTS, "the suite exceeded its time limit")

    @staticmethod
    def _locate(candidate: Candidate, history: GitHistory) -> tuple[str, frozenset[str]] | Rejection:
        if not history.has_commit(candidate.merge_sha):
            return Rejection(Stage.MERGE_DIFF, "the merge commit is not in the mirror")
        try:
            base = history.first_parent(candidate.merge_sha)
            changed = history.changed_paths(base, candidate.merge_sha)
        except GitError as error:
            return Rejection(Stage.MERGE_DIFF, _short(str(error)))
        expected = {*candidate.non_test_files, *candidate.test_files}
        if not expected <= changed:
            missing = sorted(expected - changed)
            return Rejection(Stage.MERGE_DIFF, f"the merge's first-parent diff lacks {missing[:3]}")
        return base, changed

    def _run(
        self,
        candidate: Candidate,
        history: GitHistory,
        trees: GitTreeFactory,
        base: str,
        test_paths: list[str],
        overlay: Mapping[str, bytes],
    ) -> ProbedTask | Rejection:
        language = candidate.language
        env_dir = self._environment_dir(candidate)
        tooling = PYTHON_ORACLE_TOOLING if language is Language.PYTHON else ()
        with trees.materialize(TreeSpec(base)) as base_tree:
            if not has_lockfiles(language, frozenset(path.name for path in base_tree.iterdir())):
                return Rejection(Stage.ENVIRONMENT_BUILD, "the lockfiles are absent from the base commit")
            heavy = _heavy_lock(language, base_tree)
            if heavy is not None:
                return Rejection(Stage.ENVIRONMENT_BUILD, f"the lockfile pulls {heavy}, beyond the machine's budget")
            python = self._python_of(language, base_tree)
            built = self._builder.build(EnvRequest(language, base_tree, env_dir, tooling, python))
        suite = self._suites(language, env_dir)
        with trees.materialize(TreeSpec(candidate.merge_sha)) as merge_tree:
            merge_outcomes = suite.run_all(merge_tree)
        with trees.materialize(TreeSpec(base, overlay=overlay)) as overlaid_tree:
            overlaid_outcomes = suite.run_all(overlaid_tree)
        sources = {
            p: text
            for p in test_paths
            if p.endswith(SUITE_SOURCE_SUFFIX[language]) and (text := history.file_at(candidate.merge_sha, p))
        }
        diff = added_lines(history.unified_diff(base, candidate.merge_sha, test_paths))
        ground = partition_ground_tests(touched_by_diff(language, sources, diff), merge_outcomes, overlaid_outcomes)
        verdict = _ground_verdict(ground)
        if verdict is not None:
            return verdict
        self._store_tests(candidate, ground)
        return ProbedTask(
            candidate=candidate,
            base_sha=base,
            toolchain=Toolchain(TOOLCHAIN_NAME[language], built.toolchain_version),
            lockfile_digests=built.lockfile_digests,
            u_count=len(ground.u),
            r_count=len(ground.r),
            merge_total=ground.merge_total,
            ground_digest=_ground_digest(ground),
        )

    def _python_of(self, language: Language, tree: Path) -> Path | None:
        """The interpreter the upstream's own pin or `requires-python` selects; none for a Go task."""
        if language is not Language.PYTHON:
            return None

        def read(name: str) -> str | None:
            path = tree / name
            return path.read_text(encoding=ENCODING, errors="replace") if path.is_file() else None

        requires = declared_requirement(read("uv.lock"), read("pyproject.toml"))
        return self._interpreters(format_version(python_for(read(PYTHON_VERSION_FILE), requires)))

    def _store_tests(self, candidate: Candidate, ground: GroundTests) -> None:
        path = self._work / PROBE_DIRECTORY / f"{task_id_for(candidate)}{TESTS_SUFFIX}"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(canonical_json({"r": list(ground.r), "u": list(ground.u)}), encoding=ENCODING)


def _heavy_lock(language: Language, tree: Path) -> str | None:
    for name in lockfiles_for(language):
        lock = tree / name
        if lock.is_file():
            found = heavy_dependency(lock.read_text(encoding=ENCODING, errors="replace"))
            if found is not None:
                return found
    return None


def _ground_verdict(ground: GroundTests) -> Rejection | None:
    if ground.merge_total == 0:
        return Rejection(Stage.OFFLINE_TESTS, "no test ran at the merge commit")
    if ground.merge_pass_percent < MIN_SUITE_PASS_PERCENT:
        return Rejection(
            Stage.OFFLINE_TESTS,
            f"{ground.merge_passed} of {ground.merge_total} tests pass offline at merge, "
            f"at least {MIN_SUITE_PASS_PERCENT} % required",
        )
    if not ground.u:
        return Rejection(Stage.NO_FAIL_TO_PASS, "no test the pull request touched fails at base and passes at merge")
    return None


def _ground_digest(ground: GroundTests) -> str:
    return sha256_hex(canonical_json({"r": list(ground.r), "u": list(ground.u)}).encode())


def _verdict_record(outcome: ProbedTask | Rejection) -> dict[str, JsonValue]:
    if isinstance(outcome, Rejection):
        return {"reason": outcome.reason, "rejected": outcome.stage.value}
    return {
        "base_sha": outcome.base_sha,
        "ground_digest": outcome.ground_digest,
        "lockfile_digests": dict(sorted(outcome.lockfile_digests.items())),
        "merge_total": outcome.merge_total,
        "r_count": outcome.r_count,
        "toolchain": {"name": outcome.toolchain.name, "version": outcome.toolchain.version},
        "u_count": outcome.u_count,
    }


def read_verdict(candidate: Candidate, record: Mapping[str, JsonValue]) -> ProbedTask | Rejection:
    if "rejected" in record:
        return Rejection(Stage(str(record["rejected"])), str(record["reason"]))
    toolchain = mapping(record.get("toolchain"), "toolchain")
    digests = mapping(record.get("lockfile_digests"), "lockfile_digests")
    return ProbedTask(
        candidate=candidate,
        base_sha=str(record["base_sha"]),
        toolchain=Toolchain(str(toolchain["name"]), str(toolchain["version"])),
        lockfile_digests={str(k): str(v) for k, v in digests.items()},
        u_count=int(str(record["u_count"])),
        r_count=int(str(record["r_count"])),
        merge_total=int(str(record["merge_total"])),
        ground_digest=str(record["ground_digest"]),
    )
