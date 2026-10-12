"""The `attempt` verb: the facts checked against the manifest, then the attempts of PREREG 8.1 in order."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from typing import Final

from demos1.application.attempts import Attempts, AttemptSettings, UndefinedStage
from demos1.application.command_runner import CommandRunner, RefusalBudget, WaitBoundReached
from demos1.application.environment import EnvironmentProbe, read_builds, uv_releases
from demos1.application.machine_session import machine_running
from demos1.application.ports import Clock, ContainerEngine, DeviationLog, Files, Machine, OperatorStopped
from demos1.application.select_run import STAMP_FORMAT, Refused, manifest_facts
from demos1.domain.container import MEMORY
from demos1.domain.deviations import Deviation, DeviationKind
from demos1.domain.environment import drift
from demos1.domain.jsonvalue import JsonObject, as_object
from demos1.domain.layout import WorkLayout
from demos1.domain.lists import Lists
from demos1.domain.machine import ATTEMPT_CAP_BYTES, MIN_FREE_BYTES
from demos1.domain.records import AttemptRecord
from demos1.domain.summary import PICKS_PER_ECOSYSTEM

HTTPS_PROTOCOL: Final = "https"


def github_clone_url(full_name: str) -> str:
    return f"https://github.com/{full_name}.git"


@dataclass(frozen=True)
class AttemptContext:
    work: WorkLayout
    uid: int
    gid: int
    clone_url: Callable[[str], str] = github_clone_url
    clone_protocol: str = HTTPS_PROTOCOL
    memory: str = MEMORY
    attempt_cap_bytes: int = ATTEMPT_CAP_BYTES
    min_free_bytes: int = MIN_FREE_BYTES


class AttemptVerb:
    def __init__(
        self,
        files: Files,
        engine: ContainerEngine,
        clock: Clock,
        deviations: DeviationLog,
        probe: EnvironmentProbe,
        runner_of: Callable[[Callable[[], None]], CommandRunner],
        machine: Machine,
        refusals: RefusalBudget,
    ) -> None:
        self._machine = machine
        self._refusals = refusals
        self._files = files
        self._engine = engine
        self._clock = clock
        self._deviations = deviations
        self._probe = probe
        self._runner_of = runner_of

    def run(self, context: AttemptContext) -> list[AttemptRecord]:
        """PREREG 13: the machine starts before the first container and stops whatever the verb's exit."""
        with machine_running(self._machine, context.work.mounts()):
            return self._run(context)

    def _run(self, context: AttemptContext) -> list[AttemptRecord]:
        work = context.work
        manifest_document = self._files.read_json(work.manifest)
        lists_document = self._files.read_json(work.lists)
        if manifest_document is None or lists_document is None:
            raise Refused("select has not run: the manifest or the lists are missing")
        manifest = as_object(manifest_document)
        lists = Lists.from_json(as_object(lists_document))
        builds = read_builds(self._files, work)
        facts = self._probe.facts(work, builds, self._clock.now().strftime(STAMP_FORMAT))
        for mismatch in drift(manifest_facts(manifest), facts):
            self._deviate(
                DeviationKind.TOOL_VERSION,
                None,
                f"PREREG 2: {mismatch.fact} stays as recorded at select ({mismatch.expected}).",
                f"attempt observed {mismatch.observed}.",
            )
        environment: JsonObject = {
            "product_tag": manifest.get("product_tag"),
            "product_commit": manifest.get("product_commit"),
            "facts": dict(facts),
        }
        settings = AttemptSettings(
            work=work,
            image=builds.image_id,
            binaries_dir=str(context.work.product_bin),
            uid=context.uid,
            gid=context.gid,
            clone_url=context.clone_url,
            clone_protocol=context.clone_protocol,
            uv_releases=uv_releases(facts),
            environment=environment,
            memory=context.memory,
            attempt_cap_bytes=context.attempt_cap_bytes,
            min_free_bytes=context.min_free_bytes,
        )
        attempts = Attempts(
            settings, self._engine, self._files, self._clock, self._deviations, self._runner_of, self._refusals
        )
        try:
            return attempts.run_all(lists, PICKS_PER_ECOSYSTEM)
        except WaitBoundReached as reached:
            self._deviate(
                DeviationKind.WAIT_BOUND,
                attempts.current,
                "PREREG 9.3: refusals that add up to 6 hours within one invocation stop the verb.",
                f"{reached}; the next invocation resumes at the same command.",
            )
            raise
        except OperatorStopped:
            self._deviate(
                DeviationKind.DEPARTURE,
                attempts.current,
                "PREREG 9.3: a container command runs until it exits, its cap, its memory limit or the guard.",
                "the verb was interrupted; its container was stopped, inspected and removed, and the run recorded.",
            )
            raise
        except UndefinedStage as undefined:
            self._deviate(
                DeviationKind.UNDEFINED_STAGE,
                attempts.current,
                "PREREG 14: a stage this document does not define is a deviation.",
                str(undefined),
            )
            raise

    def _deviate(self, kind: DeviationKind, attempt: str | None, says: str, happened: str) -> None:
        self._deviations.append(Deviation(self._clock.now(), kind, attempt, says, happened))
