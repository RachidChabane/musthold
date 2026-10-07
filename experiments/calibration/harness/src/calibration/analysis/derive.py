"""`derive` (§12): the per-source threshold proposals CS1 and CS2 and the endpoint-model recommendation CS3, in the keys
the modules use, from `decisions.json` alone."""

from __future__ import annotations

import math
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Final

import yaml

from calibration.analysis.bands import Band
from calibration.analysis.stats.admission import MIN_FIT_CLUSTERS, MIN_FIT_DECISIONS
from calibration.analysis.stats.intervals import wilson
from calibration.domain.canonical import JsonValue
from calibration.domain.constants import (
    FLAG_POINT_LIMIT,
    KEV_DEFAULT_ADDRESS,
    MISS_UPPER_LIMIT,
    OLLAMA_DEFAULT_ADDRESS,
    SCREEN_TAU_DEFAULT,
)
from calibration.domain.jsonread import as_list, as_mapping
from calibration.domain.models import MODEL_BY_ID, MODELS, ModelSpec
from calibration.domain.ports import ProductReader
from calibration.domain.sources import host_votes_source, parse_system_one_source

MIN_VIOLATING_PAIRS: Final = 10
CS2_FALSE_ALIGNED_LOWER: Final = 0.10
CS2_ENTRY_VALUE: Final = 1.00
NO_SCREEN_ENTRY: Final = 0.00
SCREEN_MODULE: Final = "discpower"
ROUTE_MODULE: Final = "alignment"
SCREEN_CONFIG: Final = (".spec-proof/config.yaml", "policy.tau_screen", "tauScreen")
ROUTE_CONFIG: Final = (".spec-proof/alignment.yaml", "policy.tau_route", "tauRoute")
RULE_CS1: Final = "CS1"
RULE_CS2: Final = "CS2"
RULE_CS3: Final = "CS3"
SIGNAL_SW2A: Final = "SW2a"
SIGNAL_SW3: Final = "SW3"
CELL_SEPARATOR: Final = "|"
MEASURED_HOST_VOTES: Final = (2, 3, 5)
ENDPOINT_EXPERIMENT: Final = "experiment"
ENDPOINT_DEFAULT: Final = "default"
ENDPOINT_BOTH: Final = "experiment and default"
NO_EFFECT_BANDS: Final = (Band.BLOCKED.value, Band.INCONCLUSIVE_DATA.value)
SCREEN_SOURCE: Final = "S1"


@dataclass(frozen=True)
class DeriveInputs:
    """What the rules read of `decisions.json`."""

    cells: Mapping[str, Mapping[str, object]]
    comparisons: Mapping[str, object]
    provenance: Mapping[str, object]
    switches: Mapping[str, object]
    protocol_scopes: tuple[tuple[str, ...], ...]


def _cell(inputs: DeriveInputs, kind: str, model: str) -> Mapping[str, object] | None:
    return inputs.cells.get(CELL_SEPARATOR.join((kind, SCREEN_SOURCE, model)))


def _number(value: object) -> float:
    return float(value) if isinstance(value, int | float) and not isinstance(value, bool) else math.nan


def _capped(inputs: DeriveInputs, cell: Mapping[str, object], rule: str) -> bool:
    """Blocked, INCONCLUSIVE (data) or capped cells, and a protocol entry that names the rule, propose nothing."""
    capped = as_list(cell.get("capped_by")) or []
    if cell.get("band") in NO_EFFECT_BANDS or capped:
        return True
    return any(rule in scope or "all" in scope for scope in inputs.protocol_scopes)


def _grid(cell: Mapping[str, object]) -> list[Mapping[str, object]]:
    metrics = as_mapping(cell.get("metrics")) or {}
    return [as_mapping(point) or {} for point in as_list(metrics.get("screen_stratum")) or []]


def _volume_gate(cell: Mapping[str, object]) -> bool:
    """§10 precondition for the key: at least 200 labelled decisions in at least 10 clusters in the fit half."""
    admission = as_mapping(cell.get("admission")) or {}
    main = as_mapping(admission.get("main")) or {}
    return (
        _number(main.get("fit_decisions")) >= MIN_FIT_DECISIONS
        and _number(main.get("fit_clusters")) >= MIN_FIT_CLUSTERS
    )


@dataclass(frozen=True)
class Proposal:
    value: float | None
    reason: str
    inputs: dict[str, JsonValue]


def cs1(inputs: DeriveInputs, cell: Mapping[str, object]) -> Proposal:
    """`tauScreen` of one model: the largest tau of the grid whose miss-rate decision upper bound is at most 0.10 and
    whose flag-rate point is at most 0.50 on the screen stratum."""
    grid = _grid(cell)
    detail: dict[str, JsonValue] = {"stratum_points": len(grid)}
    if _capped(inputs, cell, RULE_CS1):
        return Proposal(None, "the inputs are blocked, INCONCLUSIVE or capped by a deviation", detail)
    positives = int(_number(grid[0].get("positives"))) if grid else 0
    detail["violating_pairs"] = positives
    if positives < MIN_VIOLATING_PAIRS:
        return Proposal(None, f"fewer than {MIN_VIOLATING_PAIRS} VIOLATES-truth pairs in the screen stratum", detail)
    undefined = any(
        math.isnan(_number((as_mapping(p.get("miss_rate")) or {}).get("hi")))
        or math.isnan(_number((as_mapping(p.get("flag_rate")) or {}).get("point")))
        for p in grid
    )
    if undefined:
        return Proposal(None, "an estimate of the screen stratum is undefined", detail)
    kept = [
        _number(p["tau"])
        for p in grid
        if _number((as_mapping(p.get("miss_rate")) or {}).get("hi")) <= MISS_UPPER_LIMIT
        and _number((as_mapping(p.get("flag_rate")) or {}).get("point")) <= FLAG_POINT_LIMIT
    ]
    gate = _volume_gate(cell)
    detail["volume_gate_holds"] = gate
    detail["kept_taus"] = list(kept)
    if not kept:
        return Proposal(NO_SCREEN_ENTRY, "no tau is kept: every survivor is flagged", detail)
    allowed = kept if gate else [tau for tau in kept if tau <= SCREEN_TAU_DEFAULT]
    if not allowed:
        return Proposal(NO_SCREEN_ENTRY, "no kept tau at or below 0.20 without the volume gate", detail)
    return Proposal(max(allowed), "the largest kept tau", detail)


def cs2(inputs: DeriveInputs, cell: Mapping[str, object]) -> Proposal:
    """`tauRoute` of one model: 1.00 when the Wilson lower bound of the false-ALIGNED rate at confidence 0.90 exceeds
    0.10."""
    detail: dict[str, JsonValue] = {}
    if _capped(inputs, cell, RULE_CS2):
        return Proposal(None, "the inputs are blocked, INCONCLUSIVE or capped by a deviation", detail)
    metrics = as_mapping(cell.get("metrics")) or {}
    counts = as_mapping(metrics.get("false_aligned_counts"))
    if counts is None:
        return Proposal(None, "no ALIGNED decision at confidence 0.90 or no K3 metric", detail)
    successes, total = int(_number(counts.get("successes"))), int(_number(counts.get("total")))
    lower = wilson(successes, total).lo
    detail.update({"false_aligned": successes, "decisions": total, "wilson_lower": lower})
    if total > 0 and lower > CS2_FALSE_ALIGNED_LOWER:
        return Proposal(CS2_ENTRY_VALUE, "the Wilson lower bound of the false-ALIGNED rate exceeds 0.10", detail)
    return Proposal(None, "the false-ALIGNED rate does not exclude 0.10", detail)


def _entries(
    module_config: tuple[str, str, str],
    module: str,
    model: ModelSpec,
    cell: Mapping[str, object],
    proposal: Proposal,
    template: object,
    rule: str,
    sources: Sequence[tuple[str, str]],
) -> list[dict[str, JsonValue]]:
    config_file, config_key, policy_key = module_config
    value = NO_SCREEN_ENTRY if proposal.value is None else proposal.value
    entries: list[dict[str, JsonValue]] = []
    for source, endpoint in sources:
        entries.append(
            {
                "module": module,
                "configFile": config_file,
                "configKey": config_key,
                "policyKey": policy_key,
                "source": source,
                "override": {source: value},
                "kind": "K4" if module == SCREEN_MODULE else "K3",
                "model": model.model_id,
                "templateDigest": template if isinstance(template, str) else None,
                "rule": rule,
                "inputs": {**proposal.inputs, "endpoint": endpoint, "reason": proposal.reason},
            }
        )
    return entries


def _source_pairs(reader: ProductReader, cell: Mapping[str, object], model: ModelSpec) -> list[tuple[str, str]]:
    """The exact source of the experiment's endpoint and of the canonical default address; for Kev they coincide."""
    source = cell.get("confidence_source")
    parsed = parse_system_one_source(source) if isinstance(source, str) else None
    if parsed is None:
        return []
    default_address = OLLAMA_DEFAULT_ADDRESS if model.is_ollama else KEV_DEFAULT_ADDRESS
    default_source = reader.endpoint_digest(default_address, parsed.model).source
    if default_source == source:
        return [(str(source), ENDPOINT_BOTH)]
    return [(str(source), ENDPOINT_EXPERIMENT), (default_source, ENDPOINT_DEFAULT)]


def _measured_sources(inputs: DeriveInputs) -> list[str]:
    s1 = [
        str(cell["confidence_source"])
        for cell in inputs.cells.values()
        if isinstance(cell.get("confidence_source"), str)
    ]
    return sorted({*s1, *(host_votes_source(count) for count in MEASURED_HOST_VOTES)})


def _checked(
    reader: ProductReader,
    entry: dict[str, JsonValue],
    default: float,
    measured: Sequence[str],
) -> str | None:
    """The product's own reading of the entry: it must resolve its source to the proposed value and every other
    measured source to the default. Returns the failure, or None."""
    override = as_mapping(entry["override"]) or {}
    source, value = next(iter(override.items()))
    keyed = yaml.safe_dump({source: value})
    others = [candidate for candidate in measured if candidate != source]
    try:
        result = reader.check_threshold(keyed, default, [source, *others])
    except Exception as failure:
        return str(failure)
    if result.resolved.get(source) != value:
        return f"For({source}) resolves to {result.resolved.get(source)}, not {value}"
    for other in others:
        if result.resolved.get(other) != default:
            return f"For({other}) resolves to {result.resolved.get(other)}, not the default {default}"
    return None


def cs3(inputs: DeriveInputs) -> dict[str, JsonValue]:
    """The endpoint model for the relay's `systemone.model`: not a threshold."""
    sizes = as_mapping(inputs.comparisons.get("model_sizes")) or {}
    k4 = as_mapping(sizes.get("K4")) or {}
    baseline = _cell(inputs, "K4", "M1")
    if baseline is None or baseline.get("band") == Band.BLOCKED.value:
        return {"rule": RULE_CS3, "model": None, "reason": "M1 is blocked: no recommendation"}
    if any(RULE_CS3 in scope or "all" in scope for scope in inputs.protocol_scopes):
        return {"rule": RULE_CS3, "model": None, "reason": "a protocol entry caps the rule"}
    candidates: list[tuple[float, float, str]] = []
    for model in MODELS:
        if model.model_id == "M1":
            continue
        comparison = as_mapping(k4.get(f"{model.model_id}-M1"))
        k4_cell, k3_cell = _cell(inputs, "K4", model.model_id), _cell(inputs, "K3", model.model_id)
        if comparison is None or k4_cell is None or k3_cell is None:
            continue
        auroc = as_mapping(comparison.get("auroc")) or {}
        brier = as_mapping(comparison.get("brier")) or {}
        auroc_lo = _number((as_mapping(auroc.get("difference")) or {}).get("lo"))
        brier_hi = _number((as_mapping(brier.get("difference")) or {}).get("hi"))
        meets = k4_cell.get("band") not in NO_EFFECT_BANDS and k3_cell.get("band") != Band.USELESS.value
        if auroc_lo > 0 and brier_hi < 0 and meets:
            candidates.append(
                (-_number(auroc.get("first")), MODEL_BY_ID[model.model_id].parameters_billion, model.model_id)
            )
    if candidates:
        best = min(candidates)
        return {"rule": RULE_CS3, "model": best[2], "reason": "better AUROC and Brier than M1 on K4"}
    if baseline.get("band") in (Band.THRESHOLD_READY.value, Band.ROUTE_ONLY.value):
        return {"rule": RULE_CS3, "model": "M1", "reason": "M1 is THRESHOLD_READY or ROUTE_ONLY on K4"}
    return {"rule": RULE_CS3, "model": None, "reason": "no model qualifies"}


def derive_policy_inputs(inputs: DeriveInputs, reader: ProductReader) -> dict[str, JsonValue]:
    """The entries of CS1 and CS2 that pass the product's own check, what was reported without effect, the CS3
    recommendation and the failures that must be logged as `protocol` entries."""
    defaults = reader.defaults()
    measured = _measured_sources(inputs)
    entries: list[dict[str, JsonValue]] = []
    reported: list[JsonValue] = []
    failures: list[JsonValue] = []
    for model in MODELS:
        for rule, kind, switch, config, module, template_key, proposer, default in (
            (RULE_CS1, "K4", SIGNAL_SW2A, SCREEN_CONFIG, SCREEN_MODULE, "discpower", cs1, defaults.tau_screen),
            (RULE_CS2, "K3", SIGNAL_SW3, ROUTE_CONFIG, ROUTE_MODULE, "alignment", cs2, defaults.tau_route),
        ):
            cell = _cell(inputs, kind, model.model_id)
            if cell is None:
                continue
            proposal = proposer(inputs, cell)
            record: dict[str, JsonValue] = {
                "rule": rule,
                "model": model.model_id,
                "reason": proposal.reason,
                "inputs": proposal.inputs,
            }
            if proposal.value is None:
                reported.append({**record, "proposal": None})
                continue
            if inputs.switches.get(switch) is True:
                reported.append({**record, "proposal": proposal.value, "effect": "none: the pilot's switch has fired"})
                continue
            template = as_mapping(inputs.provenance.get("template_digests"))
            for entry in _entries(
                config,
                module,
                model,
                cell,
                proposal,
                (template or {}).get(template_key),
                rule,
                _source_pairs(reader, cell, model),
            ):
                failure = _checked(reader, entry, default, measured)
                if failure is None:
                    entries.append(entry)
                else:
                    failures.append({"rule": rule, "model": model.model_id, "failure": failure})
    return {
        "entries": list(entries),
        "reported": reported,
        "recommendation": cs3(inputs),
        "failures": failures,
        "defaults": {"tauScreen": defaults.tau_screen, "tauRoute": defaults.tau_route},
    }


def inputs_of(decisions: Mapping[str, object]) -> DeriveInputs:
    """Reads `decisions.json` into the inputs of the rules."""
    cells = {key: as_mapping(value) or {} for key, value in (as_mapping(decisions.get("cells")) or {}).items()}
    deviations = as_mapping(decisions.get("deviations")) or {}
    scopes = tuple(
        tuple(str(name) for name in as_list((as_mapping(entry) or {}).get("scope")) or [])
        for entry in as_list(deviations.get("details")) or []
        if (as_mapping(entry) or {}).get("kind") == "protocol"
    )
    return DeriveInputs(
        cells=cells,
        comparisons=as_mapping(decisions.get("comparisons")) or {},
        provenance=as_mapping(decisions.get("provenance")) or {},
        switches=as_mapping(decisions.get("pilot_switches")) or {},
        protocol_scopes=scopes,
    )
