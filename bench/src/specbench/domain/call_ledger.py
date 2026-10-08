"""The call ledger: model executions charged and reserved per night and harness.

A night is named by the local date on which its window opened. A reservation belongs to the night of its admission, and
so does the charge that settles it, whenever the call finishes. A charge may exceed its reservation (a wave of
subagents overshoots the kill by up to one wave); the ledger records the overshoot and never hides it.
"""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass, field
from datetime import date
from typing import Final

from specbench.domain.arms import Harness
from specbench.domain.canonical import JsonValue
from specbench.domain.jsonread import RecordError, enum_of, integer, mapping

LEGACY_HARNESS_KEYS: Final = {"claude": Harness.CLAUDE_CODE.value}
"""Keys an older ledger used for a harness; the pilot's ledger writes `claude`."""
CHARGED_KEY: Final = "charged"
RESERVED_KEY: Final = "reserved"


@dataclass(frozen=True)
class Account:
    """What one harness spent on one night: executions charged, and executions reserved by calls still in flight."""

    charged: int = 0
    reserved: Mapping[str, int] = field(default_factory=dict[str, int])

    @property
    def committed(self) -> int:
        return self.charged + sum(self.reserved.values())


@dataclass(frozen=True)
class Admission:
    """A call admitted on a night; its charge lands on that night whenever the call completes."""

    night: date
    call_id: str | None = None


class UnknownReservation(KeyError):
    """A settlement named a reservation that the ledger does not hold."""


@dataclass(frozen=True)
class CallLedger:
    """Executions per night and harness."""

    accounts: Mapping[str, Mapping[str, Account]]

    def account(self, night: date, harness: Harness) -> Account:
        return self.accounts.get(night.isoformat(), {}).get(harness.value, Account())

    def calls_on(self, night: date, harness: Harness) -> int:
        """Executions charged on a night."""
        return self.account(night, harness).charged

    def reserved_on(self, night: date, harness: Harness) -> int:
        return sum(self.account(night, harness).reserved.values())

    def committed_on(self, night: date, harness: Harness) -> int:
        """Executions charged plus executions reserved on a night: what no new admission may exceed the budget with."""
        return self.account(night, harness).committed

    def is_reserved(self, night: date, harness: Harness, call_id: str) -> bool:
        return call_id in self.account(night, harness).reserved

    def pending(self, harness: Harness) -> tuple[Admission, ...]:
        """Every reservation of a harness, oldest night first: calls that were admitted and never settled."""
        return tuple(
            Admission(date.fromisoformat(night), call_id)
            for night, per_harness in sorted(self.accounts.items())
            for call_id in sorted(per_harness.get(harness.value, Account()).reserved)
        )

    def with_call(self, night: date, harness: Harness) -> CallLedger:
        """One execution charged on a night."""
        account = self.account(night, harness)
        return self._with_account(night, harness, Account(account.charged + 1, account.reserved))

    def with_reservation(self, night: date, harness: Harness, call_id: str, size: int) -> CallLedger:
        """Reserve `size` executions for a call; a call that already holds a reservation on that night keeps it."""
        if size < 1:
            raise RecordError("a reservation holds at least one execution")
        account = self.account(night, harness)
        if call_id in account.reserved:
            return self
        return self._with_account(night, harness, Account(account.charged, {**account.reserved, call_id: size}))

    def settled(self, night: date, harness: Harness, call_id: str, actual: int) -> CallLedger:
        """Charge the executions a call spent and release its reservation, both on the night of its admission."""
        if actual < 0:
            raise RecordError("a call cannot spend a negative number of executions")
        account = self.account(night, harness)
        if call_id not in account.reserved:
            raise UnknownReservation(f"{call_id} holds no reservation on {night.isoformat()}")
        remaining = {key: size for key, size in account.reserved.items() if key != call_id}
        return self._with_account(night, harness, Account(account.charged + actual, remaining))

    def _with_account(self, night: date, harness: Harness, account: Account) -> CallLedger:
        updated = {key: dict(per_harness) for key, per_harness in self.accounts.items()}
        updated.setdefault(night.isoformat(), {})[harness.value] = account
        return CallLedger(updated)

    def to_record(self) -> dict[str, JsonValue]:
        return {
            night: {
                name: {CHARGED_KEY: account.charged, RESERVED_KEY: dict(sorted(account.reserved.items()))}
                for name, account in sorted(per_harness.items())
            }
            for night, per_harness in sorted(self.accounts.items())
        }

    @classmethod
    def from_record(cls, body: Mapping[str, object]) -> CallLedger:
        """Read a ledger. A count with no reservations (`{"claude": 250}`) is a charge; the legacy key is an alias."""
        accounts: dict[str, dict[str, Account]] = {}
        for night, per_harness in body.items():
            date.fromisoformat(night)
            nightly = accounts.setdefault(night, {})
            for key, value in mapping(per_harness, night).items():
                name = enum_of(Harness, LEGACY_HARNESS_KEYS.get(key, key), f"ledger.{night} harness").value
                read = _read_account(value, f"ledger.{night}.{key}")
                known = nightly.get(name, Account())
                nightly[name] = Account(known.charged + read.charged, {**known.reserved, **read.reserved})
        return cls(accounts)


def _read_account(value: object, where: str) -> Account:
    if isinstance(value, int) and not isinstance(value, bool):
        return Account(integer(value, where))
    body = mapping(value, where)
    reserved = mapping(body.get(RESERVED_KEY, {}), f"{where}.{RESERVED_KEY}")
    return Account(
        integer(body.get(CHARGED_KEY, 0), f"{where}.{CHARGED_KEY}"),
        {call_id: integer(size, f"{where}.{RESERVED_KEY}.{call_id}", 1) for call_id, size in reserved.items()},
    )


EMPTY_LEDGER: Final = CallLedger({})
