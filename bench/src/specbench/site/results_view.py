"""A typed reading of `results.json`: every figure keeps the path it was read from, so the page can cite it."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from enum import Enum
from typing import Final, cast

from specbench.domain.arms import ACCEPTANCE_ARMS, Arm, Harness, Module
from specbench.domain.jsonread import RecordError, enum_of, integer, mapping, sequence, text
from specbench.site.figures import Comparison, Estimate, Figure, FigureKind, Sample, Unit

POINTER_SEPARATOR: Final = "/"
RESULTS_SCHEMA: Final = "specbench.results/1"
REFERENCE_ARM: Final = Arm.T
UNIT_TRUNKS: Final = Unit("trunk at S_T", "trunks at S_T")
UNIT_ACCEPTED: Final = Unit("accepted change", "accepted changes")
UNIT_CORRECT: Final = Unit("correct change at S_T", "correct changes at S_T")
UNIT_CRITERIA: Final = Unit("criterion", "criteria")


def pointer(*segments: str) -> str:
    """The path of a value in the results, as RFC 6901 escapes it, without the leading slash."""
    return POINTER_SEPARATOR.join(s.replace("~", "~0").replace("/", "~1") for s in segments)


def resolve(document: object, path: str) -> object:
    """The value at `path` in a decoded results document; a missing segment raises `KeyError`."""
    value = document
    for raw in path.split(POINTER_SEPARATOR):
        key = raw.replace("~1", "/").replace("~0", "~")
        if not isinstance(value, Mapping):
            raise KeyError(path)
        value = cast(Mapping[str, object], value)[key]
    return value


class ArmMetric(Enum):
    """The figures of an arm the site shows, each with its field, its unit and the denominator it rests on."""

    RESIDUAL_DEFECTS = "defect_rate_accepted"
    REDUCTION = "x_loop"
    FALSE_REJECTION = "false_rejection"
    YIELD = "yield"
    ESCAPED = "escaped_defects"
    CALLS = "calls_per_trunk"
    SECONDS = "seconds_per_trunk"
    OUT_OF_SPEC = "o_defect_rate_accepted"


METRIC_KIND: Final = {
    ArmMetric.RESIDUAL_DEFECTS: FigureKind.RATE,
    ArmMetric.REDUCTION: FigureKind.RATE,
    ArmMetric.FALSE_REJECTION: FigureKind.RATE,
    ArmMetric.YIELD: FigureKind.RATE,
    ArmMetric.ESCAPED: FigureKind.RATE,
    ArmMetric.CALLS: FigureKind.CALLS,
    ArmMetric.SECONDS: FigureKind.MINUTES,
    ArmMetric.OUT_OF_SPEC: FigureKind.RATE,
}
STANDARD_MODE: Final = "std"


@dataclass(frozen=True)
class ArmView:
    arm: Arm
    trunks: int
    accepted: int
    clean_trunks: int
    figures: Mapping[ArmMetric, Figure]

    def figure(self, metric: ArmMetric) -> Figure:
        return self.figures[metric]


@dataclass(frozen=True)
class Proportion:
    """A Wilson proportion of `results.json`: successes out of n, with its interval."""

    pointer: str
    successes: int
    n: int
    lo: float
    hi: float

    def as_figure(self, unit: Unit, repositories: int) -> Figure:
        value = self.successes / self.n if self.n else None
        return Figure(
            self.pointer, FigureKind.RATE, Estimate(value, self.lo, self.hi), Sample(self.n, unit, repositories)
        )


@dataclass(frozen=True)
class CompositionView:
    module: Module
    criteria: int
    mismatches: Proportion


@dataclass(frozen=True)
class AbsentProbeView:
    pointer: str
    arm: Arm
    module: Module
    probes: int
    counted_as_passed: int


@dataclass(frozen=True)
class HarnessView:
    harness: Harness
    model: str
    tasks: tuple[str, ...]
    repositories: int
    trunks_total: int
    reached_s_t: int
    unreached: int
    defective_at_s_t: Proportion
    arms: tuple[ArmView, ...]
    placebo: Figure | None
    extra_calls: Figure | None
    extra_seconds: Figure | None
    composition: tuple[CompositionView, ...]
    absent_probes: tuple[AbsentProbeView, ...]

    def arm(self, arm: Arm) -> ArmView | None:
        return next((view for view in self.arms if view.arm is arm), None)


@dataclass(frozen=True)
class Method:
    interval: str
    level: float
    resamples: int
    seed: int


@dataclass(frozen=True)
class SampleView:
    tasks: int
    repositories: int
    trunks: int
    languages: tuple[str, ...]


@dataclass(frozen=True)
class ResultsView:
    label: str
    method: Method
    sample: SampleView
    harnesses: tuple[HarnessView, ...]


def _estimate(value: object, where: str) -> Estimate:
    return Estimate.from_record(mapping(value, where))


def _proportion(value: object, path: str) -> Proportion:
    body = mapping(value, path)
    lo, hi = body.get("lo"), body.get("hi")
    if not isinstance(lo, int | float) or not isinstance(hi, int | float):
        raise RecordError(f"{path} has no interval")
    return Proportion(path, integer(body.get("successes"), path), integer(body.get("n"), path), float(lo), float(hi))


def _arm_view(harness: str, arm: Arm, body: Mapping[str, object], repositories: int) -> ArmView:
    base = ("harnesses", harness, "arms", arm.value)
    trunks = integer(body.get("trunks"), "trunks")
    accepted = integer(body.get("accepted"), "accepted")
    clean = integer(body.get("clean_trunks"), "clean_trunks")
    denominators = {
        ArmMetric.RESIDUAL_DEFECTS: Sample(accepted, UNIT_ACCEPTED, repositories),
        ArmMetric.REDUCTION: Sample(trunks, UNIT_TRUNKS, repositories),
        ArmMetric.FALSE_REJECTION: Sample(clean, UNIT_CORRECT, repositories),
        ArmMetric.YIELD: Sample(trunks, UNIT_TRUNKS, repositories),
        ArmMetric.ESCAPED: Sample(trunks, UNIT_TRUNKS, repositories),
        ArmMetric.CALLS: Sample(trunks, UNIT_TRUNKS, repositories),
        ArmMetric.SECONDS: Sample(trunks, UNIT_TRUNKS, repositories),
        ArmMetric.OUT_OF_SPEC: Sample(accepted, UNIT_ACCEPTED, repositories),
    }
    figures: dict[ArmMetric, Figure] = {}
    for metric in ArmMetric:
        if metric is ArmMetric.REDUCTION:
            path = pointer(*base, metric.value, STANDARD_MODE)
            raw = mapping(body.get(metric.value), path).get(STANDARD_MODE)
            figures[metric] = Figure(
                path,
                METRIC_KIND[metric],
                _estimate(raw, path),
                denominators[metric],
                comparison=Comparison.REDUCTION,
                reference=arm is REFERENCE_ARM,
                undefined_reason="the tests-only arm accepted no defective change, so the ratio is undefined",
            )
            continue
        path = pointer(*base, metric.value)
        figures[metric] = Figure(
            path, METRIC_KIND[metric], _estimate(body.get(metric.value), path), denominators[metric]
        )
    return ArmView(arm, trunks, accepted, clean, figures)


def _cost_figure(
    harness: str, cost: object, field: str, kind: FigureKind, trunks: int, repositories: int
) -> Figure | None:
    if cost is None:
        return None
    path = pointer("harnesses", harness, "cost", field)
    return Figure(
        path,
        kind,
        _estimate(mapping(cost, path).get(field), path),
        Sample(trunks, UNIT_TRUNKS, repositories),
        comparison=Comparison.DIFFERENCE,
    )


def _harness_view(name: str, body: Mapping[str, object]) -> HarnessView:
    harness = enum_of(Harness, name, "harness")
    repositories = integer(body.get("repositories"), f"{name}.repositories")
    arms_body = mapping(body.get("arms"), f"{name}.arms")
    arms = tuple(
        _arm_view(name, arm, mapping(arms_body[arm.value], arm.value), repositories)
        for arm in ACCEPTANCE_ARMS
        if arm.value in arms_body
    )
    trunks = mapping(body.get("trunks"), f"{name}.trunks")
    placebo = body.get("placebo")
    all_arm = next((view for view in arms if view.arm is Arm.ALL), None)
    trunks_of_all = all_arm.trunks if all_arm is not None else 0
    cost = body.get("cost")
    composition = mapping(body.get("composition"), f"{name}.composition")
    absent = mapping(body.get("absent_modules"), f"{name}.absent_modules")
    return HarnessView(
        harness=harness,
        model=text(body.get("model"), f"{name}.model"),
        tasks=tuple(text(t, "task") for t in sequence(body.get("tasks"), f"{name}.tasks")),
        repositories=repositories,
        trunks_total=integer(trunks.get("total"), "trunks.total"),
        reached_s_t=integer(trunks.get("reached_s_t"), "trunks.reached_s_t"),
        unreached=integer(trunks.get("unreached"), "trunks.unreached"),
        defective_at_s_t=_proportion(
            trunks.get("s_t_defective"), pointer("harnesses", name, "trunks", "s_t_defective")
        ),
        arms=arms,
        placebo=None
        if placebo is None
        else Figure(
            pointer("harnesses", name, "placebo", "x_all_minus_x_pl"),
            FigureKind.RATE,
            _estimate(mapping(placebo, "placebo").get("x_all_minus_x_pl"), "placebo"),
            Sample(trunks_of_all, UNIT_TRUNKS, repositories),
            comparison=Comparison.DIFFERENCE,
        ),
        extra_calls=_cost_figure(
            name, cost, "extra_calls_per_accepted_change", FigureKind.CALLS, trunks_of_all, repositories
        ),
        extra_seconds=_cost_figure(
            name, cost, "extra_seconds_per_accepted_change", FigureKind.MINUTES, trunks_of_all, repositories
        ),
        composition=tuple(
            CompositionView(
                enum_of(Module, module, "composition module"),
                integer(mapping(raw, module).get("criteria"), "criteria"),
                _proportion(
                    mapping(raw, module).get("mismatch_rate"),
                    pointer("harnesses", name, "composition", module, "mismatch_rate"),
                ),
            )
            for module, raw in sorted(composition.items())
        ),
        absent_probes=tuple(_absent(name, key, mapping(raw, key)) for key, raw in sorted(absent.items())),
    )


def _absent(harness: str, key: str, body: Mapping[str, object]) -> AbsentProbeView:
    arm_name, _, module = key.partition("|")
    return AbsentProbeView(
        pointer("harnesses", harness, "absent_modules", key),
        enum_of(Arm, arm_name, "absent probe arm"),
        enum_of(Module, module, "absent probe module"),
        integer(body.get("probes"), "probes"),
        integer(body.get("counted_as_passed"), "counted_as_passed"),
    )


def results_view(document: object) -> ResultsView:
    """The site's reading of a results document; a document of another schema is refused."""
    body = mapping(document, "results")
    if body.get("schema") != RESULTS_SCHEMA:
        raise RecordError(f"results schema must be {RESULTS_SCHEMA}")
    method = mapping(body.get("method"), "method")
    level = method.get("level")
    if isinstance(level, bool) or not isinstance(level, int | float):
        raise RecordError("method.level must be a number")
    sample = mapping(body.get("sample"), "sample")
    harnesses = mapping(body.get("harnesses"), "harnesses")
    languages: Sequence[object] = sequence(sample.get("languages"), "sample.languages")
    return ResultsView(
        label=text(body.get("label"), "label"),
        method=Method(
            text(method.get("interval"), "method.interval"),
            float(level),
            integer(method.get("resamples"), "method.resamples"),
            integer(method.get("seed"), "method.seed"),
        ),
        sample=SampleView(
            integer(sample.get("tasks"), "sample.tasks"),
            integer(sample.get("repositories"), "sample.repositories"),
            integer(sample.get("trunks"), "sample.trunks"),
            tuple(text(language, "language") for language in languages),
        ),
        harnesses=tuple(_harness_view(name, mapping(harnesses[name], name)) for name in sorted(harnesses)),
    )
