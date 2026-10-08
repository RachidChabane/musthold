from __future__ import annotations

import pytest

from specbench.domain.arm_policy import (
    ALL_MODULES,
    GATED_STOP,
    MAX_ATTEMPTS,
    MAX_HOST_WAITS,
    PLACEBO_MESSAGE,
    TRUNK_STOP,
    StopKind,
    policy_for,
)
from specbench.domain.arms import Arm, EvaluationKind, Host, Module, host_for


def test_the_trunk_stops_on_tests_only_and_gated_branches_use_the_product_caps() -> None:
    assert TRUNK_STOP.kind is StopKind.TESTS and TRUNK_STOP.max_attempts == MAX_ATTEMPTS == 3
    assert GATED_STOP.kind is StopKind.GATE and GATED_STOP.max_host_waits == MAX_HOST_WAITS == 3
    assert GATED_STOP.max_consecutive_blocks == 6


def test_arms_without_shallnot_are_n_and_t() -> None:
    assert {arm for arm in Arm if not policy_for(arm).has_shallnot} == {Arm.N, Arm.T}


@pytest.mark.parametrize(
    ("arm", "require"),
    [
        (Arm.N, ()),
        (Arm.T, ()),
        (Arm.TR, ()),
        (Arm.AL, ("alignment",)),
        (Arm.DP, ("discriminating-power",)),
        (Arm.CO, ()),
        (Arm.ALL, ("alignment", "discriminating-power")),
        (Arm.PL, ("alignment", "discriminating-power")),
        (Arm.ALL_NATIVE, ("alignment", "discriminating-power")),
        (Arm.CHEAT, ("alignment", "discriminating-power")),
    ],
)
def test_proofs_require_per_arm(arm: Arm, require: tuple[str, ...]) -> None:
    assert policy_for(arm).proofs_require == require


def test_modules_installed_per_arm() -> None:
    assert policy_for(Arm.TR).modules == {Module.SHALLNOT}
    assert policy_for(Arm.AL).modules == {Module.SHALLNOT, Module.ALIGNMENT}
    assert policy_for(Arm.DP).modules == {Module.SHALLNOT, Module.DISCRIMINATING_POWER}
    assert policy_for(Arm.CO).modules == {Module.SHALLNOT, Module.COMPLETENESS}
    assert policy_for(Arm.ALL).modules == ALL_MODULES


def test_completeness_never_gates() -> None:
    assert all(Module.COMPLETENESS not in policy_for(arm).required for arm in Arm)


def test_co_accepts_as_tr_and_does_not_branch() -> None:
    co = policy_for(Arm.CO)
    assert co.verdict_from is Arm.TR and not co.branches and co.evaluation is EvaluationKind.SELECTION_ONLY


def test_placebo_blocks_where_all_blocks_with_the_fixed_message() -> None:
    pl = policy_for(Arm.PL)
    assert pl.verdict_from is Arm.ALL and pl.branches
    assert pl.message_override == PLACEBO_MESSAGE == "Re-read the spec and strengthen your tests"
    assert [a for a in Arm if policy_for(a).message_override is not None] == [Arm.PL]


def test_branching_arms_are_the_forked_gated_arms() -> None:
    assert {a for a in Arm if policy_for(a).branches} == {Arm.TR, Arm.AL, Arm.DP, Arm.ALL, Arm.PL}


def test_the_policy_host_is_the_one_host_for_returns() -> None:
    assert all(policy_for(a).host is host_for(a) for a in Arm)
    assert policy_for(Arm.CHEAT).host is Host.PLUGIN


def test_the_spec_gap_evaluation_kind_is_distinct_from_every_arm_evaluation() -> None:
    assert EvaluationKind.SPEC_GAP not in {policy_for(a).evaluation for a in Arm}
