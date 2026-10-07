"""`results/REPORT.md` (§15.2): rendered from `decisions.json` and, once derived, `policy-inputs.json`."""

from __future__ import annotations

import math
from collections.abc import Mapping, Sequence
from typing import Final

from calibration.analysis.report_text import (
    CONCLUSION_BEGIN,
    CONCLUSION_END,
    EXPLORATORY_LABEL,
    LIMITS,
    QUOTED_BEGIN,
    QUOTED_END,
    SECTION_PREFIX,
    SECTIONS,
    SOURCE2_NOTE,
    TITLE,
    VENDOR_FIGURES,
    VENDOR_MARK,
)
from calibration.domain.jsonread import as_list, as_mapping
from calibration.domain.models import MODEL_BY_ID

DECIMALS: Final = 3
HOST_SOURCES: Final = ("JH5", "JH3", "VH2")
MISSING: Final = "n/a"
NEWLINE: Final = "\n"
POLICY_NOT_DERIVED: Final = "The proposals are written by `derive` to `policy-inputs.json`; none was derived yet."


def _num(value: object) -> str:
    if (
        isinstance(value, bool)
        or not isinstance(value, int | float)
        or (isinstance(value, float) and math.isnan(value))
    ):
        return MISSING
    if isinstance(value, int):
        return str(value)
    return f"{value:.{DECIMALS}f}"


def _estimate(value: object) -> str:
    record = as_mapping(value)
    if record is None:
        return MISSING
    return f"{_num(record.get('point'))} [{_num(record.get('lo'))}, {_num(record.get('hi'))}]"


def _table(headers: Sequence[str], rows: Sequence[Sequence[object]]) -> list[str]:
    lines = ["| " + " | ".join(headers) + " |", "|" + "|".join("---" for _ in headers) + "|"]
    lines.extend("| " + " | ".join(str(cell) for cell in row) + " |" for row in rows)
    return lines


def _cells(decisions: Mapping[str, object]) -> list[tuple[str, Mapping[str, object]]]:
    cells = as_mapping(decisions.get("cells")) or {}
    return [(key, as_mapping(value) or {}) for key, value in cells.items()]


def _model_name(cell: Mapping[str, object]) -> str:
    return str(cell.get("model"))


def _metrics(cell: Mapping[str, object]) -> Mapping[str, object]:
    return as_mapping(cell.get("metrics")) or {}


def _provenance(decisions: Mapping[str, object]) -> list[str]:
    provenance = as_mapping(decisions.get("provenance")) or {}
    checks = as_mapping(decisions.get("checks")) or {}
    keys = as_mapping(checks.get("keys")) or {}
    lines = [
        f"- run: {decisions.get('run_id')}, seed {decisions.get('seed')}, {decisions.get('resamples')} resamples",
        f"- pre-registration commit: {provenance.get('prereg_commit')}",
        f"- harness commit: {provenance.get('harness_commit')}",
        f"- pilot: {as_mapping(provenance.get('pilot'))}",
        f"- snapshot digest: {provenance.get('snapshot_digest')}",
        f"- product export digest: {provenance.get('product_export_digest')}",
        f"- builders: {as_mapping(provenance.get('builders'))}",
        f"- template digests: {as_mapping(provenance.get('template_digests'))}",
        f"- source-(2) prompt digests: {as_mapping(provenance.get('source2_prompt_digests'))}",
        "",
    ]
    rows = [
        [model, MODEL_BY_ID[model].description if model in MODEL_BY_ID else "", source]
        for model, source in sorted(keys.items())
    ]
    return [*lines, *_table(["Model", "Description", "confidenceSource"], rows)]


def _band_table(decisions: Mapping[str, object]) -> list[str]:
    rows = [
        [
            cell.get("kind"),
            cell.get("source"),
            _model_name(cell),
            cell.get("band"),
            (as_mapping(cell.get("counts")) or {}).get("with_prediction"),
            _estimate(_metrics(cell).get("auroc")),
            _estimate(_metrics(cell).get("ece")),
            cell.get("band_reason"),
        ]
        for _, cell in _cells(decisions)
    ]
    return [
        *_table(["Kind", "Source", "Model", "Band", "n", "AUROC", "ECE", "Reason"], rows),
        "",
        SOURCE2_NOTE,
    ]


def _conclusions(decisions: Mapping[str, object]) -> list[str]:
    lines = [CONCLUSION_BEGIN]
    lines.extend(f"- {cell.get('conclusion')}" for _, cell in _cells(decisions))
    lines.append(CONCLUSION_END)
    return lines


def _source3(decisions: Mapping[str, object]) -> list[str]:
    rows: list[list[object]] = []
    for _, cell in _cells(decisions):
        for point in as_list(cell.get("source3")) or []:
            record = as_mapping(point) or {}
            rows.append(
                [
                    cell.get("kind"),
                    cell.get("source"),
                    _model_name(cell),
                    f"{record.get('threshold')} ({record.get('rule')})",
                    _num(record.get("undetermined_rate")),
                    _num(record.get("error_rate_among_decided")),
                    record.get("composition"),
                ]
            )
        grid = [as_mapping(p) or {} for p in as_list(_metrics(cell).get("screen_all")) or []]
        for point in grid:
            if cell.get("source") == "S1" and point.get("tau") == 0.2:
                rows.append(
                    [
                        "K4",
                        "S1",
                        _model_name(cell),
                        "tau 0.20 screen-out",
                        _num(1 - (_float((as_mapping(point.get("flag_rate")) or {}).get("point")))),
                        _estimate(point.get("miss_rate")),
                        "screen-out rate and miss rate",
                    ]
                )
    return [
        *_table(
            ["Kind", "Source", "Model", "Operating point", "UNDETERMINED", "Error among decided", "Composition"], rows
        ),
        "",
        SOURCE2_NOTE,
    ]


def _float(value: object) -> float:
    return float(value) if isinstance(value, int | float) and not isinstance(value, bool) else math.nan


def _admission(decisions: Mapping[str, object]) -> list[str]:
    rows: list[list[object]] = []
    for _, cell in _cells(decisions):
        admission = as_mapping(cell.get("admission")) or {}
        main = as_mapping(admission.get("main")) or {}
        rows.append(
            [
                cell.get("kind"),
                cell.get("source"),
                _model_name(cell),
                cell.get("admission_outcome"),
                main.get("fit_decisions"),
                main.get("fit_clusters"),
                main.get("held_out_decisions"),
                main.get("selected_decisions"),
                _num(main.get("error_point")),
                _num(main.get("error_upper")),
                _num(main.get("coverage")),
            ]
        )
    return [
        *_table(
            [
                "Kind",
                "Source",
                "Model",
                "Outcome",
                "Fit n",
                "Fit clusters",
                "Held-out n",
                "Selected",
                "Error",
                "Upper",
                "Coverage",
            ],
            rows,
        ),
        "",
        SOURCE2_NOTE,
    ]


def _proposals(policy: Mapping[str, object] | None) -> list[str]:
    if policy is None:
        return [POLICY_NOT_DERIVED]
    entries = [as_mapping(e) or {} for e in as_list(policy.get("entries")) or []]
    rows = [[e.get("rule"), e.get("model"), e.get("policyKey"), e.get("source"), e.get("override")] for e in entries]
    recommendation = as_mapping(policy.get("recommendation")) or {}
    reported = [as_mapping(r) or {} for r in as_list(policy.get("reported")) or []]
    return [
        *_table(["Rule", "Model", "Key", "Source", "Override"], rows),
        "",
        f"CS3 endpoint model: {recommendation.get('model')} ({recommendation.get('reason')}).",
        "",
        *_table(["Rule", "Model", "Reason"], [[r.get("rule"), r.get("model"), r.get("reason")] for r in reported]),
    ]


def _reliability(decisions: Mapping[str, object]) -> list[str]:
    rows = [
        [
            cell.get("kind"),
            cell.get("source"),
            _model_name(cell),
            _estimate(_metrics(cell).get("ece")),
            _estimate(_metrics(cell).get("ece_top_label")),
            _estimate(_metrics(cell).get("ece_equal_mass")),
            _estimate(_metrics(cell).get("q09")),
            _estimate(_metrics(cell).get("brier")),
            _estimate(_metrics(cell).get("aurc")),
            _estimate(_metrics(cell).get("c05")),
            _num(_metrics(cell).get("c05u")),
        ]
        for _, cell in _cells(decisions)
    ]
    return [
        "Reliability tables: `results/reliability/<kind>__<source>__<model>.csv`; risk-coverage curves: "
        "`results/risk_coverage/<kind>__<source>__<model>.csv`.",
        "",
        *_table(
            [
                "Kind",
                "Source",
                "Model",
                "ECE",
                "ECE top-label",
                "ECE equal-mass",
                "Q0.9",
                "Brier",
                "AURC",
                "C05",
                "C05u",
            ],
            rows,
        ),
        "",
        SOURCE2_NOTE,
    ]


def _determinism(decisions: Mapping[str, object]) -> list[str]:
    rows: list[list[object]] = []
    for _, cell in _cells(decisions):
        record = as_mapping(cell.get("determinism"))
        if record is None:
            continue
        rows.append(
            [
                cell.get("kind"),
                _model_name(cell),
                record.get("pairs"),
                _num(record.get("identical_share")),
                _num(record.get("max_abs_delta_confidence")),
                _num(record.get("p99_abs_delta_confidence")),
                _estimate(record.get("flip_rate")),
                "UNSTABLE" if record.get("unstable") else "",
            ]
        )
    return _table(["Kind", "Model", "Pairs", "Identical", "Max delta", "P99 delta", "Flip rate", "Annotation"], rows)


def _comparison_rows(label: str, comparison: object) -> list[list[object]]:
    record = as_mapping(comparison)
    if record is None:
        return [[label, str(comparison), "", "", ""]]
    rows: list[list[object]] = []
    for metric in ("accuracy", "auroc", "ece", "brier"):
        entry = as_mapping(record.get(metric)) or {}
        rows.append(
            [
                label,
                record.get("shared"),
                metric,
                f"{_num(entry.get('first'))} / {_num(entry.get('second'))}",
                _estimate(entry.get("difference")),
            ]
        )
    return rows


def _comparisons(decisions: Mapping[str, object]) -> list[str]:
    comparisons = as_mapping(decisions.get("comparisons")) or {}
    versus = as_mapping(comparisons.get("source1_vs_source2")) or {}
    sizes = as_mapping(comparisons.get("model_sizes")) or {}
    simulated = as_mapping(comparisons.get("two_signal")) or {}
    header = ["Pair", "Shared", "Metric", "First / second", "Difference (paired cluster bootstrap)"]
    lines = [
        "Source (1) against source (2) on shared items; the inputs differ (the host votes answered the pilot's "
        "prompts).",
        "",
        *_table(header, [row for key, value in sorted(versus.items()) for row in _comparison_rows(key, value)]),
        "",
        SOURCE2_NOTE,
        "",
        "Two-signal ALIGNED simulation (ADR 0005 claim-power row ALIGNED):",
        "",
    ]
    two = [
        [
            source,
            r.get("threshold"),
            r.get("two_signal_aligned"),
            _estimate(r.get("false_aligned")),
            _num(r.get("kappa")),
            r.get("both_wrong"),
        ]
        for source, raw in sorted(simulated.items())
        for r in [as_mapping(raw) or {}]
    ]
    lines += _table(["Source", "Threshold", "Two-signal ALIGNED", "False ALIGNED", "kappa", "Both wrong"], two)
    lines += ["", "Model sizes (paired differences):", ""]
    size_rows = [
        row
        for kind, per_kind in sorted(sizes.items())
        for pair, comparison in sorted((as_mapping(per_kind) or {}).items())
        for row in _comparison_rows(f"{kind} {pair}", comparison)
    ]
    return [*lines, *_table(header, size_rows)]


def _checks(decisions: Mapping[str, object]) -> list[str]:
    checks = as_mapping(decisions.get("checks")) or {}
    per_model = as_mapping(checks.get("per_model")) or {}
    rows: list[list[object]] = []
    for model, raw in sorted(per_model.items()):
        record = as_mapping(raw) or {}
        server = "Ollama" if MODEL_BY_ID[model].is_ollama else "Kev"
        regression = as_mapping(record.get("screen_regression")) or {}
        spearman = as_mapping(record.get("spearman_confidence_vs_returned")) or {}
        rows.append(
            [
                model,
                f"returned probability ({server})",
                ", ".join(f"{kind} {_num(value)}" for kind, value in sorted(spearman.items())),
                record.get("answers_without_confidence"),
                f"{record.get('formula_answers_checked')} checked, {len(as_list(record.get('formula_findings')) or [])} outside tolerance",
                "failed" if regression.get("failed") else "passed",
                regression.get("without_probability"),
            ]
        )
    collisions = as_list(checks.get("key_collisions")) or []
    return [
        *_table(
            [
                "Model",
                "Measure (server)",
                "Spearman rho",
                "No confidence",
                "Formula check",
                "Screen regression",
                "No probability",
            ],
            rows,
        ),
        "",
        f"Key collisions: {collisions if collisions else 'none'}.",
    ]


def _sensitivity(decisions: Mapping[str, object]) -> list[str]:
    rows: list[list[object]] = []
    for _, cell in _cells(decisions):
        variants = as_mapping(cell.get("sensitivity")) or {}
        server = (
            "Ollama" if cell.get("model") in MODEL_BY_ID and MODEL_BY_ID[str(cell.get("model"))].is_ollama else "Kev"
        )
        for name, raw in variants.items():
            record = as_mapping(raw) or {}
            if name == "strata":
                continue
            label = {
                "primary": "PRIMARY",
                "consensus": "CONSENSUS",
                "alternative_measure": f"returned probability ({server})"
                if cell.get("source") == "S1"
                else "weakest stated",
            }.get(name, name)
            rows.append(
                [
                    cell.get("kind"),
                    cell.get("source"),
                    _model_name(cell),
                    label,
                    record.get("n"),
                    _estimate(record.get("accuracy")),
                    _estimate(record.get("auroc")),
                    _estimate(record.get("ece")),
                ]
            )
    return [
        *_table(["Kind", "Source", "Model", "Variant", "n", "Accuracy", "AUROC", "ECE"], rows),
        "",
        SOURCE2_NOTE,
        "",
        "Pass 2 in place of pass 1 and the K4 strata are in `decisions.json` under `sensitivity`.",
    ]


def _safety(decisions: Mapping[str, object]) -> list[str]:
    safety = as_mapping(decisions.get("safety")) or {}
    per_model = as_mapping(safety.get("per_model")) or {}
    rows = [
        [
            model,
            (as_mapping(raw) or {}).get("deferrals"),
            (as_mapping(raw) or {}).get("pressure_stops"),
            (as_mapping(raw) or {}).get("blocked"),
        ]
        for model, raw in sorted(per_model.items())
    ]
    return [
        *_table(["Model", "Deferrals", "Memory-pressure stops", "Blocked"], rows),
        "",
        f"Pauses logged: {safety.get('pauses')}; heavy-work waits: {safety.get('heavy_work_waits')}.",
    ]


def _deviations(decisions: Mapping[str, object]) -> list[str]:
    deviations = as_mapping(decisions.get("deviations")) or {}
    counts = as_mapping(deviations.get("counts")) or {}
    rows = [[kind, count] for kind, count in sorted(counts.items())]
    details = [as_mapping(d) or {} for d in as_list(deviations.get("details")) or []]
    return [
        *_table(["Kind", "Entries"], rows),
        "",
        "The entries are in `runs/<run-id>/DEVIATIONS.md`.",
        "",
        QUOTED_BEGIN,
        *(
            f"- {d.get('kind')} | scope: {', '.join(str(s) for s in as_list(d.get('scope')) or [])} | {d.get('text')}"
            for d in details
        ),
        QUOTED_END,
    ]


def _vendor() -> list[str]:
    rows = [[model, what, f"{figure} ({VENDOR_MARK})", url] for model, what, figure, url in VENDOR_FIGURES]
    return [
        "These figures come from the vendors' pages. No measured comparison uses them.",
        "",
        *_table(["Model", "Figure", "Value", "Source"], rows),
    ]


def _exploratory(decisions: Mapping[str, object]) -> list[str]:
    bands: dict[str, int] = {}
    for _, cell in _cells(decisions):
        bands[str(cell.get("band"))] = bands.get(str(cell.get("band")), 0) + 1
    counts = ", ".join(f"{count} {band}" for band, count in sorted(bands.items()))
    return [
        f"{EXPLORATORY_LABEL}: the cells fall into these bands: {counts}. Each band is read for one author-built "
        "fixture and one domain. Where a source ranks decisions above chance without reaching the calibration "
        "conditions, its confidence orders the work but its values are not error rates, and thresholds come from "
        "the derivations rather than from the stated values. The sensitivity tables show whether the reading "
        "depends on the truth set, the pass or the clustering."
    ]


def render_report(decisions: Mapping[str, object], policy: Mapping[str, object] | None) -> str:
    """The sixteen sections of §15.2, in order."""
    bodies = (
        _provenance(decisions),
        _band_table(decisions),
        _conclusions(decisions),
        _source3(decisions),
        _admission(decisions),
        _proposals(policy),
        _reliability(decisions),
        _determinism(decisions),
        _comparisons(decisions),
        _checks(decisions),
        _sensitivity(decisions),
        _safety(decisions),
        _deviations(decisions),
        _vendor(),
        [f"- {limit}" for limit in LIMITS],
        _exploratory(decisions),
    )
    lines = [TITLE, ""]
    for index, (title, body) in enumerate(zip(SECTIONS, bodies, strict=True), start=1):
        lines += [f"{SECTION_PREFIX}{index}. {title}", "", *body, ""]
    return NEWLINE.join(lines)
