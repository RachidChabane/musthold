"""Where everything of the run lives under $DEMOS1_WORK (PREREG 11), and the container names of PREREG 13."""

from __future__ import annotations

from dataclasses import dataclass
from enum import StrEnum
from pathlib import PurePosixPath
from typing import Final

from demos1.domain.container import CommandEnvironment, XdgDirectories
from demos1.domain.lists import ListName
from demos1.domain.machine import MachineMounts

MANIFEST: Final = "manifest.json"
DEVIATIONS: Final = "DEVIATIONS.md"
SELECTION: Final = "selection"
RESPONSES: Final = "responses"
FRAME: Final = "frame.jsonl"
LISTS: Final = "lists.json"
OUTSIDE_WINDOW: Final = "outside-window.jsonl"
ATTEMPTS: Final = "attempts"
ATTEMPT_RECORD: Final = "attempt.json"
PRODUCT_BIN: Final = "bin"
IMAGE: Final = "image"
PRODUCT_BUILD: Final = "build.json"
IMAGE_BUILD: Final = "image.json"
STOPPED_SELECTION_PREFIX: Final = "selection-stopped-"
SCAN_DIRECTORY: Final = ".spec-proof/scan"


class SetupStage(StrEnum):
    """The setup stages of PREREG 8.2 to 8.4, in the order they run; each names its containers."""

    CLONE = "clone"
    TOOLCHAIN = "toolchain"
    INSTALL = "install"
    CLEAN_TREE_AFTER_INSTALL = "clean-tree-1"
    SUITE_FIRST = "suite-1"
    SUITE_SECOND = "suite-2"
    CLEAN_TREE_AFTER_SUITE = "clean-tree-2"


SETUP_ORDER: Final = tuple(SetupStage)


def attempt_id(name: ListName, rank: int) -> str:
    """`<stratum>-<ecosystem>-<rank>`, for example `B-go-03`."""
    return f"{name}-{rank:02d}"


def run_suffix(run: int) -> str:
    return f"r{run}"


def setup_container(attempt: str, stage: SetupStage, run: int) -> str:
    return f"{attempt}-{stage.value}-{run_suffix(run)}"


def scan_container(attempt: str, scan: str, run: int) -> str:
    return f"{attempt}-{scan}-{run_suffix(run)}"


def replay_container(attempt: str, scan: str, finding: int, run: int) -> str:
    return f"{attempt}-{scan}-replay-{finding}-{run_suffix(run)}"


def container_prefix(attempt: str) -> str:
    """The prefix every container of an attempt shares, for `docker ps -a --filter name=<id>-`."""
    return f"{attempt}-"


@dataclass(frozen=True)
class WorkLayout:
    """$DEMOS1_WORK, and beside it the product directory whose binaries the machine mounts read-only."""

    root: PurePosixPath
    product_root: PurePosixPath

    def mounts(self) -> MachineMounts:
        return MachineMounts(self.root, self.product_bin)

    @property
    def manifest(self) -> PurePosixPath:
        return self.root / MANIFEST

    @property
    def deviations(self) -> PurePosixPath:
        return self.root / DEVIATIONS

    @property
    def selection(self) -> PurePosixPath:
        return self.root / SELECTION

    @property
    def responses(self) -> PurePosixPath:
        return self.selection / RESPONSES

    @property
    def frame(self) -> PurePosixPath:
        return self.selection / FRAME

    @property
    def lists(self) -> PurePosixPath:
        return self.selection / LISTS

    @property
    def outside_window(self) -> PurePosixPath:
        return self.selection / OUTSIDE_WINDOW

    @property
    def product_bin(self) -> PurePosixPath:
        return self.product_root / PRODUCT_BIN

    @property
    def product_build(self) -> PurePosixPath:
        return self.product_root / PRODUCT_BUILD

    @property
    def image_build(self) -> PurePosixPath:
        return self.root / IMAGE / IMAGE_BUILD

    def stopped_selection(self, index: int) -> PurePosixPath:
        return self.root / f"{STOPPED_SELECTION_PREFIX}{index}"

    def attempt(self, attempt: str) -> AttemptLayout:
        return AttemptLayout(self.root / ATTEMPTS / attempt, attempt)


@dataclass(frozen=True)
class AttemptLayout:
    """The attempt directory, mounted read-write at the same path in every container of the attempt."""

    root: PurePosixPath
    attempt: str

    @property
    def record(self) -> PurePosixPath:
        return self.root / ATTEMPT_RECORD

    @property
    def home(self) -> PurePosixPath:
        return self.root / "home"

    @property
    def gitconfig(self) -> PurePosixPath:
        return self.home / ".gitconfig"

    @property
    def clone(self) -> PurePosixPath:
        return self.root / "clone"

    @property
    def cache(self) -> PurePosixPath:
        return self.root / "cache"

    @property
    def logs(self) -> PurePosixPath:
        return self.root / "logs"

    @property
    def uv_python(self) -> PurePosixPath:
        return self.cache / "uv" / "python"

    def setup_prefix(self, stage: SetupStage, run: int) -> PurePosixPath:
        return self.logs / f"{stage.value}-{run_suffix(run)}"

    @property
    def tmp_root(self) -> PurePosixPath:
        """The directory that holds the TMPDIR of every setup command, and nothing else."""
        return self.root / "tmp"

    def setup_tmp(self, stage: SetupStage, run: int) -> PurePosixPath:
        return self.tmp_root / f"{stage.value}-{run_suffix(run)}"

    def suite_output(self, run: int, suffix: str) -> PurePosixPath:
        return self.root / f"suite-{run}{suffix}"

    def scan_dir(self, scan: str) -> PurePosixPath:
        return self.root / "scans" / scan

    def scan_run(self, scan: str, run: int) -> PurePosixPath:
        return self.scan_dir(scan) / run_suffix(run)

    def moved(self, scan: str) -> PurePosixPath:
        return self.scan_dir(scan) / "moved"

    def replay_run(self, scan: str, finding: int, run: int) -> PurePosixPath:
        return self.scan_dir(scan) / "replays" / str(finding) / run_suffix(run)

    def environment(self, tmpdir: PurePosixPath) -> CommandEnvironment:
        go = self.cache / "go"
        uv = self.cache / "uv"
        return CommandEnvironment(
            home=str(self.home),
            tmpdir=str(tmpdir),
            go_path=str(go / "path"),
            go_mod_cache=str(go / "mod"),
            go_cache=str(go / "build"),
            uv_cache=str(uv / "cache"),
            uv_python=str(self.uv_python),
        )


def run_directories(run_dir: PurePosixPath) -> tuple[PurePosixPath, XdgDirectories]:
    """The new TMPDIR and XDG directories of one run of a scan or a replay (PREREG 9.2)."""
    return run_dir / "tmp", XdgDirectories(str(run_dir / "xdg-cache"), str(run_dir / "xdg-state"))


def run_prefix(run_dir: PurePosixPath) -> PurePosixPath:
    return run_dir / "run"


def gitconfig_text(clone: PurePosixPath) -> str:
    """PREREG 8.2: the attempt's home holds only `[safe] directory = <clone>`."""
    return f"[safe]\n\tdirectory = {clone}\n"
