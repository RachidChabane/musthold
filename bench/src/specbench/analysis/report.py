"""Deterministic `REPORT.md` from `results.json`: every table names its sample and is labelled descriptive."""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from typing import Final, cast

from specbench.domain.canonical import JsonValue
from specbench.domain.reporting import DESCRIPTIVE_LABEL, SAMPLE_LINE_PREFIX

Json = Mapping[str, JsonValue]
NOT_AVAILABLE: Final = "n/a"
DECIMALS: Final = 3
SECONDS_PER_MINUTE_INVERSE: Final = 1.0 / 60.0
TEMPLATE: Final = (
    "On {tasks} real tasks from {repositories} repositories ({trunks} trunks, {languages} languages, harness {harness}, "
    "model {model}), changes accepted by all proofs had X = {x} fewer defects detected by the projects' own tests "
    "than changes accepted by tests only, for {minutes} extra minutes and {calls} extra model calls per accepted change; "
    "the gate rejected {rejected} of changes those tests found correct."
)


def _section(value: JsonValue, name: str) -> Json:
    if not isinstance(value, Mapping):
        raise ValueError(f"results.{name} is not an object")
    return cast(Json, value)


def _number(value: JsonValue, scale: float = 1.0) -> str:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return NOT_AVAILABLE
    return f"{value * scale:.{DECIMALS}f}"


def interval(value: JsonValue, scale: float = 1.0) -> str:
    """`estimate [lo, hi]` of an estimate record, or `n/a` when it is undefined."""
    if not isinstance(value, Mapping):
        return NOT_AVAILABLE
    record = cast(Json, value)
    if record.get("estimate") is None:
        return NOT_AVAILABLE
    lo, hi = _number(record.get("lo"), scale), _number(record.get("hi"), scale)
    return f"{_number(record['estimate'], scale)} [{lo}, {hi}]"


def _proportion(value: JsonValue) -> str:
    record = _section(value, "proportion")
    return f"{record['successes']}/{record['n']} [{_number(record['lo'])}, {_number(record['hi'])}]"


def _table(header: Sequence[str], rows: Sequence[Sequence[str]]) -> list[str]:
    lines = ["| " + " | ".join(header) + " |", "|" + "|".join("---" for _ in header) + "|"]
    lines += ["| " + " | ".join(row) + " |" for row in rows]
    return [*lines, ""]


def _sample_line(results: Json, harness: Json) -> str:
    sample = _section(results["sample"], "sample")
    trunks = _section(harness["trunks"], "trunks")
    tasks = cast(Sequence[str], harness["tasks"])
    return (
        f"{SAMPLE_LINE_PREFIX}{len(tasks)} tasks, {sample['repositories']} repositories, {trunks['total']} trunks "
        f"({trunks['reached_s_t']} reached S_T), harness model {harness['model']}; {DESCRIPTIVE_LABEL}."
    )


def _template_sentence(results: Json, name: str, harness: Json) -> str:
    arms = _section(harness["arms"], "arms")
    sample = _section(results["sample"], "sample")
    all_arm = arms.get("ALL")
    if all_arm is None:
        return f"The all-proofs arm was not run on harness {name}."
    arm = _section(all_arm, "ALL")
    cost = harness.get("cost")
    cost_record: Json = _section(cost, "cost") if cost is not None else {}
    return TEMPLATE.format(
        tasks=len(cast(Sequence[str], harness["tasks"])),
        repositories=sample["repositories"],
        trunks=_section(harness["trunks"], "trunks")["total"],
        languages=len(cast(Sequence[str], sample["languages"])),
        harness=name,
        model=harness["model"],
        x=interval(_section(arm["x_loop"], "x_loop")["std"]),
        minutes=interval(cost_record.get("extra_seconds_per_accepted_change"), SECONDS_PER_MINUTE_INVERSE),
        calls=interval(cost_record.get("extra_calls_per_accepted_change")),
        rejected=interval(arm["false_rejection"]),
    )


def _arms_table(harness: Json) -> list[str]:
    rows: list[list[str]] = []
    for arm, raw in _section(harness["arms"], "arms").items():
        record = _section(raw, arm)
        loop = _section(record["x_loop"], "x_loop")
        selection = _section(record["selection"], "selection")
        rows.append(
            [
                arm,
                str(record["accepted"]),
                interval(loop["std"]),
                interval(loop["lenient"]),
                interval(loop["strict"]),
                interval(selection["x_sel"]),
                interval(record["yield"]),
                interval(record["false_rejection"]),
                interval(record["escaped_defects"]),
            ]
        )
    header = [
        "Arm",
        "Accepted",
        "X in the loop",
        "X lenient",
        "X strict",
        "X_sel",
        "Yield",
        "False rejection",
        "Escaped defects",
    ]
    return ["### Arms", "", *_table(header, rows)]


def _ladder_table(harness: Json) -> list[str]:
    arms = _section(harness["arms"], "arms")
    rows = [
        [arm, interval(_section(_section(arms[arm], arm)["x_loop"], "x_loop")["std"])]
        for arm in ("TR", "AL", "DP", "CO")
        if arm in arms
    ]
    placebo = harness.get("placebo")
    if placebo is not None:
        rows.append(["ALL minus PL", interval(_section(placebo, "placebo")["x_all_minus_x_pl"])])
    return ["### Per-module ladder and placebo", "", *_table(["Arm", "X in the loop"], rows)] if rows else []


def _composition_table(harness: Json) -> list[str]:
    composition = _section(harness["composition"], "composition")
    absent = _section(harness["absent_modules"], "absent_modules")
    lines: list[str] = []
    if composition:
        rows = [
            [module, str(_section(r, module)["criteria"]), _proportion(_section(r, module)["mismatch_rate"])]
            for module, r in composition.items()
        ]
        lines += ["### Composition check", "", *_table(["Module", "Criteria", "Mismatches"], rows)]
    if absent:
        rows = [
            [key, str(_section(r, key)["probes"]), str(_section(r, key)["counted_as_passed"])]
            for key, r in absent.items()
        ]
        lines += ["### Absent-module probes", "", *_table(["Arm and module", "Probes", "Counted as passed"], rows)]
    return lines


def _blind_spot_table(harness: Json) -> list[str]:
    blind = _section(harness["blind_spot"], "blind_spot")
    rows = [
        [source, str(_section(r, source)["tasks"]), interval(_section(r, source)["sb_coder"])]
        for source, r in blind.items()
    ]
    return (
        ["### Blind spots on agent-written tests", "", *_table(["Mutant source", "Tasks", "SB_coder"], rows)]
        if rows
        else []
    )


def _judge_table(harness: Json) -> list[str]:
    judge = harness.get("judge_replay")
    if judge is None:
        return []
    record = _section(judge, "judge_replay")
    rows = [
        [
            metric,
            interval(_section(record[metric], metric)["same_model"]),
            interval(_section(record[metric], metric)["replay"]),
            interval(_section(record[metric], metric)["delta_replay_minus_same"]),
        ]
        for metric in ("sensitivity", "specificity", "false_rejection", "x_sel")
    ]
    return [
        f"### Same-model judge replay ({record['snapshots']} snapshots)",
        "",
        *_table(["Metric", "Same model", "Replay", "Delta"], rows),
    ]


def _calibration_table(harness: Json) -> list[str]:
    calibration = _section(harness["calibration"], "calibration")
    rows = [
        [
            source,
            str(_section(r, source)["decisions"]),
            interval(_section(r, source)["ece"]),
            interval(_section(r, source)["brier"]),
            interval(_section(r, source)["coverage_at_5pct_error"]),
        ]
        for source, r in calibration.items()
        if isinstance(r, Mapping) and "ece" in r
    ]
    rows.append(
        ["undetermined rate", str(calibration["decisions"]), interval(calibration["undetermined_rate"]), "", ""]
    )
    return [
        "### System One calibration per confidence source",
        "",
        *_table(["Source", "Decisions", "ECE", "Brier", "Coverage at 5 % error"], rows),
    ]


def _interaction_table(results: Json) -> list[str]:
    interaction = results.get("interaction")
    if interaction is None:
        return []
    record = _section(interaction, "interaction")
    differences = _section(record["x_loop_difference"], "x_loop_difference")
    rows = [[arm, interval(value)] for arm, value in differences.items()]
    return [
        "## Harness by arm interaction",
        "",
        f"{SAMPLE_LINE_PREFIX}the tasks both harnesses ran, {record['of']} minus {record['minus']}; {DESCRIPTIVE_LABEL}.",
        "",
        *_table(["Arm", "Difference in X in the loop"], rows),
    ]


def render_report(results: Json) -> str:
    """The report of a sample: one section per harness, never a figure across harnesses."""
    sample = _section(results["sample"], "sample")
    lines = [
        "# Benchmark sample report",
        "",
        f"{SAMPLE_LINE_PREFIX}{sample['tasks']} tasks from {sample['repositories']} repositories ({sample['trunks']} trunks); "
        f"every figure is {DESCRIPTIVE_LABEL}, with 95 % percentile cluster-bootstrap intervals by repository.",
        "",
    ]
    for name, raw in _section(results["harnesses"], "harnesses").items():
        harness = _section(raw, name)
        lines += [
            f"## Harness {name}",
            "",
            _sample_line(results, harness),
            "",
            _template_sentence(results, name, harness),
            "",
            *_arms_table(harness),
            *_ladder_table(harness),
            *_composition_table(harness),
            *_blind_spot_table(harness),
            *_judge_table(harness),
            *_calibration_table(harness),
        ]
    lines += _interaction_table(results)
    return "\n".join(lines).rstrip("\n") + "\n"
