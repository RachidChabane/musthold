"""Renders `results/REPORT.md` from `prereg/REPORT.tmpl.md` (PREREG 13)."""

from __future__ import annotations

import math
import re
from collections import Counter
from collections.abc import Mapping, Sequence

from pilot.analysis.bands import Outcome
from pilot.analysis.composite import named_component
from pilot.analysis.pipeline import AnalysisOutput
from pilot.analysis.results import EndpointResult, n_needed_text
from pilot.analysis.table import Table, format_cell
from pilot.domain import analysis_constants as k

PLACEHOLDER = re.compile(r"^\{\{(?P<name>[a-z_]+)\}\}\s*$")
CONCLUSION_LINE = re.compile(r'^- (?P<outcome>GO|NO-GO|INCONCLUSIVE): "(?P<text>.*)"\s*$')
NO_EXPLORATORY = "No exploratory discussion has been written for this run."
FAILURE_ROWS_SHOWN = 40
DECIMALS = 3
BAND_SEPARATOR = "; "


class TemplateError(Exception):
    """The report template lacks a required part."""


def estimate_text(result: EndpointResult) -> str:
    if math.isnan(result.point):
        return "undefined"
    return f"{result.point:.{DECIMALS}f} [{result.lo:.{DECIMALS}f}, {result.hi:.{DECIMALS}f}]"


def parse_conclusion_templates(template: str) -> dict[str, str]:
    inside = False
    found: dict[str, str] = {}
    for line in template.splitlines():
        if line.strip() == k.REPORT_CONCLUSION_BEGIN:
            inside = True
        elif line.strip() == k.REPORT_CONCLUSION_END:
            break
        elif inside and (match := CONCLUSION_LINE.match(line)):
            found[match.group("outcome")] = match.group("text")
    missing = {o.value for o in Outcome} - set(found)
    if missing:
        raise TemplateError(f"conclusion templates missing: {sorted(missing)}")
    return found


def conclusion_for(result: EndpointResult, templates: Mapping[str, str]) -> str:
    text = templates[result.outcome.value]
    interval = f"[{result.lo:.{DECIMALS}f}, {result.hi:.{DECIMALS}f}]"
    text = text.replace("<endpoint>", result.id).replace("[lo, hi]", interval)
    text = text.replace(", n)", f", n = {result.n})")
    if "<component>" in text:
        text = text.replace("<component>", named_component(result))
    return text.replace("<n_needed>", str(n_needed_text(result)))


def render_conclusions(output: AnalysisOutput, templates: Mapping[str, str]) -> str:
    lines = [f"- {conclusion_for(r, templates)}" for r in output.endpoints]
    return "\n".join([k.QUOTED_BEGIN_MARKER, *lines, k.QUOTED_END_MARKER])


def decision_table(output: AnalysisOutput) -> Table:
    ordered = sorted(output.endpoints, key=lambda r: (math.isnan(r.margin), r.margin))
    rows = [
        [r.id, r.limit, r.measure, r.n, estimate_text(r), r.band_go, r.band_nogo,
         r.outcome.value if r.outcome is r.uncapped_outcome else f"{r.outcome.value} (band: {r.uncapped_outcome.value})",
         ", ".join(r.capped_by)]
        for r in ordered
    ]
    return Table("decision_table", "decision table, least favourable bound first",
                 ["ID", "limit", "measure", "n", "estimate [lo, hi]", "GO", "NO-GO", "outcome", "capped by"], rows)


def provenance_block(provenance: Mapping[str, object]) -> str:
    """Values are echoed from run files, so the block is quoted text for the lint."""
    lines = [f"- {key}: {value}" for key, value in provenance.items()]
    return "\n".join([k.QUOTED_BEGIN_MARKER, *lines, k.QUOTED_END_MARKER])


def feasibility_block(output: AnalysisOutput) -> str:
    table = Table("feasibility", "feasibility", ["ID", "check", "value", "status"],
                  [[f.id, f.description, f.value, f.status] for f in output.feasibility])
    return f"{table.markdown()}\n\nProtocol verdict: {output.feasibility_verdict}"


def preconditions_block(output: AnalysisOutput) -> str:
    table = Table("preconditions", "preconditions", ["ID", "precondition", "status", "detail", "affects"],
                  [[p.id, p.description, p.status.value, p.detail, ", ".join(p.affects)] for p in output.preconditions])
    return table.markdown()


def blind_spot_block(output: AnalysisOutput) -> str:
    return "\n\n".join(f"**{t.title}**\n\n{t.markdown()}" for t in output.blind_spot_tables)


def confusion_block(output: AnalysisOutput) -> str:
    return "\n\n".join(f"**{t.title}**\n\n{t.markdown()}" for t in output.tables)


def multiplication_block(output: AnalysisOutput) -> str:
    em = output.error_multiplication
    links = Table("links", "per-link error rates", ["link", "false pass", "false block", "definition"],
                  [[link.name, link.false_pass, link.false_block, link.detail] for link in em.links])
    summary = Table("summary", "end-to-end error", ["measure", "predicted under independence", "observed", "ratio", "ratio 95% interval"], [
        ["false pass (gold bad test passed by G3, single-test scenarios)", em.predicted_false_pass, em.observed_false_pass,
         em.ratio_false_pass, f"[{format_cell(em.ratio_false_pass_interval.lo)}, {format_cell(em.ratio_false_pass_interval.hi)}]"],
        ["false block (gold good test blocked by G3, single-test scenarios)", em.predicted_false_block, em.observed_false_block,
         em.ratio_false_block, f"[{format_cell(em.ratio_false_block_interval.lo)}, {format_cell(em.ratio_false_block_interval.hi)}]"],
    ])
    validity = (f"Mutant validity against the oracle: false VALID rate {format_cell(em.validity_false_valid)}, "
                f"false-discard rate {format_cell(em.validity_false_discard)}.")
    attribution = [f"- {r.id}: {r.attribution['errors']} error(s); removed by {r.attribution['removed_by']}; "
                   f"component {r.attribution['component']}" for r in output.endpoints if r.attribution]
    return "\n\n".join([links.markdown(), summary.markdown(), validity, "Attribution by substitution:", "\n".join(attribution)])


def failure_block(table: Table) -> str:
    counts = Counter(row[0] for row in table.rows)
    summary = "; ".join(f"{kind}: {n}" for kind, n in sorted(counts.items())) or "none"
    shown = Table(table.name, table.title, table.header, table.rows[:FAILURE_ROWS_SHOWN])
    more = len(table.rows) - FAILURE_ROWS_SHOWN
    tail = f"\n\n{more} further row(s) are in results/failure_modes.csv." if more > 0 else ""
    return f"Catalogue size by kind: {summary}.\n\n{shown.markdown()}{tail}"


def cost_block(output: AnalysisOutput) -> str:
    return f"{output.cost_table.markdown()}\n\n{output.timing_table.markdown()}"


def sensitivity_block(output: AnalysisOutput) -> str:
    lines = []
    for row in output.sensitivity:
        lines.append(
            f"- {row.variant}, {row.result.id}: {row.variant} {estimate_text(row.result)} n = {row.result.n} "
            f"({row.result.outcome.value}); PRIMARY {estimate_text(row.primary)} n = {row.primary.n} "
            f"({row.primary.outcome.value})"
        )
    return "\n".join(lines)


LIMITS_PARAGRAPH = (
    "Limits: one language; one domain; 38 tests and 10 criteria; author-assigned labels; ground truth "
    "relative to the violation set and the oracle; relabels and audits done by models; one model executes every "
    "product role; the contrast model shares its family; the fixture, its tests and labels were authored in a "
    "session of the contrast model; the oracle's author model is not recorded; unadjusted intervals."
)


def render_report(
    template: str, output: AnalysisOutput, provenance: Mapping[str, object], exploratory: str | None
) -> str:
    templates = parse_conclusion_templates(template)
    blocks = {
        "provenance": provenance_block(provenance),
        "decision_table": decision_table(output).markdown(),
        "feasibility": feasibility_block(output),
        "preconditions": preconditions_block(output),
        "blind_spot": blind_spot_block(output),
        "confusion": confusion_block(output),
        "multiplication": multiplication_block(output),
        "failure_modes": failure_block(output.failure_table),
        "cost": cost_block(output),
        "sensitivity": sensitivity_block(output),
        "composite": output.composite.text,
        "exploratory": exploratory.strip() if exploratory else NO_EXPLORATORY,
    }
    return _substitute(template, blocks, render_conclusions(output, templates))


def _substitute(template: str, blocks: Mapping[str, str], conclusions: str) -> str:
    rendered: list[str] = []
    skipping = False
    for line in template.splitlines():
        stripped = line.strip()
        if stripped == k.REPORT_CONCLUSION_BEGIN:
            rendered.append(conclusions)
            skipping = True
            continue
        if stripped == k.REPORT_CONCLUSION_END:
            skipping = False
            continue
        if skipping:
            continue
        match = PLACEHOLDER.match(line)
        if match is None:
            rendered.append(line)
        elif match.group("name") in blocks:
            rendered.append(blocks[match.group("name")])
        else:
            raise TemplateError(f"unknown placeholder {{{{{match.group('name')}}}}}")
    return "\n".join(rendered) + "\n"


def endpoint_documents(endpoints: Sequence[EndpointResult]) -> list[dict[str, object]]:
    return [e.to_mapping() for e in endpoints]
