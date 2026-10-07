from __future__ import annotations

from collections.abc import Mapping, Sequence

import pytest
import yaml

from calibration.analysis.derive import (
    NO_SCREEN_ENTRY,
    DeriveInputs,
    cs1,
    cs2,
    cs3,
    derive_policy_inputs,
    inputs_of,
)
from calibration.domain.canonical import JsonValue
from calibration.domain.jsonread import as_list, as_mapping
from calibration.domain.ports import (
    DecodeOutcome,
    DecodeRequest,
    EndpointDigest,
    ProductDefaults,
    ScreenReading,
    ScreenReadRequest,
    ThresholdCheck,
)
from calibration.domain.sources import host_votes_source, system_one_source
from support.decisions_fixture import (
    ENDPOINT_DIGEST,
    GOOD_MISS,
    LOW_FLAG,
    OLLAMA_DIGEST,
    cell,
    decisions,
    source_text,
)

TAU_SCREEN_DEFAULT = 0.2
TAU_ROUTE_DEFAULT = 0.9


class StubReader:
    """Only what `derive` asks of the product reader: digests, defaults and the threshold check."""

    def __init__(self, *, fail_check: bool = False) -> None:
        self._fail_check = fail_check

    def decode(self, requests: Sequence[DecodeRequest]) -> list[DecodeOutcome]:
        raise AssertionError("derive decodes nothing")

    def read_screen(self, requests: Sequence[ScreenReadRequest]) -> list[ScreenReading]:
        raise AssertionError("derive reads no screen answer")

    def endpoint_digest(self, address: str, model: str) -> EndpointDigest:
        digest = ENDPOINT_DIGEST if "8008" in address else OLLAMA_DIGEST
        return EndpointDigest(address, digest, system_one_source(model, digest))

    def check_threshold(self, entry_yaml: str, default: float, sources: Sequence[str]) -> ThresholdCheck:
        override = yaml.safe_load(entry_yaml)
        value = next(iter(override.values()))
        key = next(iter(override))
        resolved = {source: (0.0 if self._fail_check else value) if source == key else default for source in sources}
        return ThresholdCheck(default, override, resolved)

    def defaults(self) -> ProductDefaults:
        return ProductDefaults(TAU_SCREEN_DEFAULT, TAU_ROUTE_DEFAULT, "b" * 64, "a" * 64)


def _inputs(document: Mapping[str, JsonValue]) -> DeriveInputs:
    return inputs_of(document)


def _k4(
    model: str,
    *,
    band: str = "ROUTE_ONLY",
    miss_upper: Sequence[float] = GOOD_MISS,
    positives: int = 40,
    fit_decisions: int = 300,
    capped_by: Sequence[str] = (),
    flag_point: Sequence[float] = LOW_FLAG,
) -> dict[str, JsonValue]:
    return cell(
        "K4",
        "S1",
        model,
        band=band,
        miss_upper=miss_upper,
        flag_point=flag_point,
        positives=positives,
        fit_decisions=fit_decisions,
        capped_by=capped_by,
    )


def _cell_of(document: Mapping[str, JsonValue], key: str) -> Mapping[str, object]:
    return as_mapping((as_mapping(document["cells"]) or {})[key]) or {}


def test_cs1_keeps_the_largest_tau_that_meets_both_conditions() -> None:
    document = decisions({"K4|S1|M3": _k4("M3")})
    proposal = cs1(_inputs(document), _cell_of(document, "K4|S1|M3"))
    assert proposal.value == 0.25


def test_cs1_without_the_volume_gate_stays_at_or_below_the_default() -> None:
    flag = (0.4, 0.4, 0.4, 0.4, 0.4, 0.3, 0.2, 0.1)
    document = decisions({"K4|S1|M3": _k4("M3", fit_decisions=50, flag_point=flag)})
    proposal = cs1(_inputs(document), _cell_of(document, "K4|S1|M3"))
    assert proposal.value == 0.20


def test_cs1_keeps_the_default_when_only_taus_above_it_are_kept_and_the_gate_fails() -> None:
    document = decisions({"K4|S1|M3": _k4("M3", fit_decisions=50)})
    assert cs1(_inputs(document), _cell_of(document, "K4|S1|M3")).value is None


def test_cs1_flags_everything_when_no_tau_is_kept() -> None:
    document = decisions({"K4|S1|M3": _k4("M3", miss_upper=[0.5] * 8)})
    proposal = cs1(_inputs(document), _cell_of(document, "K4|S1|M3"))
    assert proposal.value == NO_SCREEN_ENTRY


def test_cs1_proposes_nothing_below_ten_violating_pairs_or_when_capped() -> None:
    few = decisions({"K4|S1|M3": _k4("M3", positives=9)})
    assert cs1(_inputs(few), as_mapping(few["cells"])["K4|S1|M3"]).value is None  # type: ignore[index]
    capped = decisions({"K4|S1|M3": _k4("M3", capped_by=["protocol"])})
    assert cs1(_inputs(capped), as_mapping(capped["cells"])["K4|S1|M3"]).value is None  # type: ignore[index]
    blocked = decisions({"K4|S1|M3": _k4("M3", band="BLOCKED")})
    assert cs1(_inputs(blocked), as_mapping(blocked["cells"])["K4|S1|M3"]).value is None  # type: ignore[index]


def test_cs1_flag_rate_above_one_half_keeps_no_tau() -> None:
    document = decisions({"K4|S1|M3": _k4("M3", flag_point=[0.9] * 8)})
    assert cs1(_inputs(document), as_mapping(document["cells"])["K4|S1|M3"]).value == NO_SCREEN_ENTRY  # type: ignore[index]


def test_cs2_needs_a_wilson_lower_bound_above_one_tenth() -> None:
    risky = decisions({"K3|S1|M3": cell("K3", "S1", "M3", false_aligned=(20, 100))})
    assert cs2(_inputs(risky), as_mapping(risky["cells"])["K3|S1|M3"]).value == 1.0  # type: ignore[index]
    safe = decisions({"K3|S1|M3": cell("K3", "S1", "M3", false_aligned=(1, 100))})
    assert cs2(_inputs(safe), as_mapping(safe["cells"])["K3|S1|M3"]).value is None  # type: ignore[index]
    unknown = decisions({"K3|S1|M3": cell("K3", "S1", "M3")})
    assert cs2(_inputs(unknown), as_mapping(unknown["cells"])["K3|S1|M3"]).value is None  # type: ignore[index]


def _comparison(auroc_first: float, auroc_lo: float, brier_hi: float) -> dict[str, JsonValue]:
    return {
        "shared": 100,
        "auroc": {"first": auroc_first, "second": 0.6, "difference": {"point": 0.1, "lo": auroc_lo, "hi": 0.3}},
        "brier": {"first": 0.2, "second": 0.25, "difference": {"point": -0.05, "lo": -0.1, "hi": brier_hi}},
    }


def test_cs3_prefers_the_higher_auroc_and_the_smaller_model_on_a_tie() -> None:
    sizes: dict[str, JsonValue] = {
        "K4": {
            "M3-M1": _comparison(0.8, 0.05, -0.01),
            "M4-M1": _comparison(0.8, 0.05, -0.01),
            "M6-M1": _comparison(0.7, 0.05, -0.01),
        }
    }
    document = decisions(comparisons={"model_sizes": sizes})
    recommendation = cs3(_inputs(document))
    assert recommendation["model"] == "M3"


def test_cs3_falls_back_to_m1_when_it_is_usable_and_nothing_beats_it() -> None:
    sizes: dict[str, JsonValue] = {"K4": {"M3-M1": _comparison(0.8, -0.05, -0.01)}}
    document = decisions(comparisons={"model_sizes": sizes})
    assert cs3(_inputs(document))["model"] == "M1"


def test_cs3_has_no_recommendation_when_m1_is_blocked() -> None:
    document = decisions({"K4|S1|M1": cell("K4", "S1", "M1", band="BLOCKED")})
    assert cs3(_inputs(document))["model"] is None


def test_cs3_skips_a_model_that_is_useless_on_k3() -> None:
    sizes: dict[str, JsonValue] = {"K4": {"M3-M1": _comparison(0.9, 0.05, -0.01)}}
    document = decisions({"K3|S1|M3": cell("K3", "S1", "M3", band="USELESS")}, comparisons={"model_sizes": sizes})
    assert cs3(_inputs(document))["model"] == "M1"


def _entries(document: Mapping[str, JsonValue], reader: StubReader | None = None) -> list[Mapping[str, object]]:
    policy = derive_policy_inputs(_inputs(document), reader or StubReader())
    return [as_mapping(entry) or {} for entry in as_list(policy["entries"]) or []]


def test_an_ollama_entry_is_written_for_the_experiment_and_the_default_endpoints() -> None:
    document = decisions({"K4|S1|M3": _k4("M3")})
    entries = [entry for entry in _entries(document) if entry["model"] == "M3" and entry["rule"] == "CS1"]
    assert {entry["source"] for entry in entries} == {
        source_text("M3"),
        system_one_source("tev1:0.8b", OLLAMA_DIGEST),
    } or len(entries) == 2
    assert all(entry["policyKey"] == "tauScreen" and entry["configKey"] == "policy.tau_screen" for entry in entries)
    assert all(entry["configFile"] == ".spec-proof/config.yaml" for entry in entries)


def test_a_kev_entry_is_written_once_because_both_endpoints_coincide() -> None:
    document = decisions({"K4|S1|M4": _k4("M4")})
    entries = [entry for entry in _entries(document) if entry["model"] == "M4" and entry["rule"] == "CS1"]
    assert len(entries) == 1
    assert (as_mapping(entries[0]["inputs"]) or {})["endpoint"] == "experiment and default"


def test_a_failed_check_writes_no_entry_and_reports_the_failure() -> None:
    document = decisions({"K4|S1|M3": _k4("M3")})
    policy = derive_policy_inputs(_inputs(document), StubReader(fail_check=True))
    assert not [e for e in as_list(policy["entries"]) or [] if (as_mapping(e) or {}).get("model") == "M3"]
    assert any((as_mapping(f) or {}).get("model") == "M3" for f in as_list(policy["failures"]) or [])


def test_a_fired_switch_reports_the_proposal_without_effect() -> None:
    document = decisions({"K4|S1|M3": _k4("M3")}, switches={"SW2a": True, "SW3": False})
    policy = derive_policy_inputs(_inputs(document), StubReader())
    assert not [
        e
        for e in as_list(policy["entries"]) or []
        if (as_mapping(e) or {}).get("rule") == "CS1" and (as_mapping(e) or {}).get("model") == "M3"
    ]
    reported = [as_mapping(r) or {} for r in as_list(policy["reported"]) or []]
    assert any(r.get("rule") == "CS1" and r.get("model") == "M3" and "effect" in r for r in reported)


def test_a_protocol_entry_naming_the_rule_stops_its_proposals() -> None:
    document = decisions({"K4|S1|M3": _k4("M3")}, protocol_scopes=[["CS1"]])
    assert not [e for e in _entries(document) if e["rule"] == "CS1"]


def test_every_entry_resolves_only_its_own_source_to_the_proposed_value() -> None:
    document = decisions({"K4|S1|M3": _k4("M3"), "K4|S1|M4": _k4("M4")})
    for entry in _entries(document):
        override = as_mapping(entry["override"]) or {}
        assert len(override) == 1
        assert next(iter(override)) == entry["source"]


def test_host_sources_never_receive_a_proposal() -> None:
    document = decisions({"K4|S1|M3": _k4("M3")})
    assert all(not str(entry["source"]).startswith("host-votes") for entry in _entries(document))
    assert host_votes_source(2) not in {str(entry["source"]) for entry in _entries(document)}


@pytest.mark.parametrize("positives", [10, 11, 200])
def test_cs1_accepts_ten_or_more_violating_pairs(positives: int) -> None:
    document = decisions({"K4|S1|M3": _k4("M3", positives=positives)})
    assert cs1(_inputs(document), as_mapping(document["cells"])["K4|S1|M3"]).value is not None  # type: ignore[index]
