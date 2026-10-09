"""The real product in a test: the binaries and Shallnot of `.tools/bin`, a small project, and a fake gate-host session.

Nothing here starts `claude` or `codex`. `FakeGateSession` plays the part of the session that runs `prove-all`: it
drives the real module binaries through the real `fakehost` (persona `honest`, answers read from recordings) and
streams the events a Claude Code session would.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
from collections.abc import Callable, Mapping
from dataclasses import dataclass, field
from pathlib import Path
from typing import Final

from specbench.adapters.workspace_repo import IDENTITY
from specbench.application.ports import EventSink, GateSessionEnd, GateSessionSpec
from specbench.domain.product import PROOF_MODULES, ProofModule

REPOSITORY: Final = Path(__file__).resolve().parents[3]
BINARIES: Final = REPOSITORY / ".tools" / "bin"
UMBRELLA_PLUGIN: Final = REPOSITORY / ".tools" / "plugins" / "spec-proof"
PARCEL_FIXTURE: Final = REPOSITORY / "discpower" / "fixtures" / "prove" / "parcel"
PARCEL_ANSWERS: Final = Path(__file__).resolve().parents[1] / "fixtures" / "parcel_answers"
FAKEHOST_PACKAGE: Final = "./hostproto/hostprototest/cmd/fakehost"
ALIGNMENT_POLICY_FILE: Final = ".spec-proof/alignment.yaml"
SHALLNOT_CONFIG_FILE: Final = "shallnot.yaml"
LOWERED_ISOLATION_FLOOR: Final = {
    ALIGNMENT_POLICY_FILE: "policy:\n  min_isolation: inline\n",
    SHALLNOT_CONFIG_FILE: (
        "version: 1\nspecs:\n  - specs\ntests:\n  - tests\nresults:\n  - test-results/pytest.xml\n"
        "test_commands:\n  - pytest -o junit_family=xunit1 --junitxml=test-results/pytest.xml\n"
        "proofs:\n  min_isolation: inline\n"
    ),
}
"""A project that credits the answers of an inline host, which is what the fake host is, in the policy of alignment and in
the gate that composes the result."""
EDITED_FILE: Final = "parcel/rates.py"
EDIT: Final = "\n# reviewed\n"
SESSION_ID: Final = "5b0c2a64-6a3e-4f4e-9d0e-0a1f6c8d2b11"
RESOLVED_MODEL: Final = "claude-sonnet-5-5"
MAX_WAVES: Final = 12
RECORD_FLAG: Final = "--record "
ORACLE_FLAGS: Final = ("-oracle", "parcel")
EXIT_AWAITING_HOST: Final = 3
GIT: Final = "git"


def product_is_built() -> bool:
    needed = [BINARIES / name for name in ("shallnot", "spec-proof-verify", "spec-proof-ledger")]
    needed += [BINARIES / module.binary for module in PROOF_MODULES]
    return all(path.is_file() for path in needed) and UMBRELLA_PLUGIN.is_dir()


def build_fakehost(destination: Path) -> Path:
    """The product's fake host, built once under nice with two Go procs at most."""
    binary = destination / "fakehost"
    env = {**os.environ, "GOMAXPROCS": "2"}
    subprocess.run(
        [
            "nice",
            "-n",
            "10",
            "go",
            "build",
            "-C",
            str(REPOSITORY / "format"),
            "-p",
            "1",
            "-o",
            str(binary),
            FAKEHOST_PACKAGE,
        ],
        check=True,
        env=env,
        capture_output=True,
    )
    return binary


def git(tree: Path, *arguments: str) -> str:
    done = subprocess.run(
        [GIT, "-C", str(tree), *arguments], check=True, capture_output=True, text=True, env={**os.environ, **IDENTITY}
    )
    return done.stdout.strip()


def make_parcel_tree(destination: Path, extra_files: Mapping[str, str] | None = None) -> Path:
    """The parcel project at a base commit, with one uncommitted edit: what a coder session leaves behind.

    `extra_files` are written into the project before the base commit, as the project's own configuration would be.
    """
    shutil.copytree(PARCEL_FIXTURE, destination)
    for relative, content in (extra_files or {}).items():
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")
    git(destination, "init", "--initial-branch", "main")
    git(destination, "add", "--all")
    git(destination, "commit", "--message", "base")
    edited = destination / EDITED_FILE
    edited.write_text(edited.read_text() + EDIT)
    return destination


def stream_line(body: Mapping[str, object]) -> str:
    return json.dumps(body, separators=(",", ":"))


def init_event(modules: tuple[ProofModule, ...], omit_plugin: str | None) -> str:
    plugins = [{"name": "spec-proof", "path": "/p"}] + [{"name": m.plugin, "path": "/p"} for m in modules]
    servers = [{"name": m.server, "status": "connected"} for m in modules]
    return stream_line(
        {
            "type": "system",
            "subtype": "init",
            "session_id": SESSION_ID,
            "model": RESOLVED_MODEL,
            "plugins": [p for p in plugins if p["name"] != omit_plugin],
            "mcp_servers": servers,
            "tools": ["Agent"],
        }
    )


def launch_events(number: int, module: ProofModule, ticket: str) -> list[str]:
    tool_use_id = f"toolu_{number:04d}"
    launch = {
        "type": "tool_use",
        "id": tool_use_id,
        "name": "Agent",
        "input": {"subagent_type": f"{module.plugin}:judge", "prompt": f"ticket {ticket}"},
    }
    usage = {"input_tokens": 60, "output_tokens": 40, "cache_read_input_tokens": 5, "cache_creation_input_tokens": 3}
    return [
        stream_line(
            {"type": "assistant", "message": {"role": "assistant", "content": [launch]}, "parent_tool_use_id": None}
        ),
        stream_line(
            {
                "type": "assistant",
                "message": {
                    "role": "assistant",
                    "content": [{"type": "text", "text": "receipt"}],
                    "model": RESOLVED_MODEL,
                    "usage": usage,
                },
                "parent_tool_use_id": tool_use_id,
            }
        ),
    ]


RESULT_EVENT: Final = stream_line(
    {
        "type": "result",
        "subtype": "success",
        "is_error": False,
        "session_id": SESSION_ID,
        "result": "done",
        "num_turns": 3,
    }
)


class SessionKilled(Exception):
    """The fake session stops mid-gate, as a window close would end the real one."""


@dataclass
class FakeGateSession:
    """Plays `prove-all` with the product's fake host: prove, answer the tickets from recordings, resume, until done."""

    fakehost: Path
    binaries: Path
    answers: Path
    omit_plugin: str | None = None
    kill_after_waves: int | None = None
    exit_code: int = 0
    kill_round_after: int | None = None
    answer_rounds: bool = True
    oracle_for_all: bool = False
    specs: list[GateSessionSpec] = field(default_factory=lambda: [])
    answered: int = 0
    waves: int = 0
    round_answers: int = 0

    def run(self, spec: GateSessionSpec, on_event: EventSink) -> GateSessionEnd:
        self.specs.append(spec)
        modules = self._modules_of(spec)
        if not on_event(init_event(modules, self.omit_plugin)):
            return GateSessionEnd(self.exit_code)
        prompt = spec.argv[spec.argv.index("-p") + 1]
        env = {**os.environ, **spec.env}
        if RECORD_FLAG not in prompt:
            return self._answer_round(modules, spec.cwd, env, on_event)
        record = Path(prompt.split(RECORD_FLAG)[1].split("`")[0])
        for module in modules:
            if not self._prove_module(module, record, spec.cwd, env, on_event):
                return GateSessionEnd(self.exit_code)
        on_event(RESULT_EVENT)
        return GateSessionEnd(self.exit_code)

    def is_round(self, spec: GateSessionSpec) -> bool:
        return RECORD_FLAG not in spec.argv[spec.argv.index("-p") + 1]

    def _answer_round(
        self, modules: tuple[ProofModule, ...], tree: Path, env: Mapping[str, str], on_event: EventSink
    ) -> GateSessionEnd:
        """A CI round trip: answer the pulled tickets of every module with the recordings, then the parcel oracle."""
        for module in modules if self.answer_rounds else ():
            if not self._answer_wave(module, tree, env, on_event, ORACLE_FLAGS, in_round=True):
                return GateSessionEnd(self.exit_code)
        on_event(RESULT_EVENT)
        return GateSessionEnd(self.exit_code)

    def _oracle_flags(self) -> tuple[str, ...]:
        return ORACLE_FLAGS if self.oracle_for_all else ()

    def _modules_of(self, spec: GateSessionSpec) -> tuple[ProofModule, ...]:
        directories = {spec.argv[i + 1] for i, word in enumerate(spec.argv) if word == "--plugin-dir"}
        return tuple(m for m in PROOF_MODULES if str(REPOSITORY / m.plugin_directory) in directories)

    def _prove_module(
        self, module: ProofModule, record: Path, tree: Path, env: Mapping[str, str], on_event: EventSink
    ) -> bool:
        binary = str(self.binaries / module.binary)
        prove = [binary, "prove", "--changed", "--record", str(record)]
        code = subprocess.run(prove, cwd=tree, env=env, capture_output=True, check=False).returncode
        for _ in range(MAX_WAVES):
            if code != EXIT_AWAITING_HOST:
                return True
            if not self._answer_wave(module, tree, env, on_event):
                return False
            self.waves += 1
            if self.kill_after_waves == self.waves:
                raise SessionKilled
            code = subprocess.run([*prove, "--resume"], cwd=tree, env=env, capture_output=True, check=False).returncode
        return True

    def _answer_wave(
        self,
        module: ProofModule,
        tree: Path,
        env: Mapping[str, str],
        on_event: EventSink,
        extra_flags: tuple[str, ...] = (),
        in_round: bool = False,
    ) -> bool:
        host = [str(self.fakehost), "-persona", "honest", "-module-bin", str(self.binaries / module.binary)]
        done = subprocess.run(
            [*host, "-recordings", str(self.answers), *(extra_flags or self._oracle_flags())],
            cwd=tree,
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
        for line in done.stdout.splitlines():
            outcome = json.loads(line)
            if not outcome["submitted"]:
                continue
            self.answered += 1
            if in_round:
                self.round_answers += 1
                if self.kill_round_after == self.round_answers:
                    raise SessionKilled
            for event in launch_events(self.answered, module, outcome["ticket"]):
                if not on_event(event):
                    return False
        return True


Behaviour = Callable[[], None]
