from __future__ import annotations

from collections.abc import Sequence

from hypothesis import HealthCheck, given, settings
from hypothesis import strategies as st

from calibration.analysis.derive import cs1, inputs_of
from calibration.domain.constants import SCREEN_TAU_DEFAULT
from calibration.domain.jsonread import as_mapping
from support.decisions_fixture import cell, decisions

GRID_SIZE = 8
rates = st.lists(st.floats(min_value=0.0, max_value=1.0), min_size=GRID_SIZE, max_size=GRID_SIZE)


def _proposal(miss_upper: Sequence[float], flag_point: Sequence[float], *, gate: bool) -> float | None:
    document = decisions(
        {
            "K4|S1|M3": cell(
                "K4", "S1", "M3", miss_upper=miss_upper, flag_point=flag_point, fit_decisions=300 if gate else 10
            )
        }
    )
    selected = as_mapping((as_mapping(document["cells"]) or {})["K4|S1|M3"]) or {}
    return cs1(inputs_of(document), selected).value


@settings(max_examples=60, suppress_health_check=[HealthCheck.too_slow], deadline=None)
@given(miss=rates, flag=rates, index=st.integers(0, GRID_SIZE - 1), bump=st.floats(0.0, 1.0))
def test_a_worse_miss_bound_never_raises_the_proposed_tau(
    miss: list[float], flag: list[float], index: int, bump: float
) -> None:
    worse = list(miss)
    worse[index] = min(1.0, worse[index] + bump)
    before, after = _proposal(miss, flag, gate=True), _proposal(worse, flag, gate=True)
    assert before is not None and after is not None
    assert after <= before


@settings(max_examples=60, suppress_health_check=[HealthCheck.too_slow], deadline=None)
@given(miss=rates, flag=rates, index=st.integers(0, GRID_SIZE - 1), bump=st.floats(0.0, 1.0))
def test_a_worse_flag_rate_never_raises_the_proposed_tau(
    miss: list[float], flag: list[float], index: int, bump: float
) -> None:
    worse = list(flag)
    worse[index] = min(1.0, worse[index] + bump)
    before, after = _proposal(miss, flag, gate=True), _proposal(miss, worse, gate=True)
    assert before is not None and after is not None
    assert after <= before


@settings(max_examples=60, suppress_health_check=[HealthCheck.too_slow], deadline=None)
@given(miss=rates, flag=rates)
def test_without_the_volume_gate_the_proposal_never_exceeds_the_default(miss: list[float], flag: list[float]) -> None:
    proposal = _proposal(miss, flag, gate=False)
    assert proposal is None or proposal <= SCREEN_TAU_DEFAULT


@settings(max_examples=60, suppress_health_check=[HealthCheck.too_slow], deadline=None)
@given(miss=rates, flag=rates)
def test_the_gate_never_lowers_the_proposal(miss: list[float], flag: list[float]) -> None:
    gated, ungated = _proposal(miss, flag, gate=True), _proposal(miss, flag, gate=False)
    assert gated is not None
    assert ungated is None or gated >= ungated
