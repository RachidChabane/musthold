"""The pilot's switch rules SW2a and SW3, as the pilot's `decisions.json` records them (§12)."""

from __future__ import annotations

import json
from collections.abc import Mapping
from typing import Final

from calibration.domain.jsonread import as_list, as_mapping

SWITCH_NAMES: Final = ("SW2a", "SW3")
FIRED_KEY: Final = "fired"
NAME_KEY: Final = "name"
STATUS_KEY: Final = "status"
FIRED_WORD: Final = "fired"


def _fires(value: object) -> bool:
    """A switch has fired when its value is true, the word FIRED, or an object with `fired` true or a `status` that
    reads FIRED."""
    if value is True:
        return True
    if isinstance(value, str):
        return value.strip().lower() == FIRED_WORD
    record = as_mapping(value)
    if record is None:
        return False
    return _fires(record.get(FIRED_KEY)) or _fires(record.get(STATUS_KEY))


def _walk(value: object, names: tuple[str, ...], found: dict[str, bool]) -> None:
    record = as_mapping(value)
    items = as_list(value)
    if record is not None:
        named = record.get(NAME_KEY)
        if isinstance(named, str) and named in names:
            found[named] = found.get(named, False) or _fires(record)
        for key, child in record.items():
            if key in names:
                found[key] = found.get(key, False) or _fires(child)
            _walk(child, names, found)
    elif items is not None:
        for child in items:
            _walk(child, names, found)


def fired_switches(decisions_text: str | None, names: tuple[str, ...] = SWITCH_NAMES) -> Mapping[str, bool]:
    """Which of the switches the pilot's `results/decisions.json` records as fired; a switch the file does not
    mention has not fired, and a missing or unreadable file fires none."""
    found: dict[str, bool] = dict.fromkeys(names, False)
    if decisions_text is None:
        return found
    try:
        document = json.loads(decisions_text)
    except json.JSONDecodeError:
        return found
    _walk(document, names, found)
    return found
