"""PREREG 2: the facts of the product, the image, the engine and the host, gathered for the manifest and each attempt."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import PurePosixPath
from typing import Final, Protocol, cast

from demos1.application.command_runner import SETUP_SLOTS, CommandPlan, CommandRunner, PlannedRun
from demos1.application.ports import Files, Machine
from demos1.domain import probe
from demos1.domain.container import CommandEnvironment, ContainerCommand, XdgDirectories
from demos1.domain.environment import Fact, go_version_word, uv_version_word
from demos1.domain.jsonvalue import JsonObject, as_object, optional_text
from demos1.domain.layout import WorkLayout
from demos1.domain.records import CommandRecord

PROBE_DIRECTORY: Final = "probe"
PROBE_NAME_PREFIX: Final = "demos-1-probe-"
BINARIES: Final = ("spec-proof", "spec-proof-dp")


class HostFacts(Protocol):
    def host(self) -> Mapping[str, str]:
        """The host's OS name, version and architecture, and the gh version."""
        ...

    def engine(self) -> Mapping[str, str]:
        """The Docker engine version and its virtual machine's memory and CPUs."""
        ...


class ProbeUnavailable(RuntimeError):
    """The product or the image is not built, or the probe container did not report."""


@dataclass(frozen=True)
class Builds:
    """What `build-product` and `build-image` recorded."""

    product: JsonObject
    image: JsonObject

    @property
    def image_id(self) -> str:
        value = optional_text(self.image, "image_id")
        if value is None:
            raise ProbeUnavailable("image.json names no image")
        return value

    @property
    def product_commit(self) -> str:
        value = optional_text(self.product, "commit")
        if value is None:
            raise ProbeUnavailable("build.json names no commit")
        return value


def read_builds(files: Files, work: WorkLayout) -> Builds:
    product = files.read_json(work.product_build)
    image = files.read_json(work.image_build)
    if product is None:
        raise ProbeUnavailable(f"no product build at {work.product_build}: run build-product")
    if image is None:
        raise ProbeUnavailable(f"no image build at {work.image_build}: run build-image")
    return Builds(as_object(product), as_object(image))


class EnvironmentProbe:
    def __init__(
        self, files: Files, host: HostFacts, machine: Machine, runner: CommandRunner, uid: int, gid: int
    ) -> None:
        self._machine = machine
        self._files = files
        self._host = host
        self._runner = runner
        self._uid = uid
        self._gid = gid

    def facts(self, work: WorkLayout, builds: Builds, stamp: str) -> dict[str, str]:
        directory = work.root / PROBE_DIRECTORY / stamp
        container = self._probe(directory, work, builds, stamp)
        facts: dict[str, str] = {}
        facts.update(self._host.host())
        facts.update(self._host.engine())
        facts.update(self._machine.facts())
        facts[Fact.IMAGE_ID] = builds.image_id
        facts[Fact.GO_SOURCE_DIGEST] = optional_text(builds.image, "go_digest") or ""
        facts[Fact.UV_SOURCE_DIGEST] = optional_text(builds.image, "uv_digest") or ""
        facts[Fact.PRODUCT_COMMIT] = builds.product_commit
        for binary, fact in zip(BINARIES, (Fact.SPEC_PROOF_SHA256, Fact.SPEC_PROOF_DP_SHA256), strict=True):
            facts[fact] = self._files.sha256(work.product_bin / binary) or ""
        facts.update(container)
        return facts

    def _read(self, directory: PurePosixPath, name: str) -> str:
        return (self._files.read_text(directory / name) or "").strip()

    def _probe(self, directory: PurePosixPath, work: WorkLayout, builds: Builds, stamp: str) -> dict[str, str]:
        name = f"{PROBE_NAME_PREFIX}{stamp}"
        tmpdir = directory / "tmp"

        def planned(run: int) -> PlannedRun:
            command = ContainerCommand(
                name=f"{name}-r{run}",
                image=builds.image_id,
                attempt_dir=str(directory),
                binaries_dir=str(work.product_bin),
                workdir=str(directory),
                network=False,
                environment=_probe_environment(directory, tmpdir),
                argv=probe.probe_command(str(directory)),
                record_prefix=str(directory / f"r{run}"),
                limit_seconds=None,
                uid=self._uid,
                gid=self._gid,
            )
            return PlannedRun(command, (directory, tmpdir), directory / f"r{run}")

        record = CommandRecord()
        self._runner.settle(CommandPlan(SETUP_SLOTS, None, planned), record)
        final = record.final()
        if final is None or final.exit_code != 0:
            raise ProbeUnavailable(f"the probe container {name} did not report; see {directory}")
        return {
            Fact.IMAGE_GO_VERSION: go_version_word(self._read(directory, probe.GO_VERSION_FILE)) or "",
            Fact.IMAGE_UV_VERSION: uv_version_word(self._read(directory, probe.UV_VERSION_FILE)) or "",
            Fact.IMAGE_GIT_VERSION: probe.git_version_word(self._read(directory, probe.GIT_VERSION_FILE)) or "",
            Fact.IMAGE_OS: probe.pretty_name(self._read(directory, probe.OS_RELEASE_FILE)) or "",
            Fact.IMAGE_ARCH: self._read(directory, probe.ARCH_FILE),
            Fact.PRODUCT_VERSION: self._read(directory, probe.PRODUCT_VERSION_FILE),
            Fact.UV_CPYTHON: json.dumps(
                probe.cpython_by_minor(self._read(directory, probe.UV_PYTHON_FILE)), sort_keys=True
            ),
        }


def _probe_environment(directory: PurePosixPath, tmpdir: PurePosixPath) -> dict[str, str]:
    cache = directory / "cache"
    environment = CommandEnvironment(
        home=str(directory),
        tmpdir=str(tmpdir),
        go_path=str(cache / "go" / "path"),
        go_mod_cache=str(cache / "go" / "mod"),
        go_cache=str(cache / "go" / "build"),
        uv_cache=str(cache / "uv" / "cache"),
        uv_python=str(cache / "uv" / "python"),
    )
    return environment.variables(offline=True, xdg=XdgDirectories(str(cache / "xdg-cache"), str(cache / "xdg-state")))


def uv_releases(facts: Mapping[str, str]) -> dict[str, str]:
    """The CPython release per minor the probe read from the image's uv."""
    decoded: object = json.loads(facts.get(Fact.UV_CPYTHON, "{}"))
    if not isinstance(decoded, dict):
        return {}
    return {str(key): str(value) for key, value in cast(dict[object, object], decoded).items()}
