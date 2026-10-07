"""In-memory doubles of the run-side ports: the run store, the clock, the transport, the servers, the probes and the
builders, and the rig that assembles them into the dependencies of a model block."""

from __future__ import annotations

import json
from collections.abc import Callable, Collection, Mapping, Sequence
from dataclasses import dataclass
from datetime import datetime, timedelta
from pathlib import Path
from types import TracebackType
from typing import Final

from calibration.application.block import BlockDeps, BlockOutcome, ModelBlock
from calibration.application.build_ports import AlignmentDriver, BuiltTools
from calibration.application.ports import (
    HeavyWorkReport,
    MemoryReading,
    ProvenanceReading,
    ServerCard,
    ShutdownReport,
)
from calibration.domain.canonical import JsonValue
from calibration.domain.jsonread import json_object, mapping
from calibration.domain.models import Flavour, ModelSpec, model_by_id
from calibration.domain.ports import (
    DecodedAnswer,
    DecodeOutcome,
    DecodeRequest,
    EndpointDigest,
    HttpReply,
    ProductDefaults,
    ScreenReading,
    ScreenReadRequest,
    ThresholdCheck,
    TransportFailure,
)
from calibration.domain.server_wire import CHOICE
from calibration.domain.window import RunWindow
from support.fakes import FakeClock, FakeSleeper
from support.run_directory_seed import (
    CANARY_CLOSED_TEXT,
    ServerBehaviour,
    digest_of,
    lock_record,
    pinned_version,
    request_ids,
    seed_run,
)

EVENING: Final = datetime(2026, 10, 7, 22, 0, 0)
MORNING: Final = datetime(2026, 10, 8, 8, 0, 0)
DEFAULT_CONTEXT_TOKENS: Final = 4096
STORE_ROOT: Final = Path("/memory-run-directory")
CLEAN_SHUTDOWN: Final = ShutdownReport(unloaded=True, models_after_unload=(), server_stopped=True, port_free=True)
UNCLEAN_SHUTDOWN: Final = ShutdownReport(
    unloaded=True, models_after_unload=("loaded",), server_stopped=True, port_free=False
)
CLEAR_REPORT: Final = HeavyWorkReport((), (), ())
BUSY_REPORT: Final = HeavyWorkReport(("ollama runner",), (), ())
ROOMY_MEMORY: Final = MemoryReading(free_percent=60, pressure_normal=True)
TIGHT_MEMORY: Final = MemoryReading(free_percent=10, pressure_normal=True)


class MemoryRunStore:
    """Implements `RunStore` over a dictionary and logs every write, so a test can assert their order."""

    def __init__(self) -> None:
        self.files: dict[str, bytes] = {}
        self.writes: list[str] = []

    @property
    def root(self) -> Path:
        return STORE_ROOT

    def exists(self, relative: str) -> bool:
        return relative in self.files or any(path.startswith(f"{relative}/") for path in self.files)

    def read_bytes(self, relative: str) -> bytes:
        return self.files[relative]

    def read_text(self, relative: str) -> str:
        return self.files[relative].decode()

    def write_bytes(self, relative: str, data: bytes) -> None:
        self.files[relative] = data
        self.writes.append(relative)

    def write_text(self, relative: str, text: str) -> None:
        self.write_bytes(relative, text.encode())

    def append_text(self, relative: str, text: str) -> None:
        self.files[relative] = self.files.get(relative, b"") + text.encode()

    def list_dir(self, relative: str) -> list[str]:
        prefix = f"{relative}/"
        return sorted({path.removeprefix(prefix).split("/", 1)[0] for path in self.files if path.startswith(prefix)})

    def text_of(self, relative: str) -> str:
        return self.read_text(relative) if self.exists(relative) else ""


class ManualClock(FakeClock):
    """A clock whose local time is set by the test, and moved by `AdvancingSleeper`."""

    def __init__(self, current: datetime = EVENING) -> None:
        super().__init__()
        self.current = current

    def now(self) -> datetime:
        return self.current


class AdvancingSleeper(FakeSleeper):
    """A sleeper that moves both the monotonic reading and the local time of a `ManualClock`."""

    def __init__(self, clock: ManualClock) -> None:
        super().__init__(clock)
        self._manual = clock

    def sleep(self, seconds: float) -> None:
        super().sleep(seconds)
        self._manual.current += timedelta(seconds=seconds)


class ScriptedPostTransport:
    """Answers each POST from `respond`, which sees the body; a `TransportFailure` answer is raised.

    `after_post` runs inside the call, so a test can move the clock while the request is in flight.
    """

    def __init__(
        self,
        respond: Callable[[bytes], HttpReply | TransportFailure],
        after_post: Callable[[bytes], None] | None = None,
    ) -> None:
        self._respond = respond
        self._after_post = after_post
        self.posts: list[bytes] = []
        self.urls: list[str] = []
        self.timeouts: list[float] = []

    def post(self, url: str, body: bytes, timeout_seconds: float) -> HttpReply:
        self.posts.append(body)
        self.urls.append(url)
        self.timeouts.append(timeout_seconds)
        answer = self._respond(body)
        if self._after_post is not None:
            self._after_post(body)
        if isinstance(answer, TransportFailure):
            raise answer
        return answer

    def get(self, url: str, timeout_seconds: float, headers: Mapping[str, str] | None = None) -> HttpReply:
        raise AssertionError("no GET expected")


def queued_replies(replies: Sequence[HttpReply | TransportFailure]) -> Callable[[bytes], HttpReply | TransportFailure]:
    """A responder that gives the replies in order and fails the test when they run out."""
    remaining = list(replies)

    def respond(_body: bytes) -> HttpReply | TransportFailure:
        if not remaining:
            raise AssertionError("more requests than scripted replies")
        return remaining.pop(0)

    return respond


class FakeModelServer:
    """Implements `ModelServer`; every start and stop is appended to the shared `events`."""

    def __init__(
        self,
        model_id: str,
        flavour: Flavour,
        events: list[str],
        *,
        loaded_context_tokens: int | None = DEFAULT_CONTEXT_TOKENS,
        server_version: str | None = None,
        model_digest: str | None = None,
        shutdown: ShutdownReport = CLEAN_SHUTDOWN,
        cache_hits: Sequence[int | None] | None = None,
        start_failure: Exception | None = None,
    ) -> None:
        self.model_id = model_id
        self.endpoint_url = f"http://server.invalid/{model_id}/v1/systemone"
        self.started_with: list[tuple[str, str | None]] = []
        self.stops = 0
        self._events = events
        self._card = ServerCard(
            server_version=server_version or pinned_version(flavour),
            model_digest=model_digest or digest_of(model_id),
            environment={"FAKE": "1"},
            documents={},
            loaded_context_tokens=loaded_context_tokens,
        )
        self._shutdown = shutdown
        self._cache_hits = cache_hits
        self._cache_reads = 0
        self._start_failure = start_failure

    def start(self, model: ModelSpec, run_reference: str | None) -> None:
        self._events.append(f"start:{model.model_id}")
        self.started_with.append((model.identifier, run_reference))
        if self._start_failure is not None:
            raise self._start_failure

    def card(self) -> ServerCard:
        return self._card

    def prefix_cache_hits(self) -> int | None:
        if self._cache_hits is None:
            return None
        reading = self._cache_hits[self._cache_reads]
        self._cache_reads += 1
        return reading

    def stop(self) -> ShutdownReport:
        self._events.append(f"stop:{self.model_id}")
        self.stops += 1
        return self._shutdown


class FakeServerFactory:
    """Implements `ServerFactory`: one `FakeModelServer` per model, created on first request unless installed."""

    def __init__(self, events: list[str]) -> None:
        self.events = events
        self.servers: dict[str, FakeModelServer] = {}
        self.requested: list[str] = []

    def install(self, server: FakeModelServer) -> FakeModelServer:
        self.servers[server.model_id] = server
        return server

    def server_for(self, model: ModelSpec) -> FakeModelServer:
        self.requested.append(model.model_id)
        if model.model_id not in self.servers:
            self.servers[model.model_id] = FakeModelServer(model.model_id, model.flavour, self.events)
        return self.servers[model.model_id]


class ScriptedHeavyWork:
    """Implements `HeavyWorkProbe`: the reports in order, the last one repeating."""

    def __init__(self, reports: Sequence[HeavyWorkReport] = (CLEAR_REPORT,)) -> None:
        self._reports = reports
        self.calls = 0

    def check(self) -> HeavyWorkReport:
        report = self._reports[min(self.calls, len(self._reports) - 1)]
        self.calls += 1
        return report


class FakeProvenanceProbe:
    def __init__(self, values: Mapping[str, str] | None = None) -> None:
        self.values = dict(values or {})

    def reading(self) -> ProvenanceReading:
        return ProvenanceReading(dict(self.values))


class FakeHarnessIdentity:
    def __init__(self, values: Mapping[str, str] | None = None) -> None:
        self.values = dict(values or {"harness_commit": "harness-1", "builder_digest": "builders-1"})

    def identity(self) -> Mapping[str, str]:
        return dict(self.values)


class FakeMemoryProbe:
    """Implements `MemoryProbe`: the readings in order, the last one repeating."""

    def __init__(self, readings: Sequence[MemoryReading] = (ROOMY_MEMORY,), total_memory_bytes: int = 0) -> None:
        self._readings = readings
        self._total = total_memory_bytes
        self.calls = 0

    def total_memory_bytes(self) -> int:
        return self._total

    def reading(self) -> MemoryReading:
        reading = self._readings[min(self.calls, len(self._readings) - 1)]
        self.calls += 1
        return reading


class MirroringReader:
    """Implements `ProductReader.decode` by restating the reply's answers; the other reads are never used.

    The flags let a test make the relay refuse a reply or make the answer document differ from the reply.
    """

    def __init__(
        self,
        *,
        accepted: bool = True,
        probabilities: Mapping[str, float] | None = None,
        confidence: float | None = None,
        keeps_answers: bool = True,
    ) -> None:
        self._accepted = accepted
        self._probabilities = probabilities
        self._confidence = confidence
        self._keeps_answers = keeps_answers
        self.decoded: list[DecodeRequest] = []

    def decode(self, requests: Sequence[DecodeRequest]) -> list[DecodeOutcome]:
        self.decoded.extend(requests)
        return [self._outcome(request) for request in requests]

    def _outcome(self, request: DecodeRequest) -> DecodeOutcome:
        document = json_object(json.loads(request.reply), "reply")
        answers: Mapping[str, JsonValue] = json_object(document["answers"], "answers") if self._keeps_answers else {}
        kept = tuple(self._answer(question_id, mapping(answer, question_id)) for question_id, answer in answers.items())
        stage, error = (None, None) if self._accepted else ("relay", "the relay refuses the reply")
        return DecodeOutcome(request.key, self._accepted, stage, error, "served", kept)

    def _answer(self, question_id: str, answer: Mapping[str, object]) -> DecodedAnswer:
        stated = answer.get("confidence")
        declared = self._probabilities or mapping(answer["probabilities"], "probabilities")
        confidence = self._confidence if self._confidence is not None else stated
        return DecodedAnswer(
            question_id,
            str(answer.get(CHOICE, "")),
            float(confidence) if isinstance(confidence, int | float) else None,
            {key: float(value) for key, value in declared.items() if isinstance(value, int | float)},
        )

    def read_screen(self, requests: Sequence[ScreenReadRequest]) -> list[ScreenReading]:
        raise NotImplementedError

    def endpoint_digest(self, address: str, model: str) -> EndpointDigest:
        raise NotImplementedError

    def check_threshold(self, entry_yaml: str, default: float, sources: Sequence[str]) -> ThresholdCheck:
        raise NotImplementedError

    def defaults(self) -> ProductDefaults:
        raise NotImplementedError


class EchoBuilders:
    """Implements `Builders.render_closed` by making the closed set itself the request body of every identifier."""

    def render_closed(self, closed_text: str, identifiers: Sequence[str]) -> Mapping[str, JsonValue]:
        return {"bodies": dict.fromkeys(identifiers, closed_text)}

    def alignment_build(self, project: Path, shallnot: Path, identifiers: Sequence[str]) -> Mapping[str, JsonValue]:
        raise NotImplementedError

    def alignment_defaults(self) -> Mapping[str, JsonValue]:
        raise NotImplementedError

    def compare_export(self, project: Path, shallnot: Path, export_text: str) -> Mapping[str, JsonValue]:
        raise NotImplementedError

    def screen_build(
        self, project: Path, shallnot: Path, identifiers: Sequence[str], mutants: Sequence[Mapping[str, JsonValue]]
    ) -> Mapping[str, JsonValue]:
        raise NotImplementedError

    def screen_defaults(self) -> Mapping[str, JsonValue]:
        raise NotImplementedError


class UnusedAlignmentDriver:
    def first_wave_export(self, export_dir: Path, shallnot_script: Path, report: Path) -> str:
        raise NotImplementedError


class RecordingLock:
    """A context manager that logs its entry and exit among the events of a session."""

    def __init__(self, events: list[str]) -> None:
        self._events = events

    def __enter__(self) -> None:
        self._events.append("lock-enter")

    def __exit__(
        self, kind: type[BaseException] | None, error: BaseException | None, trace: TracebackType | None
    ) -> None:
        self._events.append("lock-exit")


class ScriptedLauncher:
    """Implements `BlockLauncher`: exit codes by model, 0 for a model not named; `after_launch` runs after each."""

    def __init__(
        self,
        events: list[str],
        codes: Mapping[str, int] | None = None,
        after_launch: Callable[[str], None] | None = None,
    ) -> None:
        self._events = events
        self._codes = dict(codes or {})
        self._after_launch = after_launch
        self.launched: list[str] = []

    def launch(self, model_id: str) -> int:
        self.launched.append(model_id)
        self._events.append(f"launch:{model_id}")
        if self._after_launch is not None:
            self._after_launch(model_id)
        return self._codes.get(model_id, 0)


@dataclass
class BlockRig:
    """The doubles behind one `BlockDeps`, kept apart so a test can drive and inspect each of them."""

    store: MemoryRunStore
    clock: ManualClock
    sleeper: AdvancingSleeper
    transport: ScriptedPostTransport
    servers: FakeServerFactory
    heavy: ScriptedHeavyWork
    provenance: FakeProvenanceProbe
    harness: FakeHarnessIdentity
    reader: MirroringReader
    window: RunWindow | None
    events: list[str]

    def run(self, model_id: str = "M3", *, canary_only: bool = False) -> BlockOutcome:
        return ModelBlock(self.deps()).run(model_by_id(model_id), canary_only=canary_only)

    def deps(self) -> BlockDeps:
        alignment: AlignmentDriver = UnusedAlignmentDriver()
        return BlockDeps(
            store=self.store,
            clock=self.clock,
            sleeper=self.sleeper,
            transport=self.transport,
            servers=self.servers,
            tools=BuiltTools(EchoBuilders(), self.reader, alignment),
            heavy=self.heavy,
            provenance=self.provenance,
            harness=self.harness,
            window=self.window,
            canary_closed_text=CANARY_CLOSED_TEXT,
        )


def block_rig(
    *,
    model_id: str = "M3",
    behaviour: ServerBehaviour | None = None,
    window: RunWindow | None = None,
    heavy: ScriptedHeavyWork | None = None,
    reader: MirroringReader | None = None,
    after_post: Callable[[bytes], None] | None = None,
    store: MemoryRunStore | None = None,
    clock: ManualClock | None = None,
) -> BlockRig:
    """A rig whose server answers like a conforming one of the flavour of `model_id`."""
    run_clock = clock or ManualClock()
    events: list[str] = []
    answering = behaviour or ServerBehaviour(model_by_id(model_id).flavour)
    return BlockRig(
        store=store or MemoryRunStore(),
        clock=run_clock,
        sleeper=AdvancingSleeper(run_clock),
        transport=ScriptedPostTransport(answering, after_post),
        servers=FakeServerFactory(events),
        heavy=heavy or ScriptedHeavyWork(),
        provenance=FakeProvenanceProbe(),
        harness=FakeHarnessIdentity(),
        reader=reader or MirroringReader(),
        window=window,
        events=events,
    )


def seeded_rig(
    *,
    model_id: str = "M3",
    pass1: int = 3,
    pass2: int = 0,
    blocked: Collection[str] = (),
    behaviour: ServerBehaviour | None = None,
    window: RunWindow | None = None,
    heavy: ScriptedHeavyWork | None = None,
    reader: MirroringReader | None = None,
    after_post: Callable[[bytes], None] | None = None,
    clock: ManualClock | None = None,
) -> tuple[BlockRig, list[str], list[str]]:
    """A rig over a run directory holding the lock, the schedule and the items of `pass1` requests, of which the
    first `pass2` are sent again in reverse; returns the rig and the two request id lists."""
    rig = block_rig(
        model_id=model_id,
        behaviour=behaviour,
        window=window,
        heavy=heavy,
        reader=reader,
        after_post=after_post,
        clock=clock,
    )
    first = request_ids(pass1)
    second = first[:pass2][::-1]
    seed_run(rig.store, pass1=first, pass2=second, lock=lock_record(blocked=blocked))
    return rig, first, second
