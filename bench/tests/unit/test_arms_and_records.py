from __future__ import annotations

import pytest

from specbench.domain.arms import (
    ACCEPTANCE_ARMS,
    FROZEN_MODEL,
    JUDGE_REPLAY_MODEL,
    S_T_ARMS,
    Arm,
    Harness,
    Host,
    host_for,
)
from specbench.domain.jsonread import RecordError
from specbench.domain.records import trunk_from_record


def test_only_the_in_session_arms_use_the_plugin_host() -> None:
    assert {arm for arm in Arm if host_for(arm) is Host.PLUGIN} == {Arm.ALL_NATIVE, Arm.CHEAT}
    assert all(host_for(arm) is Host.ISOLATED_CLI for arm in S_T_ARMS)


def test_every_harness_has_one_frozen_model_and_a_replay_model_of_another_family_or_tier() -> None:
    assert FROZEN_MODEL == {Harness.CLAUDE_CODE: "claude-sonnet-5-5", Harness.CODEX: "gpt-6-luna"}
    assert all(JUDGE_REPLAY_MODEL[h] != FROZEN_MODEL[h] for h in Harness)


def test_acceptance_arms_leave_out_n_and_the_cheat_arm() -> None:
    assert Arm.N not in ACCEPTANCE_ARMS and Arm.CHEAT not in ACCEPTANCE_ARMS
    assert set(ACCEPTANCE_ARMS) - {Arm.ALL_NATIVE} <= set(S_T_ARMS)


def trunk(replay_model: str) -> dict[str, object]:
    truth = {"defective": False, "defective_full": False, "o_failed": False}
    return {
        "arms": {},
        "harness": "codex",
        "judge_replay": {"replay_model": replay_model, "same_pass": True, "replay_pass": True},
        "language": "go",
        "model": FROZEN_MODEL[Harness.CODEX],
        "reached_s_t": True,
        "repository": "acme/shelf",
        "s_n": truth,
        "s_t": truth,
        "schema": "specbench.trunk/1",
        "task_id": "T1",
    }


def test_the_judge_replay_runs_through_the_model_the_harness_names() -> None:
    assert trunk_from_record(trunk(JUDGE_REPLAY_MODEL[Harness.CODEX])).judge_replay is not None  # type: ignore[arg-type]
    with pytest.raises(RecordError):
        trunk_from_record(trunk("claude-opus-5-5"))  # type: ignore[arg-type]
