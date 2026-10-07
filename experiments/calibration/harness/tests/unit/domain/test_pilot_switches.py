from __future__ import annotations

import json

import pytest

from calibration.domain.pilot_switches import SWITCH_NAMES, fired_switches

NEITHER = {"SW2a": False, "SW3": False}


def _fired(document: object) -> dict[str, bool]:
    return dict(fired_switches(json.dumps(document)))


class TestMissingInput:
    def test_the_switches_are_sw2a_and_sw3(self) -> None:
        assert SWITCH_NAMES == ("SW2a", "SW3")

    def test_a_missing_file_fires_nothing(self) -> None:
        assert dict(fired_switches(None)) == NEITHER

    def test_an_unreadable_file_fires_nothing(self) -> None:
        assert dict(fired_switches("{not json")) == NEITHER

    def test_a_switch_the_file_does_not_mention_has_not_fired(self) -> None:
        assert _fired({"SW9": True, "other": {}}) == NEITHER


class TestFiringForms:
    @pytest.mark.parametrize(
        "value",
        [True, "FIRED", "fired", " Fired ", {"fired": True}, {"status": "FIRED"}, {"fired": "fired"}],
    )
    def test_a_keyed_switch_fires(self, value: object) -> None:
        assert _fired({"SW2a": value}) == {"SW2a": True, "SW3": False}

    @pytest.mark.parametrize("value", [False, "NOT_FIRED", "no", 1, None, {"fired": False}, {"status": "HELD"}, {}])
    def test_a_keyed_switch_does_not_fire(self, value: object) -> None:
        assert _fired({"SW2a": value}) == NEITHER

    def test_a_named_record_fires_by_its_fired_member(self) -> None:
        assert _fired({"switches": [{"name": "SW3", "fired": True}]}) == {"SW2a": False, "SW3": True}

    def test_a_named_record_fires_by_its_status(self) -> None:
        assert _fired([{"name": "SW3", "status": "FIRED"}]) == {"SW2a": False, "SW3": True}

    def test_a_named_record_that_did_not_fire_leaves_the_switch_unfired(self) -> None:
        assert _fired([{"name": "SW3", "fired": False}]) == NEITHER


class TestNesting:
    def test_a_deeply_nested_switch_is_found(self) -> None:
        assert _fired({"a": {"b": [{"c": {"SW3": True}}]}}) == {"SW2a": False, "SW3": True}

    def test_a_switch_that_fires_anywhere_fires(self) -> None:
        document = {"first": {"SW2a": False}, "second": [{"name": "SW2a", "fired": True}]}
        assert _fired(document) == {"SW2a": True, "SW3": False}

    def test_a_later_unfired_mention_does_not_clear_a_fired_one(self) -> None:
        document = [{"name": "SW3", "fired": True}, {"SW3": False}]
        assert _fired(document) == {"SW2a": False, "SW3": True}

    def test_both_switches_can_fire(self) -> None:
        assert _fired({"SW2a": True, "SW3": {"fired": True}}) == {"SW2a": True, "SW3": True}

    def test_the_names_asked_for_are_the_ones_reported(self) -> None:
        assert dict(fired_switches('{"SW9": true}', ("SW9",))) == {"SW9": True}
