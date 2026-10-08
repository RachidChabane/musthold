from __future__ import annotations

import tempfile
from pathlib import Path

from hypothesis import given, settings
from hypothesis import strategies as st

from specbench.adapters.machine_slots import SLOT_COUNT, MachineSlots, SlotHold
from specbench.domain.night_budget import NightInputs, NightJob, canary_claim, construct_claim, share_of

amounts = st.integers(min_value=0, max_value=400)


@settings(max_examples=60, deadline=None)
@given(st.lists(st.one_of(st.just(1), st.just(2), st.just(0)), max_size=30))
def test_no_sequence_of_acquisitions_and_releases_holds_more_than_two_slots(operations: list[int]) -> None:
    """0 releases the oldest hold; 1 and 2 try to take that many slots."""
    with tempfile.TemporaryDirectory() as directory:
        slots = MachineSlots(Path(directory))
        holds: list[SlotHold] = []
        for operation in operations:
            if operation == 0:
                if holds:
                    holds.pop(0).release()
            else:
                hold = slots.try_acquire(operation)
                if hold is not None:
                    holds.append(hold)
            held = [index for hold in holds for index in hold.indices]
            assert len(held) <= SLOT_COUNT
            assert len(set(held)) == len(held)
            assert slots.free() == SLOT_COUNT - len(held)
        for hold in holds:
            hold.release()
        assert slots.free() == SLOT_COUNT


@given(
    cap=amounts,
    pilot_spent=amounts,
    canary_reserve=amounts,
    allowance=amounts,
    construct_spent=amounts,
    pending=st.booleans(),
)
def test_the_shares_never_exceed_the_cap_and_never_overlap(
    cap: int, pilot_spent: int, canary_reserve: int, allowance: int, construct_spent: int, pending: bool
) -> None:
    night = NightInputs(cap, pilot_spent, canary_reserve, allowance, construct_spent, pending)
    shares = {job: share_of(job, night) for job in NightJob}
    assert all(0 <= share <= cap for share in shares.values())
    assert shares[NightJob.CONSTRUCT] <= allowance
    if pilot_spent <= cap:
        committed = pilot_spent + canary_claim(night) + construct_claim(night) + shares[NightJob.SAMPLE]
        assert committed <= max(cap, pilot_spent + canary_claim(night) + construct_spent)
        assert pilot_spent + canary_claim(night) + shares[NightJob.CONSTRUCT] <= cap


@given(cap=amounts, pilot_spent=amounts, more=amounts, canary_reserve=amounts, allowance=amounts, pending=st.booleans())
def test_what_the_pilot_spends_never_raises_a_lower_priority_share(
    cap: int, pilot_spent: int, more: int, canary_reserve: int, allowance: int, pending: bool
) -> None:
    before = NightInputs(cap, pilot_spent, canary_reserve, allowance, 0, pending)
    after = NightInputs(cap, pilot_spent + more, canary_reserve, allowance, 0, pending)
    for job in (NightJob.CANARIES, NightJob.CONSTRUCT, NightJob.SAMPLE):
        assert share_of(job, after) <= share_of(job, before)
