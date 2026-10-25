from __future__ import annotations

import pytest

from specbench.domain.arms import Module
from specbench.domain.host_requests import request_id_of, request_kind_of, ticket_in_prompt
from specbench.domain.jsonread import RecordError
from specbench.domain.sessions import RequestKind

REQUEST_ID = "864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38"
TICKET = f"{REQUEST_ID}.0.1"


def test_the_ticket_is_found_among_the_padding_of_a_launch_prompt() -> None:
    assert ticket_in_prompt(f"Mind the boundary at 50. ticket {TICKET} Mind the boundary at 50.") == TICKET


def test_a_prompt_with_no_ticket_or_two_tickets_names_none() -> None:
    assert ticket_in_prompt("no ticket here") is None
    assert ticket_in_prompt(f"ticket {TICKET} ticket {REQUEST_ID}.1.1") is None


def test_the_same_ticket_written_twice_is_still_one_ticket() -> None:
    assert ticket_in_prompt(f"ticket {TICKET} again {TICKET}") == TICKET


def test_the_request_id_is_the_ticket_without_vote_and_emission() -> None:
    assert request_id_of(TICKET) == REQUEST_ID


def test_a_string_that_is_not_a_ticket_is_refused() -> None:
    with pytest.raises(RecordError):
        request_id_of("not-a-ticket")


@pytest.mark.parametrize(
    ("module", "kind", "expected"),
    [
        (Module.DISCRIMINATING_POWER, "generate.targeted", RequestKind.MUTANTS),
        (Module.DISCRIMINATING_POWER, "generate.witnessStrategy", RequestKind.MUTANTS),
        (Module.DISCRIMINATING_POWER, "generate.witness", RequestKind.WITNESSES),
        (Module.DISCRIMINATING_POWER, "judge.scope", RequestKind.SCOPE),
        (Module.DISCRIMINATING_POWER, "judge.validity", RequestKind.VALIDITY),
        (Module.DISCRIMINATING_POWER, "judge.equivalent", RequestKind.EQUIVALENCE),
        (Module.DISCRIMINATING_POWER, "decide.closed", RequestKind.CLOSED_QUESTION),
        (Module.ALIGNMENT, "judge.alignment", RequestKind.ALIGNMENT),
        (Module.ALIGNMENT, "decide.closed", RequestKind.CLOSED_QUESTION),
        (Module.COMPLETENESS, "decide.closed", RequestKind.COMPLETENESS),
    ],
)
def test_a_product_request_kind_maps_to_the_benchmarks_kind(module: Module, kind: str, expected: RequestKind) -> None:
    assert request_kind_of(module, kind) is expected


@pytest.mark.parametrize("kind", ["judge.unheard-of", "generate.promises", "judge.labels"])
def test_an_unknown_request_kind_is_refused(kind: str) -> None:
    with pytest.raises(RecordError, match="unknown request kind"):
        request_kind_of(Module.DISCRIMINATING_POWER, kind)
