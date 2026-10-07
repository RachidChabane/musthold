from __future__ import annotations

from hypothesis import given
from hypothesis import strategies as st

from calibration.domain.constants import (
    BYTES_PER_GIB,
    MEMORY_DEFERRAL_LIMIT,
    MEMORY_RESERVE_FRACTION,
    MEMORY_STOP_LIMIT,
)
from calibration.domain.memory_rules import (
    FitDecision,
    blocked_by_pressure,
    dynamic_decision,
    dynamic_fits,
    footprint_bytes,
    free_bytes,
    required_free_bytes,
    static_blocked,
)

TOTAL_MEMORY = 24 * BYTES_PER_GIB
KEV_9B_BASE_WEIGHTS = 12 * BYTES_PER_GIB
FULL_PERCENT = 100
FREE_PERCENTS = st.integers(min_value=0, max_value=FULL_PERCENT)


class TestFootprint:
    def test_it_is_the_weight_size_plus_one_gibibyte(self) -> None:
        assert footprint_bytes(5 * BYTES_PER_GIB) == 6 * BYTES_PER_GIB

    def test_an_empty_model_still_costs_the_overhead(self) -> None:
        assert footprint_bytes(0) == BYTES_PER_GIB


class TestStaticRule:
    def test_exactly_half_of_the_memory_is_not_blocked(self) -> None:
        assert not static_blocked(TOTAL_MEMORY // 2, TOTAL_MEMORY)

    def test_one_byte_above_half_is_blocked(self) -> None:
        assert static_blocked(TOTAL_MEMORY // 2 + 1, TOTAL_MEMORY)

    def test_kev_9b_with_base_weights_above_11_gib_is_blocked_on_24_gib(self) -> None:
        assert static_blocked(footprint_bytes(KEV_9B_BASE_WEIGHTS), TOTAL_MEMORY)

    def test_base_weights_of_11_gib_sit_on_the_limit(self) -> None:
        eleven_gib = 11 * BYTES_PER_GIB
        assert not static_blocked(footprint_bytes(eleven_gib), TOTAL_MEMORY)
        assert static_blocked(footprint_bytes(eleven_gib + 1), TOTAL_MEMORY)

    def test_a_small_model_is_admitted(self) -> None:
        assert not static_blocked(footprint_bytes(BYTES_PER_GIB), TOTAL_MEMORY)


class TestDynamicRule:
    FOOTPRINT = 6 * BYTES_PER_GIB

    def test_the_requirement_is_the_footprint_plus_fifteen_percent_of_the_total(self) -> None:
        assert (
            required_free_bytes(self.FOOTPRINT, TOTAL_MEMORY) == self.FOOTPRINT + MEMORY_RESERVE_FRACTION * TOTAL_MEMORY
        )

    def test_free_bytes_are_the_percentage_of_the_total(self) -> None:
        assert free_bytes(50, TOTAL_MEMORY) == TOTAL_MEMORY / 2

    def test_free_memory_equal_to_the_requirement_fits(self) -> None:
        total = 100 * BYTES_PER_GIB
        footprint = 25 * BYTES_PER_GIB
        assert dynamic_fits(40, total, footprint)

    def test_free_memory_below_the_requirement_does_not_fit(self) -> None:
        total = 100 * BYTES_PER_GIB
        footprint = 25 * BYTES_PER_GIB
        assert not dynamic_fits(39, total, footprint)

    @given(FREE_PERCENTS, FREE_PERCENTS)
    def test_fitting_is_monotone_in_free_memory(self, first: int, second: int) -> None:
        low, high = sorted((first, second))
        if dynamic_fits(low, TOTAL_MEMORY, self.FOOTPRINT):
            assert dynamic_fits(high, TOTAL_MEMORY, self.FOOTPRINT)

    @given(FREE_PERCENTS, st.integers(min_value=0, max_value=MEMORY_DEFERRAL_LIMIT - 1))
    def test_the_decision_agrees_with_the_fit_predicate(self, free_percent: int, prior: int) -> None:
        check = dynamic_decision(free_percent, TOTAL_MEMORY, self.FOOTPRINT, prior)
        assert (check.decision is FitDecision.LOAD) == dynamic_fits(free_percent, TOTAL_MEMORY, self.FOOTPRINT)


class TestDecisions:
    FOOTPRINT = 6 * BYTES_PER_GIB
    ENOUGH = FULL_PERCENT
    TOO_LITTLE = 0

    def test_enough_memory_loads_and_keeps_the_deferral_count(self) -> None:
        check = dynamic_decision(self.ENOUGH, TOTAL_MEMORY, self.FOOTPRINT, 3)
        assert (check.decision, check.deferrals) == (FitDecision.LOAD, 3)

    def test_too_little_memory_defers_and_counts_the_deferral(self) -> None:
        check = dynamic_decision(self.TOO_LITTLE, TOTAL_MEMORY, self.FOOTPRINT, 0)
        assert (check.decision, check.deferrals) == (FitDecision.DEFER, 1)

    def test_the_fifth_deferral_blocks_the_model(self) -> None:
        check = dynamic_decision(self.TOO_LITTLE, TOTAL_MEMORY, self.FOOTPRINT, MEMORY_DEFERRAL_LIMIT - 1)
        assert (check.decision, check.deferrals) == (FitDecision.BLOCK_MEMORY_FIT, MEMORY_DEFERRAL_LIMIT)

    def test_the_fourth_deferral_still_defers(self) -> None:
        check = dynamic_decision(self.TOO_LITTLE, TOTAL_MEMORY, self.FOOTPRINT, MEMORY_DEFERRAL_LIMIT - 2)
        assert check.decision is FitDecision.DEFER

    def test_the_check_records_its_inputs_and_computation(self) -> None:
        check = dynamic_decision(30, TOTAL_MEMORY, self.FOOTPRINT, 0)
        assert check.free_percent == 30
        assert check.total_memory == TOTAL_MEMORY
        assert check.footprint == self.FOOTPRINT
        assert check.required == required_free_bytes(self.FOOTPRINT, TOTAL_MEMORY)
        assert check.available == free_bytes(30, TOTAL_MEMORY)


class TestPressureStops:
    def test_a_first_stop_does_not_block(self) -> None:
        assert not blocked_by_pressure(MEMORY_STOP_LIMIT - 1)

    def test_a_second_stop_blocks(self) -> None:
        assert blocked_by_pressure(MEMORY_STOP_LIMIT)
