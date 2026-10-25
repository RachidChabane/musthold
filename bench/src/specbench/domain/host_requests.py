"""Reading the requests a gate host answers: tickets in executor prompts, and the kind of request a ticket belongs to."""

from __future__ import annotations

import re
from typing import Final

from specbench.domain.arms import Module
from specbench.domain.jsonread import RecordError
from specbench.domain.sessions import RequestKind

TICKET_PATTERN: Final = re.compile(r"[0-9a-f]{64}\.\d+\.\d+")
KIND_GENERATE_TARGETED: Final = "generate.targeted"
KIND_GENERATE_WITNESS_STRATEGY: Final = "generate.witnessStrategy"
KIND_GENERATE_WITNESS: Final = "generate.witness"
KIND_JUDGE_SCOPE: Final = "judge.scope"
KIND_JUDGE_VALIDITY: Final = "judge.validity"
KIND_JUDGE_EQUIVALENT: Final = "judge.equivalent"
KIND_JUDGE_ALIGNMENT: Final = "judge.alignment"
KIND_DECIDE_CLOSED: Final = "decide.closed"


def ticket_in_prompt(prompt: str) -> str | None:
    """The single ticket an executor prompt holds, or `None` when it holds none or several."""
    found = set(TICKET_PATTERN.findall(prompt))
    return next(iter(found)) if len(found) == 1 else None


def request_id_of(ticket: str) -> str:
    """The request id of a ticket `<requestId>.<vote>.<emission>`."""
    if TICKET_PATTERN.fullmatch(ticket) is None:
        raise RecordError(f"{ticket!r} is not a ticket")
    return ticket.split(".", 1)[0]


EXACT_REQUEST_KINDS: Final[dict[str, RequestKind]] = {
    KIND_GENERATE_TARGETED: RequestKind.MUTANTS,
    KIND_GENERATE_WITNESS_STRATEGY: RequestKind.MUTANTS,
    KIND_GENERATE_WITNESS: RequestKind.WITNESSES,
    KIND_JUDGE_SCOPE: RequestKind.SCOPE,
    KIND_JUDGE_VALIDITY: RequestKind.VALIDITY,
    KIND_JUDGE_EQUIVALENT: RequestKind.EQUIVALENCE,
    KIND_JUDGE_ALIGNMENT: RequestKind.ALIGNMENT,
    KIND_DECIDE_CLOSED: RequestKind.CLOSED_QUESTION,
}
"""Product request kinds the benchmark counts, each mapped to one benchmark request kind by its exact name; the
scan's kinds are not among them, since no benchmark arm runs a scan."""


def request_kind_of(module: Module, kind: str) -> RequestKind:
    """The benchmark's request kind of a product request: completeness questions are one kind whatever they ask."""
    if module is Module.COMPLETENESS:
        return RequestKind.COMPLETENESS
    if (exact := EXACT_REQUEST_KINDS.get(kind)) is not None:
        return exact
    raise RecordError(f"unknown request kind {kind!r}")
