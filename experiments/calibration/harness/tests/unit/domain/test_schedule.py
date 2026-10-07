from __future__ import annotations

import hashlib
from random import Random

from hypothesis import given
from hypothesis import strategies as st

from calibration.domain.constants import HASH_SEPARATOR, PASS2_HEX_DIGITS, PASS2_LABEL, PASS2_MODULUS, SEED_TEXT
from calibration.domain.schedule import (
    Pass,
    Schedule,
    alignment_request_id,
    in_pass2,
    is_alignment_request,
    mutant_request_id,
    order_key,
    pass1_order,
    request_slug,
)

NODEIDS = [f"tests/test_{name}.py::test_case" for name in "abcdefgh"]
MUTANT_IDS = [f"mut{number:03d}" for number in range(60)]
REQUEST_ID_SETS = st.sets(st.text(alphabet="abcdefghij0123456789|:_", min_size=1, max_size=12), max_size=40)


def _sha256(text: str) -> str:
    return hashlib.sha256(text.encode()).hexdigest()


def _law_pass2_member(request_id: str) -> bool:
    digest = _sha256(SEED_TEXT + HASH_SEPARATOR + PASS2_LABEL + HASH_SEPARATOR + request_id)
    return int(digest[:PASS2_HEX_DIGITS], 16) % PASS2_MODULUS == 0


def _all_requests() -> list[str]:
    return [alignment_request_id(nodeid) for nodeid in NODEIDS] + [mutant_request_id(m) for m in MUTANT_IDS]


class TestRequestIds:
    def test_an_alignment_id_slugs_the_node_id(self) -> None:
        assert alignment_request_id("tests/x.py::Cls::t[1]") == "A|tests_x.py__Cls__t_1_"

    def test_a_mutant_id_is_prefixed(self) -> None:
        assert mutant_request_id("abc") == "M|abc"

    def test_the_kind_of_a_request_is_read_from_its_prefix(self) -> None:
        assert is_alignment_request(alignment_request_id("t.py::a"))
        assert not is_alignment_request(mutant_request_id("abc"))

    def test_the_slug_of_a_request_replaces_the_separator(self) -> None:
        assert request_slug("A|tests_x.py__t") == "A_tests_x.py__t"


class TestPassOneOrder:
    def test_it_is_the_ascending_order_of_the_seeded_sha256(self) -> None:
        ids = _all_requests()
        expected = sorted(ids, key=lambda request_id: _sha256("20261007|" + request_id))
        assert pass1_order(ids) == expected

    def test_the_order_key_is_the_seeded_digest(self) -> None:
        assert order_key("M|x") == _sha256("20261007|M|x")

    def test_duplicates_are_sent_once(self) -> None:
        assert pass1_order(["M|a", "M|a"]) == ["M|a"]

    @given(REQUEST_ID_SETS, st.randoms(use_true_random=False))
    def test_the_order_is_a_permutation_independent_of_the_input_order(
        self, request_ids: set[str], shuffler: Random
    ) -> None:
        listed = sorted(request_ids)
        shuffled = list(listed)
        shuffler.shuffle(shuffled)
        assert pass1_order(shuffled) == pass1_order(listed)
        assert sorted(pass1_order(listed)) == listed


class TestPassTwoMembership:
    def test_every_alignment_request_is_in_pass_two(self) -> None:
        assert all(in_pass2(alignment_request_id(nodeid)) for nodeid in NODEIDS)

    def test_a_mutant_is_in_pass_two_when_its_pass_two_hash_is_divisible_by_four(self) -> None:
        for mutant_id in MUTANT_IDS:
            request_id = mutant_request_id(mutant_id)
            assert in_pass2(request_id) is _law_pass2_member(request_id)

    def test_the_mutant_fixture_has_both_members_and_non_members(self) -> None:
        members = {_law_pass2_member(mutant_request_id(m)) for m in MUTANT_IDS}
        assert members == {True, False}


class TestSchedule:
    def test_pass_one_holds_every_request_in_pass_one_order(self) -> None:
        ids = _all_requests()
        assert Schedule.of(ids).pass1 == tuple(pass1_order(ids))

    def test_pass_two_is_the_reverse_of_pass_one_restricted_to_its_members(self) -> None:
        ids = _all_requests()
        schedule = Schedule.of(ids)
        expected = tuple(r for r in reversed(pass1_order(ids)) if is_alignment_request(r) or _law_pass2_member(r))
        assert schedule.pass2 == expected

    def test_pass_two_holds_all_alignment_requests(self) -> None:
        schedule = Schedule.of(_all_requests())
        assert {r for r in schedule.pass2 if is_alignment_request(r)} == {alignment_request_id(n) for n in NODEIDS}

    def test_ids_of_selects_the_pass(self) -> None:
        schedule = Schedule.of(_all_requests())
        assert schedule.ids_of(Pass.ONE) == schedule.pass1
        assert schedule.ids_of(Pass.TWO) == schedule.pass2

    @given(REQUEST_ID_SETS, st.randoms(use_true_random=False))
    def test_the_schedule_is_independent_of_the_input_order(self, request_ids: set[str], shuffler: Random) -> None:
        listed = sorted(request_ids)
        shuffled = list(listed)
        shuffler.shuffle(shuffled)
        assert Schedule.of(shuffled) == Schedule.of(listed)

    @given(REQUEST_ID_SETS)
    def test_pass_two_is_a_sub_sequence_of_the_reversed_pass_one(self, request_ids: set[str]) -> None:
        schedule = Schedule.of(request_ids)
        assert set(schedule.pass2) <= set(schedule.pass1)
        positions = [schedule.pass1.index(request_id) for request_id in schedule.pass2]
        assert positions == sorted(positions, reverse=True)
