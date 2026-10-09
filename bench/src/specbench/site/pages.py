"""Every page of the site, as a pure function of the inputs: overview, cases, arms, modules, methodology, limits, reproduce."""

from __future__ import annotations

from collections.abc import Sequence
from dataclasses import dataclass
from typing import Final

from specbench.domain.arms import ACCEPTANCE_ARMS, Arm, host_for
from specbench.domain.cases import MutantOutcome
from specbench.domain.deviations import DeviationEntry
from specbench.domain.records import TrunkRecord
from specbench.domain.scoring import FAILURE_QUORUM, SCORING_RUNS
from specbench.site import copy
from specbench.site.charts import ChartRow
from specbench.site.components import (
    chart_figure,
    count,
    coverage_note,
    figure,
    percent_of,
    verdict,
)
from specbench.site.gate_report import GateReading, read_gate_report
from specbench.site.highlight import diff_stats, render_diff, unified_diff
from specbench.site.html import Content, Markup, classed, element, escape, join, link, table
from specbench.site.model import SYNTHETIC_BADGE, SYNTHETIC_NOTICE, CaseMaterial, SiteInputs
from specbench.site.results_view import (
    UNIT_CRITERIA,
    UNIT_TRUNKS,
    ArmMetric,
    HarnessView,
    ResultsView,
    pointer,
)
from specbench.site.template_files import PageTemplate, fill

CASES_DIRECTORY: Final = "cases"
INDEX: Final = "index.html"
DIGEST_PREFIX_LENGTH: Final = 12
MODULE_ARMS: Final = (Arm.TR, Arm.AL, Arm.DP, Arm.CO, Arm.ALL)
OVERVIEW_METRICS: Final = (ArmMetric.RESIDUAL_DEFECTS, ArmMetric.REDUCTION, ArmMetric.FALSE_REJECTION)
ARM_TABLE_METRICS: Final = (
    ArmMetric.RESIDUAL_DEFECTS,
    ArmMetric.REDUCTION,
    ArmMetric.FALSE_REJECTION,
    ArmMetric.CALLS,
    ArmMetric.SECONDS,
)
SECONDARY_METRICS: Final = (ArmMetric.YIELD, ArmMetric.ESCAPED, ArmMetric.OUT_OF_SPEC)
NOT_RECORDED: Final = "Not recorded in the archive."
UNGATED_ARMS: Final = frozenset({Arm.N, Arm.T})
NO_GATE: Final = "no gate"


@dataclass(frozen=True)
class NavItem:
    path: str
    label: str


NAV: Final = (
    NavItem(INDEX, "Overview"),
    NavItem(f"{CASES_DIRECTORY}/{INDEX}", "Cases"),
    NavItem("arms.html", "Arms"),
    NavItem("modules.html", "Modules"),
    NavItem("methodology.html", "Methodology"),
    NavItem("limits.html", "Limits"),
    NavItem("reproduce.html", "Reproduce"),
)


@dataclass(frozen=True)
class Page:
    path: str
    title: str
    description: str
    body: Markup

    @property
    def root(self) -> str:
        return "../" * self.path.count("/")


def case_path(case_id: str) -> str:
    return f"{CASES_DIRECTORY}/{case_id}.html"


def _interval_header(results: ResultsView) -> Markup:
    level = count("method/level", results.method.level, percent_of(results.method.level))
    return join(["estimate [", level, " interval], then the sample it rests on"])


def _harness_heading(harness: HarnessView, level: str = "h2") -> Markup:
    name = copy.HARNESS_NAMES[harness.harness]
    return element(level, {"id": harness.harness.value}, f"{name} ", classed("span", "muted", f"({harness.model})"))


def _harness_sample(harness: HarnessView) -> Markup:
    h = harness.harness.value
    return classed(
        "p",
        "small muted",
        join(
            [
                "Sample: ",
                count(pointer("harnesses", h, "trunks", "total"), harness.trunks_total),
                " trunks on ",
                count(pointer("harnesses", h, "repositories"), harness.repositories),
                " repositories; ",
                count(pointer("harnesses", h, "trunks", "reached_s_t"), harness.reached_s_t),
                " reached S_T and ",
                count(pointer("harnesses", h, "trunks", "unreached"), harness.unreached),
                " did not. Defective at S_T: ",
                figure(harness.defective_at_s_t.as_figure(UNIT_TRUNKS, harness.repositories)),
            ]
        ),
    )


def _rows(harness: HarnessView, metric: ArmMetric, arms: Sequence[Arm] = ACCEPTANCE_ARMS) -> list[ChartRow]:
    return [ChartRow(view.arm.value, view.figure(metric)) for arm in arms if (view := harness.arm(arm)) is not None]


def _metric_chart(
    harness: HarnessView, metric: ArmMetric, run_id: str, arms: Sequence[Arm] = ACCEPTANCE_ARMS
) -> Markup:
    heading = copy.METRICS[metric].heading
    chart_id = f"chart-{harness.harness.value}-{metric.value}".replace("_", "-")
    caption = (
        f"{heading} per arm, dot at the estimate and bar over the interval. Run {run_id}, harness "
        f"{harness.harness.value}. {copy.METRICS[metric].definition}"
    )
    bounded = metric is not ArmMetric.REDUCTION
    return chart_figure(heading, caption, _rows(harness, metric, arms), bounded, chart_id)


def _arm_table(
    results: ResultsView, harness: HarnessView, metrics: Sequence[ArmMetric], arms: Sequence[Arm] = ACCEPTANCE_ARMS
) -> Markup:
    h = harness.harness.value
    rows: list[list[Content]] = []
    for arm in arms:
        view = harness.arm(arm)
        if view is None:
            continue
        rows.append(
            [
                view.arm.value,
                count(pointer("harnesses", h, "arms", arm.value, "trunks"), view.trunks),
                count(pointer("harnesses", h, "arms", arm.value, "accepted"), view.accepted),
                *(figure(view.figure(metric)) for metric in metrics),
            ]
        )
    header: list[Content] = ["Arm", "Trunks", "Accepted", *(copy.METRICS[m].heading for m in metrics)]
    caption = join([f"Harness {h}. Each cell: ", _interval_header(results), "."])
    return table(header, rows, caption=caption)


# ---------------------------------------------------------------- overview


def _sample_strip(results: ResultsView) -> Markup:
    stats = [
        (count("sample/tasks", results.sample.tasks), "tasks"),
        (count("sample/repositories", results.sample.repositories), "repositories"),
        (count("sample/trunks", results.sample.trunks), "trunks"),
        (count("sample/languages", list(results.sample.languages), ", ".join(results.sample.languages)), "languages"),
    ]
    return join(classed("div", "stat", value, classed("span", "label", label)) for value, label in stats)


def _case_card(case: CaseMaterial, root: str) -> Markup:
    record = case.record
    reading = read_gate_report(case.gate_report, record.criterion)
    blocking = next((f for f in reading.findings if f.blocking), None)
    outcome = case.trunk.arms.get(record.arm) if case.trunk is not None else None
    return element(
        "li",
        {"class": "card"},
        element("h3", None, link(f"{root}{case_path(record.case_id)}", f"{record.criterion}: {record.task_id}")),
        classed("p", "small muted", f"Arm {record.arm.value}, harness {record.harness.value}, {record.repository}"),
        element("p", None, classed("span", "quoted", reading.statement)),
        element(
            "p",
            None,
            "Gate: ",
            verdict(reading.verdict),
            f" ({blocking.category.replace('_', ' ')})" if blocking is not None else "",
        ),
        element("p", None, "Final: ", verdict(outcome.verdict.value) if outcome is not None else NOT_RECORDED),
    )


def _cards(cases: Sequence[CaseMaterial], root: str) -> Markup:
    if not cases:
        return classed("p", "note is-info", "The archive of this run holds no case record.")
    return element("ul", {"class": "cards"}, *(_case_card(case, root) for case in cases))


def overview(inputs: SiteInputs, results: ResultsView) -> Page:
    run_id = inputs.manifest.run_id
    headline = [case for case in inputs.cases if case.record.headline] or list(inputs.cases)
    comparison = join(
        join(
            [
                _harness_heading(harness, "h3"),
                _harness_sample(harness),
                coverage_note(pointer("harnesses", harness.harness.value, "repositories"), harness.repositories),
                _metric_chart(harness, ArmMetric.RESIDUAL_DEFECTS, run_id),
                _arm_table(results, harness, OVERVIEW_METRICS),
            ]
        )
        for harness in results.harnesses
    )
    method_sentence = join(
        [
            "Intervals are ",
            count("method/level", results.method.level, percent_of(results.method.level)),
            f" intervals ({results.method.interval}, ",
            count("method/resamples", results.method.resamples, f"{results.method.resamples:,}"),
            " resamples).",
        ]
    )
    body = fill(
        PageTemplate.OVERVIEW,
        {
            "sample_strip": _sample_strip(results),
            "label": count("label", results.label),
            "method_sentence": method_sentence,
            "headline_cases": _cards(headline, ""),
            "arm_comparison": comparison,
            "root": Markup(""),
        },
    )
    return Page(
        INDEX, copy.SITE_TITLE, "Results of the spec-proof benchmark, with every figure's interval and sample.", body
    )


# ---------------------------------------------------------------- arms and modules


def arms_page(inputs: SiteInputs, results: ResultsView) -> Page:
    run_id = inputs.manifest.run_id
    sections: list[Markup] = [
        element("h1", None, "Arms"),
        classed(
            "p",
            "lead",
            "Each arm gates the same changes. Residual defects, blocks on correct changes, cost and time per change, "
            "per harness; harnesses are never pooled.",
        ),
    ]
    for harness in results.harnesses:
        h = harness.harness.value
        sections += [
            _harness_heading(harness),
            _harness_sample(harness),
            coverage_note(pointer("harnesses", h, "repositories"), harness.repositories),
            _arm_table(results, harness, ARM_TABLE_METRICS),
            _metric_chart(harness, ArmMetric.RESIDUAL_DEFECTS, run_id),
            _metric_chart(harness, ArmMetric.REDUCTION, run_id),
            _metric_chart(harness, ArmMetric.FALSE_REJECTION, run_id),
            element("h3", None, "Cost of the gate against tests only"),
            _cost(harness),
            element("h3", None, "Secondary figures"),
            _arm_table(results, harness, SECONDARY_METRICS),
        ]
    return Page("arms.html", "Arms", "Residual defects, false blocks, cost and time per arm.", join(sections))


def _cost(harness: HarnessView) -> Markup:
    rows: list[list[Content]] = []
    if harness.extra_calls is not None:
        rows.append(["Extra model calls per accepted change, ALL minus T", figure(harness.extra_calls, "zero")])
    if harness.extra_seconds is not None:
        rows.append(["Extra minutes per accepted change, ALL minus T", figure(harness.extra_seconds, "zero")])
    if harness.placebo is not None:
        rows.append(["X of ALL minus X of the placebo PL", figure(harness.placebo, "zero")])
    if not rows:
        return classed("p", "muted", "This harness ran neither ALL nor T, so no difference is reported.")
    return table(["Difference", "Estimate, interval and sample"], rows)


def modules_page(inputs: SiteInputs, results: ResultsView) -> Page:
    run_id = inputs.manifest.run_id
    sections: list[Markup] = [
        element("h1", None, "Per module"),
        classed(
            "p",
            "lead",
            "TR, AL, DP and CO each install one module beside traceability; ALL installs them together. The composition "
            "check verifies that ALL's verdict on each criterion equals Shallnot's combination of the module proofs.",
        ),
    ]
    for harness in results.harnesses:
        h = harness.harness.value
        sections += [
            _harness_heading(harness),
            _harness_sample(harness),
            coverage_note(pointer("harnesses", h, "repositories"), harness.repositories),
            _arm_table(results, harness, OVERVIEW_METRICS, MODULE_ARMS),
            _metric_chart(harness, ArmMetric.REDUCTION, run_id, MODULE_ARMS),
            element("h3", None, "Composition check"),
            _composition(harness),
            element("h3", None, "Absent modules"),
            _absent(harness),
        ]
    return Page("modules.html", "Per module", "Per-module figures and the composition check.", join(sections))


def _composition(harness: HarnessView) -> Markup:
    if not harness.composition:
        return classed("p", "muted", "No composition row was recorded.")
    rows: list[list[Content]] = [
        [
            row.module.value,
            count(
                pointer("harnesses", harness.harness.value, "composition", row.module.value, "criteria"), row.criteria
            ),
            figure(row.mismatches.as_figure(UNIT_CRITERIA, harness.repositories)),
        ]
        for row in harness.composition
    ]
    return table(["Module", "Criteria", "Mismatches (Wilson interval)"], rows)


def _absent(harness: HarnessView) -> Markup:
    if not harness.absent_probes:
        return classed("p", "muted", "No absent-module probe was recorded.")
    rows: list[list[Content]] = [
        [
            probe.arm.value,
            probe.module.value,
            count(f"{probe.pointer}/probes", probe.probes),
            count(f"{probe.pointer}/counted_as_passed", probe.counted_as_passed),
        ]
        for probe in harness.absent_probes
    ]
    return table(["Arm", "Absent module", "Probes", "Counted as passed"], rows)


# ---------------------------------------------------------------- cases


def cases_index(inputs: SiteInputs) -> Page:
    rows: list[list[Content]] = []
    for case in inputs.cases:
        record = case.record
        reading = read_gate_report(case.gate_report, record.criterion)
        final = case.trunk.arms.get(record.arm) if case.trunk is not None else None
        rows.append(
            [
                link(f"{record.case_id}.html", record.case_id),
                f"{record.task_id} ({record.repository})",
                f"{record.harness.value}, {record.arm.value}",
                record.criterion,
                verdict(reading.verdict),
                verdict(final.verdict.value) if final is not None else NOT_RECORDED,
            ]
        )
    body = join(
        [
            element("h1", None, "Cases"),
            classed(
                "p",
                "lead",
                "Stops where the agent declared the work done with passing tests and an arm's gate did not accept it.",
            ),
            table(["Case", "Task", "Harness, arm", "Criterion", "Gate at the stop", "Final verdict"], rows)
            if rows
            else classed("p", "note is-info", "The archive of this run holds no case record."),
        ]
    )
    return Page(f"{CASES_DIRECTORY}/{INDEX}", "Cases", "Every archived case of the run.", body)


def _step(title: str, *children: Content) -> Markup:
    return element("li", None, element("h2", None, title), *children)


def _claim(case: CaseMaterial) -> Markup:
    if case.claim is None:
        return classed("p", "muted", NOT_RECORDED)
    return element("blockquote", {"class": "claim"}, element("p", None, case.claim))


def _test_run(reading: GateReading, criterion: str) -> Markup:
    rows: list[list[Content]] = [[test.name, test.classname, verdict(test.outcome)] for test in reading.tests]
    summary = classed(
        "p",
        "small",
        f"{reading.run.passed} of {reading.run.total} tests passed in the run the gate read; "
        f"{len(reading.tests)} cite {criterion}.",
    )
    return join([summary, table(["Test citing the criterion", "Class", "Outcome"], rows)])


def _findings(reading: GateReading) -> Markup:
    if not reading.findings:
        return classed("p", "muted", "The report holds no finding on this criterion.")
    rows: list[list[Content]] = [
        [
            finding.category.replace("_", " "),
            finding.module,
            "blocking" if finding.blocking else "not blocking",
            classed("span", "quoted", finding.message),
        ]
        for finding in reading.findings
    ]
    return table(["Finding", "Module", "Effect", "Message"], rows)


def _lines(n: int) -> str:
    return f"{n} line" if n == 1 else f"{n} lines"


def _mutant(case: CaseMaterial) -> Markup:
    mutant = case.mutant
    if mutant is None:
        return classed("p", "muted", "This block rests on a judged proof; no mutant is involved.")
    diff = unified_diff(mutant.before, mutant.after, mutant.file)
    stats = diff_stats(diff)
    survived = [r for r in mutant.results if r.outcome is MutantOutcome.SURVIVED]
    parts: list[Content] = [
        element(
            "p",
            None,
            "Mechanism: ",
            element("code", None, mutant.mechanism),
            f" ({_lines(stats.added)} added, {_lines(stats.removed)} removed).",
        ),
        render_diff(case.record.language, diff, f"Mutant of {mutant.file}"),
    ]
    if mutant.witness is not None:
        witness = mutant.witness
        parts.append(
            element(
                "p",
                None,
                "Witness: ",
                element("code", None, witness.call()),
                f" {witness.expected_kind} ",
                element("code", None, witness.expected),
                " under the criterion; the mutant does not.",
            )
        )
    rows: list[list[Content]] = [[result.test, result.outcome.value] for result in mutant.results]
    parts += [
        element(
            "p",
            None,
            f"Why the tests did not catch it: {len(survived)} of {len(mutant.results)} tests that cite "
            f"{case.record.criterion} pass on the mutant as they do on the code.",
        ),
        table(["Test", "On the mutant"], rows),
    ]
    return join(parts)


def _repair(case: CaseMaterial) -> Markup:
    parts: list[Content] = []
    if case.repair_report is not None:
        after = read_gate_report(case.repair_report, case.record.criterion)
        parts.append(element("p", None, "Gate after the repair: ", verdict(after.verdict), "."))
    if case.repair_diff is not None:
        parts.append(render_diff(case.record.language, case.repair_diff, "Repair"))
    final = case.trunk.arms.get(case.record.arm) if case.trunk is not None else None
    if final is not None:
        truth = final.final
        state = "not recorded" if truth is None else ("defective" if truth.defective else "not defective")
        parts.append(
            element(
                "p",
                None,
                f"Final verdict of {case.record.arm.value}: ",
                verdict(final.verdict.value),
                f"; the oracle finds the final change {state}.",
            )
        )
    if not parts:
        return classed("p", "muted", NOT_RECORDED)
    return join(parts)


def _arm_contrast(trunk: TrunkRecord | None) -> Markup:
    if trunk is None:
        return classed("p", "muted", "The run holds no trunk record for this task and harness.")
    s_t = trunk.s_t
    rows: list[list[Content]] = []
    for arm in Arm:
        result = trunk.arms.get(arm)
        if result is None:
            continue
        final = result.final
        rows.append(
            [
                arm.value,
                "not evaluated" if result.selection_pass is None else ("pass" if result.selection_pass else "block"),
                verdict(result.verdict.value),
                "not recorded" if final is None else ("defective" if final.defective else "not defective"),
            ]
        )
    truth = "not reached" if s_t is None else ("defective" if s_t.defective else "not defective")
    return join(
        [
            element("p", None, f"At S_T the oracle finds the change {truth}."),
            table(["Arm", "Gate at S_T", "Final verdict", "Final change"], rows),
        ]
    )


def _evidence(case: CaseMaterial) -> Markup:
    rows: list[list[Content]] = [
        [
            item.label,
            element("code", None, item.path),
            classed("span", "digest", f"sha256:{item.sha256}"),
            f"{item.size} bytes",
        ]
        for item in case.evidence
    ]
    return table(["Evidence", "Path", "Digest", "Size"], rows)


def case_page(case: CaseMaterial) -> Page:
    record = case.record
    reading = read_gate_report(case.gate_report, record.criterion)
    arm_line = f"Arm {record.arm.value}: {copy.ARM_DESCRIPTIONS[record.arm]}"
    if record.cheat_instruction is not None:
        arm_line += (
            f" Instruction ({record.cheat_instruction.value}): {copy.CHEAT_INSTRUCTIONS[record.cheat_instruction]}."
        )
    header = join(
        [
            classed("p", "small muted", link(INDEX, "Cases"), f" / {record.case_id}"),
            element("h1", None, f"{record.criterion} on {record.task_id}"),
            element(
                "dl",
                {"class": "facts"},
                element("dt", None, "Repository"),
                element("dd", None, f"{record.repository} ({record.language.value})"),
                element("dt", None, "Harness"),
                element("dd", None, f"{copy.HARNESS_NAMES[record.harness]} ({record.harness.value})"),
                element("dt", None, "Arm"),
                element("dd", None, arm_line),
            ),
        ]
    )
    steps = element(
        "ol",
        {"class": "steps"},
        _step(
            "The criterion",
            element(
                "div",
                {"class": "criterion"},
                element("code", None, record.criterion),
                " ",
                classed("span", "quoted", reading.statement),
            ),
        ),
        _step("The agent's claim", _claim(case)),
        _step("The test run", _test_run(reading, record.criterion)),
        _step("The gate's verdict", element("p", None, "Verdict: ", verdict(reading.verdict)), _findings(reading)),
        _step("The surviving mutant", _mutant(case)),
        _step("After the block", _repair(case)),
        _step("The same change under every arm", _arm_contrast(case.trunk)),
        _step("Evidence", _evidence(case)),
    )
    return Page(
        case_path(record.case_id),
        f"{record.criterion} on {record.task_id}",
        f"Case {record.case_id}: what the gate found when the agent declared the work done.",
        join([header, steps]),
    )


# ---------------------------------------------------------------- methodology, limits, reproduce


def methodology_page(inputs: SiteInputs, results: ResultsView) -> Page:
    arm_rows: list[list[Content]] = [
        [arm.value, NO_GATE if arm in UNGATED_ARMS else host_for(arm).value, copy.ARM_DESCRIPTIONS[arm]] for arm in Arm
    ]
    metric_list = element(
        "dl",
        {"class": "facts"},
        *(
            part
            for metric in ArmMetric
            for part in (
                element("dt", None, copy.METRICS[metric].heading),
                element("dd", None, copy.METRICS[metric].definition),
            )
        ),
    )
    interval_sentence = join(
        [
            "Every interval is a ",
            count("method/level", results.method.level, percent_of(results.method.level)),
            f" interval ({results.method.interval}) over ",
            count("method/resamples", results.method.resamples, f"{results.method.resamples:,}"),
            " resamples with seed ",
            count("method/seed", results.method.seed),
            ". Single proportions such as composition mismatches use Wilson intervals. The point estimate is "
            "computed on the summed counts of every repository; no interval is adjusted for multiplicity.",
        ]
    )
    adr_links = element(
        "ul",
        None,
        *(
            element("li", None, link(f"{copy.REPOSITORY_URL}/blob/{inputs.source_commit}/{record.path}", record.label))
            for record in copy.ADRS
        ),
    )
    body = fill(
        PageTemplate.METHODOLOGY,
        {
            "defect_rule": escape(
                f"A change is defective when a U or R test fails in at least {FAILURE_QUORUM} of {SCORING_RUNS} runs."
            ),
            "arm_table": table(["Arm", "Host", "Stop policy"], arm_rows),
            "acceptance_rule": escape(copy.ACCEPTANCE_RULE),
            "metric_list": metric_list,
            "interval_sentence": interval_sentence,
            "adr_links": adr_links,
        },
    )
    return Page("methodology.html", "Methodology", "How the benchmark measures each arm.", body)


def _deviation(entry: DeviationEntry) -> Markup:
    return element(
        "li",
        None,
        classed("span", "kind", entry.kind.value),
        f" arms: {entry.scope_text}. ",
        classed("span", "quoted", entry.text),
    )


def limits_page(inputs: SiteInputs, results: ResultsView) -> Page:
    sentences = [
        join(
            [
                f"{copy.HARNESS_NAMES[harness.harness]}: ",
                count(pointer("harnesses", harness.harness.value, "repositories"), harness.repositories),
                " repositories, ",
                count(pointer("harnesses", harness.harness.value, "trunks", "total"), harness.trunks_total),
                " trunks",
            ]
        )
        for harness in results.harnesses
    ]
    repositories_sentence = join(
        [
            join(sentences, "; "),
            ". Intervals on samples this small are too wide for any claim, and the cluster bootstrap undercovers "
            "with few repositories: every figure is descriptive.",
        ]
    )
    cheat_cases = sum(1 for case in inputs.cases if case.record.arm is Arm.CHEAT)
    cheat_sentence = "The results contain no cheat-arm figure. " + (
        "No case of this run comes from the cheat arm."
        if cheat_cases == 0
        else f"{cheat_cases} case{'s' if cheat_cases != 1 else ''} of this run come from the cheat arm."
    )
    deviations = (
        element("ol", {"class": "deviations"}, *(_deviation(entry) for entry in inputs.deviations))
        if inputs.deviations
        else classed("p", "muted", "The run logged no deviation.")
    )
    body = fill(
        PageTemplate.LIMITS,
        {
            "repositories_sentence": repositories_sentence,
            "cheat_sentence": escape(cheat_sentence),
            "deviations": deviations,
        },
    )
    return Page("limits.html", "Limits and deviations", "What the sample cannot show, and the run's deviations.", body)


def reproduce_page(inputs: SiteInputs) -> Page:
    manifest = inputs.manifest
    facts: list[tuple[str, Content]] = [
        ("Run", element("code", None, manifest.run_id)),
        ("Pre-registration tag", element("code", None, manifest.prereg_tag)),
        ("Corpus lock", classed("span", "digest", f"sha256:{manifest.corpus_lock_digest}")),
        ("Prices lock", classed("span", "digest", f"sha256:{manifest.prices_lock_digest}")),
        ("Shallnot commit", classed("span", "digest", manifest.provenance.shallnot_commit)),
        ("Source commit", classed("span", "digest", inputs.source_commit)),
        ("Generator", element("code", None, inputs.generator)),
        ("Seed", str(manifest.seed)),
        ("Operating system", manifest.os_name),
        ("Toolchains", ", ".join(f"{name} {version}" for name, version in sorted(manifest.toolchains.items()))),
        (
            "Harness models",
            ", ".join(
                f"{h.value}: {m}" for h, m in sorted(manifest.harness_models.items(), key=lambda kv: kv[0].value)
            ),
        ),
        ("Coding-agent CLIs", ", ".join(v for _, v in sorted(manifest.provenance.cli_versions.items()))),
        ("Sample tasks", ", ".join(manifest.sample_tasks)),
    ]
    provenance_facts = element(
        "dl",
        {"class": "facts"},
        *(part for label, value in facts for part in (element("dt", None, label), element("dd", None, value))),
    )
    digests = table(
        ["Artifact", "Digest"],
        [
            [name, classed("span", "digest", f"sha256:{digest}")]
            for name, digest in sorted(manifest.provenance.product_digests.items())
        ],
    )
    run = f"bench/runs/{manifest.run_id}"
    lines = (
        [
            "bench/script/site --fixtures",
            "",
            "# the same verb through make, from the repository root",
            'make bench-site ARGS="--fixtures"',
        ]
        if inputs.mode.is_synthetic
        else [
            "bench/script/verify-corpus --corpus bench/corpus",
            f"bench/script/analyze --run {run}",
            f"bench/script/site --run {run}",
            "",
            "# the same verbs through make, from the repository root",
            f'make bench-analyze ARGS="--run {run}"',
            f'make bench-site ARGS="--run {run}"',
        ]
    )
    commands = element(
        "pre",
        {"class": "code"},
        element("code", None, join(element("span", {"class": "line"}, line) for line in lines)),
    )
    body = fill(
        PageTemplate.REPRODUCE,
        {"provenance_facts": provenance_facts, "product_digests": digests, "commands": commands},
    )
    return Page(
        "reproduce.html", "How to reproduce", "Provenance of every figure and the commands that rebuild them.", body
    )


# ---------------------------------------------------------------- shell


def _nav(page: Page) -> Markup:
    return join(
        element(
            "a",
            {"href": f"{page.root}{item.path}", **({"aria-current": "page"} if item.path == page.path else {})},
            item.label,
        )
        for item in NAV
    )


def _provenance_footer(inputs: SiteInputs, root: str) -> Markup:
    manifest = inputs.manifest
    source = "the synthetic test fixtures" if inputs.mode.is_synthetic else f"run {manifest.run_id}"
    return join(
        [
            element(
                "p",
                None,
                f"Built from {source}. Run ",
                element("code", None, manifest.run_id),
                ", corpus lock ",
                classed("span", "digest", f"sha256:{manifest.corpus_lock_digest[:DIGEST_PREFIX_LENGTH]}"),
                ", source commit ",
                classed("span", "digest", inputs.source_commit[:DIGEST_PREFIX_LENGTH]),
                ", generator ",
                element("code", None, inputs.generator),
                ".",
            ),
            element("p", None, link(f"{root}reproduce.html", "Full provenance and how to reproduce")),
        ]
    )


def render_page(inputs: SiteInputs, page: Page) -> str:
    title = page.title if page.path == INDEX else f"{page.title} — {copy.SITE_TITLE}"
    badge = (
        element(
            "div",
            {"class": "synthetic-badge", "role": "note", "data-synthetic": "true"},
            SYNTHETIC_BADGE,
            element("span", None, SYNTHETIC_NOTICE),
        )
        if inputs.mode.is_synthetic
        else Markup("")
    )
    if inputs.mode.is_synthetic:
        title = f"[{SYNTHETIC_BADGE}] {title}"
    return fill(
        PageTemplate.PAGE,
        {
            "robots": escape("noindex" if inputs.mode.is_synthetic else "index"),
            "description": escape(page.description),
            "title": escape(title),
            "root": escape(page.root),
            "build": escape(inputs.mode.value),
            "badge": badge,
            "site_title": escape(copy.SITE_TITLE),
            "nav": _nav(page),
            "body": page.body,
            "provenance": _provenance_footer(inputs, page.root),
        },
    )
