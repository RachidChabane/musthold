from __future__ import annotations

import random

from hypothesis import given
from hypothesis import strategies as st

from specbench.domain.arm_policy import ARM_POLICIES, policy_for
from specbench.domain.arms import Arm, Host
from specbench.domain.cheat import CheatInstruction, assign_instructions

task_ids = st.sets(st.from_regex(r"[a-z][a-z0-9-]{0,10}", fullmatch=True), min_size=1, max_size=20)
seeds = st.integers(min_value=0, max_value=2**32)


def test_policies_are_total_over_the_arm_enum() -> None:
    assert set(ARM_POLICIES) == set(Arm)
    assert all(policy_for(arm).arm is arm for arm in Arm)


def test_honest_arms_never_use_the_plugin_host() -> None:
    honest = set(Arm) - {Arm.ALL_NATIVE, Arm.CHEAT}
    assert all(policy_for(arm).host is Host.ISOLATED_CLI for arm in honest)


def test_a_verdict_source_requires_what_the_arm_requires() -> None:
    for policy in ARM_POLICIES.values():
        source = policy_for(policy.verdict_from)
        assert source.required == policy.required


@given(seeds, task_ids)
def test_assignment_is_balanced(seed: int, tasks: set[str]) -> None:
    groups = list(assign_instructions(seed, tasks).values())
    a = groups.count(CheatInstruction.LEAVE_UNIMPLEMENTED)
    b = groups.count(CheatInstruction.FORGE_HOST_RESPONSES)
    assert a + b == len(tasks)
    assert 0 <= a - b <= 1


@given(seeds, task_ids, st.integers())
def test_assignment_is_seed_stable_and_independent_of_listing_order(seed: int, tasks: set[str], shuffle: int) -> None:
    shuffled = sorted(tasks)
    random.Random(shuffle).shuffle(shuffled)
    assert assign_instructions(seed, tasks) == assign_instructions(seed, shuffled)


@given(seeds)
def test_six_tasks_get_three_of_each(seed: int) -> None:
    result = assign_instructions(seed, [f"T{i}" for i in range(6)])
    assert list(result.values()).count(CheatInstruction.LEAVE_UNIMPLEMENTED) == 3
