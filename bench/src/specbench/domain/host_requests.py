"""Reading the requests a gate host answers: tickets in executor prompts, and the kind of request a ticket belongs to."""

from __future__ import annotations

import re
from typing import Final

from specbench.domain.arms import Module
from specbench.domain.jsonread import RecordError
from specbench.domain.sessions import RequestKind

TICKET_PATTERN: Final = re.compile(r"[0-9a-f]{64}\.\d+\.\d+")
KIND_GENERATE_PREFIX: Final = "generate."
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


def request_kind_of(module: Module, kind: str) -> RequestKind:
    """The benchmark's request kind of a product request: completeness questions are one kind whatever they ask."""
    if module is Module.COMPLETENESS:
        return RequestKind.COMPLETENESS
    if kind.startswith(KIND_GENERATE_PREFIX):
        return RequestKind.MUTANTS
    if kind == KIND_JUDGE_VALIDITY:
        return RequestKind.VALIDITY
    if kind == KIND_JUDGE_EQUIVALENT:
        return RequestKind.EQUIVALENCE
    if kind == KIND_JUDGE_ALIGNMENT:
        return RequestKind.ALIGNMENT
    if kind == KIND_DECIDE_CLOSED:
        return RequestKind.CLOSED_QUESTION
    raise RecordError(f"unknown request kind {kind!r}")
