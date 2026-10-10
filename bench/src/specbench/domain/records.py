"""Per-trunk result records of a run: what the analysis reads, parsed with every field checked.

One record per (task, harness) holds the S_N and S_T truth, every arm's outcome, the
composition rows, the same-model judge replay, the blind-spot cells and the System
One decisions. Verdicts are read from Shallnot reports; nothing here derives one.
"""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from enum import Enum
from typing import Final

from specbench.domain.arms import FROZEN_MODEL, JUDGE_REPLAY_MODEL, Arm, Harness, Module
from specbench.domain.canonical import JsonValue
from specbench.domain.jsonread import (
    RecordError,
    boolean,
    enum_of,
    integer,
    mapping,
    optional_boolean,
    sequence,
    text,
)
from specbench.domain.languages import Language

TRUNK_SCHEMA: Final = "specbench.trunk/1"
PERMILLE: Final = 1000


class Verdict(Enum):
    """Final clean-room verdict of an arm; only PASS is accepted (lenient adds ESCALATE)."""

    PASS = "pass"
    LET_GO = "let-go"
    ESCALATE = "escalate"
    BLOCKED = "blocked"


class Writer(Enum):
    CODER = "coder"
    UPSTREAM = "upstream"


class MutantSource(Enum):
    GENERATOR = "generator"
    OPUS = "opus"
    OPERATOR = "operator"


class ConfidenceSource(Enum):
    ENDPOINT = "endpoint"
    HOST_VOTES = "host-votes"
    UNDETERMINED = "undetermined"


@dataclass(frozen=True)
class Truth:
    """Defect truth of one tree: primary (U or R fails), full (U, H or R fails), and O failures.

    `defective_full_unexposed` is the full truth without the H tests of criteria whose H the merge shaped during
    selection (ADR 0012); None where the trunk was scored without that distinction.
    """

    defective: bool
    defective_full: bool
    o_failed: bool
    defective_full_unexposed: bool | None = None


@dataclass(frozen=True)
class ArmResult:
    arm: Arm
    model: str
    selection_pass: bool | None
    verdict: Verdict
    escalated_during: bool
    final: Truth | None
    model_calls: int
    seconds: int
    local_verdict: Verdict | None = None
    """What the same proofs earn under local trust: a developer-loop counterfactual, never read by acceptance."""

    def accepted(self, lenient: bool = False, strict: bool = False) -> bool:
        """Accepted iff the final verdict is PASS; lenient also takes ESCALATE, strict refuses any ESCALATE during the trunk."""
        if strict and self.escalated_during:
            return False
        return self.verdict is Verdict.PASS or (lenient and self.verdict is Verdict.ESCALATE)


@dataclass(frozen=True)
class CompositionRow:
    criterion: str
    module: Module
    all_verdict: str
    composed_verdict: str

    @property
    def matches(self) -> bool:
        return self.all_verdict == self.composed_verdict


@dataclass(frozen=True)
class AbsentModuleProbe:
    arm: Arm
    module: Module
    counted_as_passed: bool


@dataclass(frozen=True)
class JudgeReplay:
    replay_model: str
    same_pass: bool
    replay_pass: bool


@dataclass(frozen=True)
class BlindSpotCell:
    criterion: str
    writer: Writer
    source: MutantSource
    escaped: int
    evaluable: int


@dataclass(frozen=True)
class SourceDecision:
    source: ConfidenceSource
    confidence_permille: int | None
    truth: bool | None


@dataclass(frozen=True)
class TrunkRecord:
    task_id: str
    repository: str
    language: Language
    harness: Harness
    model: str
    reached_s_t: bool
    s_n: Truth
    s_t: Truth | None
    arms: Mapping[Arm, ArmResult]
    composition: tuple[CompositionRow, ...] = ()
    absent_probes: tuple[AbsentModuleProbe, ...] = ()
    judge_replay: JudgeReplay | None = None
    blind_spot: tuple[BlindSpotCell, ...] = ()
    decisions: tuple[SourceDecision, ...] = ()


def _truth(value: object, where: str) -> Truth:
    body = mapping(value, where)
    return Truth(
        boolean(body.get("defective"), f"{where}.defective"),
        boolean(body.get("defective_full"), f"{where}.defective_full"),
        boolean(body.get("o_failed"), f"{where}.o_failed"),
        optional_boolean(body.get("defective_full_unexposed"), f"{where}.defective_full_unexposed"),
    )


def _arm_result(arm: Arm, value: object, where: str) -> ArmResult:
    body = mapping(value, where)
    final = body.get("final")
    return ArmResult(
        arm=arm,
        model=text(body.get("model"), f"{where}.model"),
        selection_pass=optional_boolean(body.get("selection_pass"), f"{where}.selection_pass"),
        verdict=enum_of(Verdict, body.get("verdict"), f"{where}.verdict"),
        escalated_during=boolean(body.get("escalated_during"), f"{where}.escalated_during"),
        final=None if final is None else _truth(final, f"{where}.final"),
        model_calls=integer(body.get("model_calls"), f"{where}.model_calls"),
        seconds=integer(body.get("seconds"), f"{where}.seconds"),
        local_verdict=None
        if body.get("local_verdict") is None
        else enum_of(Verdict, body.get("local_verdict"), f"{where}.local_verdict"),
    )


def _decision(value: object, where: str) -> SourceDecision:
    body = mapping(value, where)
    source = enum_of(ConfidenceSource, body.get("source"), f"{where}.source")
    permille = body.get("confidence_permille")
    truth = optional_boolean(body.get("truth"), f"{where}.truth")
    if source is ConfidenceSource.UNDETERMINED:
        return SourceDecision(source, None, truth)
    if permille is None:
        raise RecordError(f"{where} states no confidence")
    confidence = integer(permille, f"{where}.confidence_permille")
    if confidence > PERMILLE or truth is None:
        raise RecordError(f"{where} has no truth or a confidence above {PERMILLE}")
    return SourceDecision(source, confidence, truth)


def _cell(row: Mapping[str, object]) -> BlindSpotCell:
    escaped = integer(row.get("escaped"), "blind_spot.escaped")
    evaluable = integer(row.get("evaluable"), "blind_spot.evaluable")
    if escaped > evaluable:
        raise RecordError("blind_spot.escaped exceeds blind_spot.evaluable")
    return BlindSpotCell(
        text(row.get("criterion"), "blind_spot.criterion"),
        enum_of(Writer, row.get("writer"), "blind_spot.writer"),
        enum_of(MutantSource, row.get("source"), "blind_spot.source"),
        escaped,
        evaluable,
    )


def trunk_from_record(record: JsonValue) -> TrunkRecord:
    body = mapping(record, "trunk")
    if body.get("schema") != TRUNK_SCHEMA:
        raise RecordError(f"schema must be {TRUNK_SCHEMA}")
    harness = enum_of(Harness, body.get("harness"), "harness")
    model = text(body.get("model"), "model")
    if model != FROZEN_MODEL[harness]:
        raise RecordError(f"{harness.value} runs {FROZEN_MODEL[harness]}, not {model}")
    arms: dict[Arm, ArmResult] = {}
    for name, raw in mapping(body.get("arms"), "arms").items():
        arm = enum_of(Arm, name, "arms key")
        result = _arm_result(arm, raw, f"arms.{name}")
        if result.model != model:
            raise RecordError(f"arm {name} ran {result.model}, not the harness model {model}")
        arms[arm] = result
    reached = boolean(body.get("reached_s_t"), "reached_s_t")
    s_t = body.get("s_t")
    if reached != (s_t is not None):
        raise RecordError("s_t must be present exactly when the trunk reached S_T")
    judge = body.get("judge_replay")
    judge_body = None if judge is None else mapping(judge, "judge_replay")
    if judge_body is not None and judge_body.get("replay_model") != JUDGE_REPLAY_MODEL[harness]:
        raise RecordError(f"{harness.value} replays its judged requests through {JUDGE_REPLAY_MODEL[harness]}")
    return TrunkRecord(
        task_id=text(body.get("task_id"), "task_id"),
        repository=text(body.get("repository"), "repository"),
        language=enum_of(Language, body.get("language"), "language"),
        harness=harness,
        model=model,
        reached_s_t=reached,
        s_n=_truth(body.get("s_n"), "s_n"),
        s_t=None if s_t is None else _truth(s_t, "s_t"),
        arms=arms,
        composition=tuple(
            CompositionRow(
                text(row.get("criterion"), "composition.criterion"),
                enum_of(Module, row.get("module"), "composition.module"),
                text(row.get("all_verdict"), "composition.all_verdict"),
                text(row.get("composed_verdict"), "composition.composed_verdict"),
            )
            for row in (mapping(r, "composition row") for r in sequence(body.get("composition", []), "composition"))
        ),
        absent_probes=tuple(
            AbsentModuleProbe(
                enum_of(Arm, row.get("arm"), "probe.arm"),
                enum_of(Module, row.get("module"), "probe.module"),
                boolean(row.get("counted_as_passed"), "probe.counted_as_passed"),
            )
            for row in (mapping(r, "probe row") for r in sequence(body.get("absent_probes", []), "absent_probes"))
        ),
        judge_replay=None
        if judge_body is None
        else JudgeReplay(
            text(judge_body.get("replay_model"), "judge_replay.replay_model"),
            boolean(judge_body.get("same_pass"), "judge_replay.same_pass"),
            boolean(judge_body.get("replay_pass"), "judge_replay.replay_pass"),
        ),
        blind_spot=tuple(
            _cell(row)
            for row in (mapping(r, "blind_spot row") for r in sequence(body.get("blind_spot", []), "blind_spot"))
        ),
        decisions=tuple(
            _decision(d, f"decisions[{i}]") for i, d in enumerate(sequence(body.get("decisions", []), "decisions"))
        ),
    )
