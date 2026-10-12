import itertools
import random
from datetime import UTC, datetime, timedelta

from hypothesis import given
from hypothesis import strategies as st

from demos1.domain.candidates import Candidate
from demos1.domain.filters import Exclusions
from demos1.domain.report_lint import lint
from demos1.domain.toolchains import go_version_key
from demos1.domain.window import Interval

BASE = datetime(2026, 7, 1, tzinfo=UTC)
seconds = st.integers(min_value=0, max_value=92 * 86400 - 1)


@given(seconds, st.integers(min_value=1, max_value=86400 * 3))
def test_halves_partition_an_interval_without_gap_or_overlap(start: int, length: int):
    interval = Interval(BASE + timedelta(seconds=start), BASE + timedelta(seconds=start + length))
    first, second = interval.halves()
    assert first.start == interval.start
    assert second.end == interval.end
    assert second.start - first.end == timedelta(seconds=1)
    assert first.seconds() + second.seconds() == interval.seconds()
    assert abs(first.seconds() - second.seconds()) <= 1


def go_version(major: int, minor: int, tail: str) -> str:
    return f"{major}.{minor}{tail}"


go_versions = st.builds(
    go_version,
    st.integers(1, 1),
    st.integers(18, 30),
    st.one_of(
        st.just(""),
        st.integers(0, 12).map(lambda patch: f".{patch}"),
        st.integers(1, 3).map(lambda number: f"rc{number}"),
        st.integers(1, 3).map(lambda number: f"beta{number}"),
    ),
)


@given(go_versions, go_versions, go_versions)
def test_go_version_order_is_total_and_transitive(a: str, b: str, c: str):
    ka, kb, kc = go_version_key(a), go_version_key(b), go_version_key(c)
    assert (ka <= kb) or (kb <= ka)
    if ka <= kb <= kc:
        assert ka <= kc


@given(st.integers(18, 30), st.integers(0, 12), st.integers(1, 3))
def test_a_language_version_precedes_its_prereleases_and_releases(minor: int, patch: int, rc: int):
    assert go_version_key(f"1.{minor}") < go_version_key(f"1.{minor}rc{rc}") < go_version_key(f"1.{minor}.{patch}")


names = st.from_regex(r"[A-Za-z0-9][A-Za-z0-9-]{0,8}/[A-Za-z0-9._-]{1,10}", fullmatch=True)


@given(names, st.sampled_from([str.lower, str.upper, str.swapcase]))
def test_exclusions_ignore_case(name: str, change: object):
    exclusions = Exclusions(frozenset({name.lower()}))
    assert exclusions.excludes(change(name))  # type: ignore[operator]


@given(st.lists(st.tuples(seconds, names, st.integers(1, 9999)), min_size=1, max_size=20), st.randoms())
def test_processing_order_is_a_total_order_independent_of_input_order(
    rows: list[tuple[int, str, int]], shuffler: random.Random
):
    candidates = [
        Candidate(f"u{index}", name, number, BASE + timedelta(seconds=offset), frozenset())
        for index, (offset, name, number) in enumerate(rows)
    ]
    forward = [candidate.order_key() for candidate in sorted(candidates, key=Candidate.order_key)]
    shuffled = list(candidates)
    shuffler.shuffle(shuffled)
    backward = [candidate.order_key() for candidate in sorted(shuffled, key=Candidate.order_key)]
    assert forward == backward
    assert all(left[0] <= right[0] for left, right in itertools.pairwise(forward))


fraction = st.tuples(st.integers(0, 500), st.integers(0, 500)).map(lambda pair: f"{pair[0]}/{pair[1]}")


@given(st.lists(fraction, min_size=1, max_size=8))
def test_listed_fractions_never_fail_the_lint_and_others_always_do(fractions: list[str]):
    markdown = "\n".join(f"- {value} sampled tests" for value in fractions)
    assert lint(markdown, fractions) == []
    flagged = lint(markdown, fractions[1:])
    if fractions[0] not in fractions[1:]:
        assert any(violation.text == fractions[0] for violation in flagged)


@given(st.text(alphabet=st.characters(blacklist_characters="`\n\r"), max_size=40))
def test_inline_code_is_never_linted(content: str):
    assert lint(f"Path `{content}` here.", []) == [] or "`" in content
