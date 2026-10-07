"""A small run directory and the local doubles a `ModelBlock` needs to run against the stub servers.

The real adapters (`UrllibTransport`, `OllamaServer`, `KevServer`, `FileRunStore`) are used as they are; only the
Go-backed ports (`Builders`, `ProductReader`), the machine probes and the clock are replaced here.
"""

from __future__ import annotations

import json
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Final

from calibration.adapters.file_run_store import FileRunStore
from calibration.application.build_ports import BuiltTools
from calibration.application.layout import item_path
from calibration.application.ports import HeavyWorkReport, ProvenanceReading
from calibration.domain.canonical import JsonValue, document_json
from calibration.domain.constants import ENCODING, KEV_COMMIT, MODELS_LOCK_FILE, OLLAMA_VERSION, SCHEDULE_FILE
from calibration.domain.items import Item, KindFamily
from calibration.domain.jsonread import json_object, mapping, sequence, text
from calibration.domain.models import identifiers
from calibration.domain.ports import (
    DecodedAnswer,
    DecodeOutcome,
    DecodeRequest,
    EndpointDigest,
    HttpReply,
    HttpTransport,
    ProductDefaults,
    ScreenReading,
    ScreenReadRequest,
    ThresholdCheck,
)
from calibration.domain.schedule import Schedule, alignment_request_id, is_alignment_request, mutant_request_id
from fixtures.wire_data import RECORDED_KEV_RUN
from repository_paths import REPOSITORY_ROOT
from support.stub_ollama import DEFAULT_DIGEST

CANARY_PROMPT: Final = REPOSITORY_ROOT / "experiments" / "calibration" / "prompts" / "canary.json"
KEV_DIGEST: Final = RECORDED_KEV_RUN.split("@", 1)[1]
ALIGNMENT_NODEID: Final = "tests/test_a.py::test_{}"
ALIGNMENT_COUNT: Final = 3
MUTANT_COUNT: Final = 4
CHOICE_OPTIONS: Final = ("blue", "green", "grey")
SCORE_LEVELS: Final = ("none", "low", "high")
OTHER_DIGEST: Final = "ab" * 32
TOTAL_MEMORY_BYTES: Final = 24 * 1024**3
FOOTPRINT_BYTES: Final = 2 * 1024**3
WINDOW_TEXT: Final = "21:00-07:30"
INSIDE_WINDOW: Final = datetime(2026, 10, 7, 21, 0, 0)
OUTSIDE_WINDOW: Final = datetime(2026, 10, 8, 8, 0, 0)
CLOSED_DIGEST: Final = "c" * 64
TEMPLATE_DIGEST: Final = "d" * 64


def canary_closed_text() -> str:
    return CANARY_PROMPT.read_text(encoding=ENCODING)


def closed_set(state: str) -> dict[str, JsonValue]:
    """A closed set with one choice and one score question and the given state."""
    return {
        "state": state,
        "questions": [
            {
                "id": "color",
                "type": "choice",
                "instructions": "Which color does the text say the sky is?",
                "options": [{"name": name, "description": name} for name in CHOICE_OPTIONS],
            },
            {
                "id": "level",
                "type": "score",
                "instructions": "How strongly does the text say it?",
                "options": [{"name": name, "description": name} for name in SCORE_LEVELS],
            },
        ],
    }


def render_body(closed_json: str, identifier: str) -> str:
    """`SystemOneRequest(model)` of a closed set: the request body the servers answer."""
    closed = json_object(json.loads(closed_json), "closed")
    questions: dict[str, JsonValue] = {}
    for raw in sequence(closed.get("questions"), "questions"):
        question = mapping(raw, "question")
        options = [mapping(option, "option") for option in sequence(question.get("options"), "options")]
        criteria: JsonValue
        if question.get("type") == "choice":
            criteria = {text(o.get("name"), "name"): text(o.get("description"), "description") for o in options}
        else:
            criteria = [text(o.get("description"), "description") for o in options]
        questions[text(question.get("id"), "id")] = {
            "type": text(question.get("type"), "type"),
            "instructions": text(question.get("instructions"), "instructions"),
            "criteria": criteria,
        }
    body: dict[str, JsonValue] = {
        "model": identifier,
        "state": text(closed.get("state"), "state"),
        "questions": questions,
    }
    return json.dumps(body, separators=(",", ":"))


class RenderingBuilders:
    """The part of the Go builders a block uses: `render_closed`."""

    def alignment_build(self, project: Path, shallnot: Path, identifiers: Sequence[str]) -> Mapping[str, JsonValue]:
        raise NotImplementedError

    def alignment_defaults(self) -> Mapping[str, JsonValue]:
        raise NotImplementedError

    def render_closed(self, closed_text: str, identifiers: Sequence[str]) -> Mapping[str, JsonValue]:
        return {"bodies": {identifier: render_body(closed_text, identifier) for identifier in identifiers}}

    def compare_export(self, project: Path, shallnot: Path, export_text: str) -> Mapping[str, JsonValue]:
        raise NotImplementedError

    def screen_build(
        self, project: Path, shallnot: Path, identifiers: Sequence[str], mutants: Sequence[Mapping[str, JsonValue]]
    ) -> Mapping[str, JsonValue]:
        raise NotImplementedError

    def screen_defaults(self) -> Mapping[str, JsonValue]:
        raise NotImplementedError


class DecodingReader:
    """The product's reading of a reply for a choice answer: the choice, its confidence and its probabilities, kept."""

    def decode(self, requests: Sequence[DecodeRequest]) -> list[DecodeOutcome]:
        return [self._decode_one(request) for request in requests]

    @staticmethod
    def _decode_one(request: DecodeRequest) -> DecodeOutcome:
        try:
            reply = json_object(json.loads(request.reply), "reply")
            answers = mapping(reply.get("answers"), "answers")
            decoded = tuple(
                DecodedAnswer(
                    question_id,
                    str(json_object(raw, question_id).get("choice", "")),
                    _confidence(raw),
                    {key: float(value) for key, value in _probabilities(raw).items()},
                )
                for question_id, raw in answers.items()
            )
        except (ValueError, TypeError) as error:
            return DecodeOutcome(request.key, False, "relay", str(error), None, ())
        return DecodeOutcome(request.key, True, None, None, str(reply.get("model")), decoded, request.reply)

    def read_screen(self, requests: Sequence[ScreenReadRequest]) -> list[ScreenReading]:
        raise NotImplementedError

    def endpoint_digest(self, address: str, model: str) -> EndpointDigest:
        raise NotImplementedError

    def check_threshold(self, entry_yaml: str, default: float, sources: Sequence[str]) -> ThresholdCheck:
        raise NotImplementedError

    def defaults(self) -> ProductDefaults:
        raise NotImplementedError


def _confidence(raw: object) -> float | None:
    value = mapping(raw, "answer").get("confidence")
    return float(value) if isinstance(value, int | float) and not isinstance(value, bool) else None


def _probabilities(raw: object) -> Mapping[str, float]:
    declared = mapping(mapping(raw, "answer").get("probabilities"), "probabilities")
    return {key: float(value) for key, value in declared.items() if isinstance(value, int | float)}


class UnusedAlignmentDriver:
    def first_wave_export(self, export_dir: Path, shallnot_script: Path, report: Path) -> str:
        raise NotImplementedError


def local_tools() -> BuiltTools:
    return BuiltTools(RenderingBuilders(), DecodingReader(), UnusedAlignmentDriver())


class ClearHeavyProbe:
    def __init__(self) -> None:
        self.checks = 0

    def check(self) -> HeavyWorkReport:
        self.checks += 1
        return HeavyWorkReport((), (), ())


class FixedProvenance:
    def reading(self) -> ProvenanceReading:
        return ProvenanceReading({"machine": "test"})


class FixedHarness:
    def identity(self) -> Mapping[str, str]:
        return {"harness_commit": "0" * 40, "builder_digest": "1" * 64}


class SettableClock:
    """Local time that a test moves; `monotonic` is a counter that grows by a millisecond at each reading."""

    def __init__(self, moment: datetime = INSIDE_WINDOW) -> None:
        self.moment = moment
        self._ticks = 0

    def now(self) -> datetime:
        return self.moment

    def monotonic(self) -> float:
        self._ticks += 1
        return self._ticks / 1000.0


class RecordingSleeper:
    """Sleeps for no time; the durations asked for are kept."""

    def __init__(self) -> None:
        self.sleeps: list[float] = []

    def sleep(self, seconds: float) -> None:
        self.sleeps.append(seconds)


class ObservedTransport:
    """The real transport, counted: `after_posts` posts, a hook runs; a timeout can be forced shorter."""

    def __init__(
        self,
        inner: HttpTransport,
        *,
        timeout_seconds: float | None = None,
        after_posts: int | None = None,
        clock_to_close: SettableClock | None = None,
    ) -> None:
        self._inner = inner
        self._timeout = timeout_seconds
        self._after = after_posts
        self._clock = clock_to_close
        self.posts: list[bytes] = []

    def post(self, url: str, body: bytes, timeout_seconds: float) -> HttpReply:
        self.posts.append(body)
        reply = self._inner.post(url, body, self._timeout or timeout_seconds)
        if self._after is not None and self._clock is not None and len(self.posts) >= self._after:
            self._clock.moment = OUTSIDE_WINDOW
        return reply

    def get(self, url: str, timeout_seconds: float, headers: Mapping[str, str] | None = None) -> HttpReply:
        return self._inner.get(url, timeout_seconds, headers)


@dataclass(frozen=True)
class WorldItem:
    request_id: str
    state: str


@dataclass(frozen=True)
class RunWorld:
    """A run directory with a lock that pins every model, the items of both families and the schedule."""

    store: FileRunStore
    items: tuple[WorldItem, ...]
    schedule: Schedule


def _lock_record() -> dict[str, JsonValue]:
    def entry(digest: str, reference: str | None) -> dict[str, JsonValue]:
        return {
            "status": "pinned",
            "block_reason": None,
            "footprint_bytes": FOOTPRINT_BYTES,
            "model_digest": digest,
            "run_reference": reference,
        }

    return {
        "total_memory_bytes": TOTAL_MEMORY_BYTES,
        "ollama": {
            "version": OLLAMA_VERSION,
            "models": {
                "M1": entry(OTHER_DIGEST, None),
                "M2": entry(OTHER_DIGEST, None),
                "M3": entry(DEFAULT_DIGEST, None),
            },
        },
        "kev": {
            "commit": KEV_COMMIT,
            "models": {
                "M4": entry(KEV_DIGEST, RECORDED_KEV_RUN),
                "M5": entry(OTHER_DIGEST, RECORDED_KEV_RUN),
                "M6": entry(OTHER_DIGEST, RECORDED_KEV_RUN),
            },
        },
    }


def _item(request_id: str, family: KindFamily, state: str) -> Item:
    closed = closed_set(state)
    closed_text = json.dumps(closed)
    bodies = {identifier: render_body(closed_text, identifier) for identifier in identifiers()}
    unit = f"unit of {request_id}"
    if family is KindFamily.ALIGNMENT:
        return Item(
            request_id, family, unit, closed, CLOSED_DIGEST, TEMPLATE_DIGEST, bodies,
            nodeid=request_id, label="L", declared="D", token_to_ref={"T1": "ref"},
        )  # fmt: skip
    return Item(
        request_id, family, unit, closed, CLOSED_DIGEST, TEMPLATE_DIGEST, bodies,
        mutant_id=request_id, question_to_ref={"q1": "ref"}, witness_status=None,
    )  # fmt: skip


def build_run_world(root: Path, *, long_state_for: str | None = None, long_state_chars: int = 0) -> RunWorld:
    """Writes the lock, the items and the schedule; one item may get a state of `long_state_chars` characters."""
    store = FileRunStore(root)
    request_ids = [alignment_request_id(ALIGNMENT_NODEID.format(n)) for n in range(ALIGNMENT_COUNT)]
    request_ids += [mutant_request_id(f"{n:016x}") for n in range(MUTANT_COUNT)]
    items: list[WorldItem] = []
    for request_id in request_ids:
        state = "x" * long_state_chars if request_id == long_state_for else f"the sky of {request_id} is green"
        family = KindFamily.ALIGNMENT if is_alignment_request(request_id) else KindFamily.SCREEN
        store.write_text(item_path(request_id), document_json(_item(request_id, family, state).to_record()))
        items.append(WorldItem(request_id, state))
    schedule = Schedule.of(request_ids)
    store.write_text(SCHEDULE_FILE, document_json({"pass1": list(schedule.pass1), "pass2": list(schedule.pass2)}))
    store.write_text(MODELS_LOCK_FILE, document_json(_lock_record()))
    return RunWorld(store, tuple(items), schedule)
