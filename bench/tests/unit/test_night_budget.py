from __future__ import annotations

from datetime import date

import pytest

from specbench.application.night_budget import NightBudget, NightSettings
from specbench.domain.arms import Harness
from specbench.domain.call_ledger import EMPTY_LEDGER, CallLedger
from specbench.domain.night_budget import (
    DEFAULT_CONSTRUCT_ALLOWANCE,
    DEFAULT_NIGHTLY_CAP,
    NightInputs,
    NightJob,
    construct_claim,
    share_of,
)

NIGHT = date(2026, 10, 8)
CLAUDE = Harness.CLAUDE_CODE
CODEX = Harness.CODEX


def inputs(
    pilot_spent: int = 0,
    canary_reserve: int = 1,
    construct_spent: int = 0,
    construct_pending: bool = True,
    cap: int = DEFAULT_NIGHTLY_CAP,
) -> NightInputs:
    return NightInputs(
        cap, pilot_spent, canary_reserve, DEFAULT_CONSTRUCT_ALLOWANCE, construct_spent, construct_pending
    )


def legacy_construct_budget(pilot_spent: int) -> int:
    """Construction's budget before the shared computation: the cap less the pilot and one canary call, capped."""
    return min(DEFAULT_CONSTRUCT_ALLOWANCE, max(0, DEFAULT_NIGHTLY_CAP - pilot_spent - 1))


@pytest.mark.parametrize("pilot_spent", [0, 1, 100, 129, 130, 200, 249, 250, 300])
def test_construction_gets_what_the_pilot_and_one_canary_call_leave_while_it_has_work(pilot_spent: int) -> None:
    assert share_of(NightJob.CONSTRUCT, inputs(pilot_spent=pilot_spent)) == legacy_construct_budget(pilot_spent)


def test_construction_is_given_nothing_once_every_selected_task_is_sealed() -> None:
    assert share_of(NightJob.CONSTRUCT, inputs(construct_pending=False)) == 0


def test_the_sample_receives_what_the_pilot_the_canaries_and_construction_leave() -> None:
    assert share_of(NightJob.SAMPLE, inputs(pilot_spent=50, canary_reserve=4)) == 250 - 50 - 4 - 120
    assert share_of(NightJob.SAMPLE, inputs(pilot_spent=50, canary_reserve=4, construct_pending=False)) == 250 - 54


def test_the_sample_cannot_take_executions_construction_already_spent() -> None:
    assert construct_claim(inputs(construct_spent=130, construct_pending=False)) == 130
    assert share_of(NightJob.SAMPLE, inputs(construct_spent=130, construct_pending=False)) == 250 - 1 - 130


def test_the_pilot_is_first_in_line_and_the_canaries_are_second() -> None:
    assert share_of(NightJob.PILOT, inputs(pilot_spent=250)) == DEFAULT_NIGHTLY_CAP
    assert share_of(NightJob.CANARIES, inputs(pilot_spent=248, canary_reserve=5)) == 2
    assert share_of(NightJob.CANARIES, inputs(pilot_spent=250, canary_reserve=5)) == 0


def test_a_negative_amount_of_executions_is_refused() -> None:
    with pytest.raises(ValueError):
        inputs(pilot_spent=-1)


def budget(
    pilot: CallLedger = EMPTY_LEDGER,
    construct: CallLedger = EMPTY_LEDGER,
    sample: CallLedger = EMPTY_LEDGER,
    pending: bool = True,
    recorded: dict[Harness, int] | None = None,
    explicit: dict[Harness, int] | None = None,
) -> NightBudget:
    class Ledgers:
        def read(self, job: NightJob, harness: Harness) -> CallLedger:
            del harness
            return {NightJob.PILOT: pilot, NightJob.CONSTRUCT: construct, NightJob.SAMPLE: sample}.get(
                job, EMPTY_LEDGER
            )

    class Progress:
        def has_unsealed_task(self) -> bool:
            return pending

    class Canaries:
        def measured(self, harness: Harness) -> int | None:
            return (recorded or {}).get(harness)

        def record(self, harness: Harness, executions: int) -> None:
            raise AssertionError("reading the budget records nothing")

    settings = NightSettings({CLAUDE: 250, CODEX: 100}, DEFAULT_CONSTRUCT_ALLOWANCE, explicit or {})
    return NightBudget(Ledgers(), Progress(), Canaries(), settings)


def test_the_pilots_claude_key_counts_against_the_claude_cap() -> None:
    pilot = CallLedger.from_record({"2026-10-08": {"claude": 200}})
    assert budget(pilot=pilot).share(NightJob.CONSTRUCT, CLAUDE, NIGHT) == 49


def test_the_codex_cap_is_separate_from_the_claude_cap() -> None:
    pilot = CallLedger.from_record({"2026-10-08": {"claude": 250, "codex": 10}})
    sample = budget(pilot=pilot)
    assert sample.share(NightJob.SAMPLE, CLAUDE, NIGHT) == 0
    assert sample.share(NightJob.SAMPLE, CODEX, NIGHT) == 100 - 10


def test_construction_never_draws_on_the_codex_cap() -> None:
    assert budget().share(NightJob.CONSTRUCT, CODEX, NIGHT) == 0
    assert budget().share(NightJob.SAMPLE, CODEX, NIGHT) == 100


def test_the_canary_reserve_is_the_explicit_value_then_the_recorded_one_then_one_call() -> None:
    assert budget().explain(CLAUDE, NIGHT)["canary_reserve"] == 1
    assert budget(recorded={CLAUDE: 9}).explain(CLAUDE, NIGHT)["canary_reserve"] == 9
    assert budget(recorded={CLAUDE: 9}, explicit={CLAUDE: 3}).explain(CLAUDE, NIGHT)["canary_reserve"] == 3
    assert budget().explain(CODEX, NIGHT)["canary_reserve"] == 0


def test_explain_reports_every_input_and_every_share() -> None:
    sample = CallLedger.from_record({"2026-10-08": {"claude-code": 17}})
    report = budget(sample=sample).explain(CLAUDE, NIGHT)
    assert report["sample_spent"] == 17
    assert report["shares"] == {"pilot": 250, "canaries": 1, "construct": 120, "sample": 250 - 1 - 120}
