from __future__ import annotations

from dataclasses import replace

from hypothesis import given, settings
from hypothesis import strategies as st

from specbench.domain.canonical import canonical_json, sha256_hex
from specbench.domain.gaps import GAP_FILE_NAME, draw_gap_criteria, gaps_record
from specbench.domain.hunks import CodeHunk, changed_regions, hunk_problems
from specbench.domain.languages import Language, is_test_path
from specbench.domain.sealing import SealedBundle, sealed_leaks, versioned_bundle
from specbench.domain.spec import Dimension, InterfaceEntry, SpecCriterion, SpecDraft, render_spec
from specbench.domain.task import CriterionStatus
from specbench.domain.workspace import WorkspaceLeak, build_workspace
from support.builders import TASK_ID, oracle_files

PATHS = ("pkg/a.py", "pkg/b.py", "tests/test_a.py", "pkg/c_test.py", "conftest.py")
MAX_LINE = 30


@st.composite
def diff_hunks(draw: st.DrawFn) -> list[tuple[int, int, int, str]]:
    """Non-overlapping hunks of one file as (start, new-side count, deleted lines, marks of the new-side lines)."""
    hunks: list[tuple[int, int, int, str]] = []
    cursor = 1
    for _ in range(draw(st.integers(0, 3))):
        start = cursor + draw(st.integers(0, 4))
        count = draw(st.integers(0, 6))
        marks = "".join(draw(st.lists(st.sampled_from(" +"), min_size=count, max_size=count)))
        hunks.append((start, count, draw(st.integers(0, 3)), marks))
        cursor = start + count
    return hunks


def render_diff(files: dict[str, list[tuple[int, int, int, str]]]) -> str:
    lines: list[str] = []
    for path, hunks in files.items():
        lines += [f"diff --git a/{path} b/{path}", f"--- a/{path}", f"+++ b/{path}"]
        for start, count, deleted, marks in hunks:
            old_count = deleted + marks.count(" ")
            lines.append(f"@@ -{start},{old_count} +{start},{count} @@")
            lines += ["-removed"] * deleted
            lines += [f"{mark}line" for mark in marks]
    return "\n".join(lines) + "\n"


def covered_lines(files: dict[str, list[tuple[int, int, int, str]]]) -> dict[str, set[int]]:
    """The reference semantics, by explicit line sets: every new-side line of a diff hunk of a non-test file."""
    return {
        path: {line for start, count, _, _ in hunks for line in range(start, start + count)}
        for path, hunks in files.items()
        if not is_test_path(Language.PYTHON, path)
    }


@settings(max_examples=300)
@given(
    st.dictionaries(st.sampled_from(PATHS), diff_hunks(), max_size=4),
    st.lists(
        st.builds(
            CodeHunk,
            st.sampled_from((*PATHS, "pkg/missing.py")),
            st.integers(0, MAX_LINE),
            st.integers(0, MAX_LINE),
        ),
        max_size=6,
    ),
)
def test_a_hunk_is_valid_exactly_when_all_its_lines_are_new_side_lines_of_a_non_test_file_of_the_diff(
    files: dict[str, list[tuple[int, int, int, str]]], hunks: list[CodeHunk]
) -> None:
    regions = changed_regions(Language.PYTHON, render_diff(files))
    covered = covered_lines(files)
    for hunk in hunks:
        inside = 1 <= hunk.start_line <= hunk.end_line and set(
            range(hunk.start_line, hunk.end_line + 1)
        ) <= covered.get(hunk.path, set())
        assert (hunk_problems("criterion 1", [hunk], regions) == []) is inside


@settings(max_examples=150)
@given(st.dictionaries(st.sampled_from(PATHS), diff_hunks(), max_size=4))
def test_no_valid_hunk_lies_in_a_test_file(files: dict[str, list[tuple[int, int, int, str]]]) -> None:
    regions = changed_regions(Language.PYTHON, render_diff(files))
    assert all(not is_test_path(Language.PYTHON, path) for path in regions.paths)


safe_segment = st.from_regex(r"[a-z][a-z0-9_]{0,6}", fullmatch=True)
safe_paths: st.SearchStrategy[str] = st.lists(safe_segment, min_size=1, max_size=3).map(lambda parts: "/".join(parts))  # noqa: PLW0108
file_trees = st.dictionaries(safe_paths, st.binary(max_size=40), max_size=6)


def gap_bundle(seed: int, spec_json: bytes) -> SealedBundle:
    record = gaps_record(seed, TASK_ID, (1, 2))
    return versioned_bundle(
        {**oracle_files(), "construction/spec.json": spec_json, GAP_FILE_NAME: canonical_json(record).encode()}
    )


@settings(max_examples=150)
@given(file_trees, file_trees, st.integers(0, 2**31), st.binary(min_size=1, max_size=40), st.integers(0, 3))
def test_a_workspace_never_holds_the_spec_gap_data_whatever_the_base_tree_copies_of_it(
    base: dict[str, bytes], init: dict[str, bytes], seed: int, spec_json: bytes, disguise: int
) -> None:
    bundle = gap_bundle(seed, spec_json)
    gaps, record = bundle.files[GAP_FILE_NAME], bundle.files["construction/spec.json"]
    hostile = dict(base)
    if disguise == 0:
        hostile["notes/gaps.txt"] = gaps
    elif disguise == 1:
        hostile[GAP_FILE_NAME] = b"anything"
    elif disguise == 2:
        hostile["notes/record.txt"] = b"see " + sha256_hex(record).encode()
    else:
        hostile["construction/spec.json"] = record
    try:
        workspace = build_workspace(hostile, b"spec text", init, bundle)
    except WorkspaceLeak:
        return
    assert GAP_FILE_NAME not in workspace.files and "construction/spec.json" not in workspace.files
    assert gaps not in workspace.files.values() and record not in workspace.files.values()
    digests = (sha256_hex(gaps).encode(), sha256_hex(record).encode())
    assert not any(d in content for d in digests for content in workspace.files.values())
    assert sealed_leaks(workspace.files.items(), {GAP_FILE_NAME: gaps, "construction/spec.json": record}) == []


def draft_with(dimensions: list[Dimension], hunks: list[tuple[CodeHunk, ...]]) -> SpecDraft:
    criteria = tuple(
        SpecCriterion(
            number,
            f"WHEN case {number} THE SYSTEM SHALL answer {number}",
            (f"op(a={number}) => {number}",),
            CriterionStatus.NEW,
            dimension,
            found,
        )
        for number, (dimension, found) in enumerate(zip(dimensions, hunks, strict=True), start=1)
    )
    return SpecDraft("title", "overview", (InterfaceEntry("pkg.op", "op(a)", ""),), criteria)


DIMENSIONS = st.lists(st.sampled_from(list(Dimension)), min_size=2, max_size=8)
HUNK_SETS = st.lists(
    st.builds(CodeHunk, st.sampled_from(PATHS), st.integers(1, 9), st.integers(1, 9)).map(
        lambda h: replace(h, end_line=max(h.start_line, h.end_line))
    ),
    max_size=3,
).map(tuple)


@given(DIMENSIONS, st.data())
def test_the_rendered_spec_does_not_depend_on_dimensions_or_hunks(
    dimensions: list[Dimension], data: st.DataObject
) -> None:
    first = data.draw(st.lists(HUNK_SETS, min_size=len(dimensions), max_size=len(dimensions)))
    second = data.draw(st.lists(HUNK_SETS, min_size=len(dimensions), max_size=len(dimensions)))
    other = data.draw(st.lists(st.sampled_from(list(Dimension)), min_size=len(dimensions), max_size=len(dimensions)))
    assert render_spec(TASK_ID, draft_with(dimensions, first)) == render_spec(TASK_ID, draft_with(other, second))


@given(DIMENSIONS, st.integers(0, 2**31))
def test_the_draw_takes_two_distinct_eligible_criteria_and_is_reproducible(
    dimensions: list[Dimension], seed: int
) -> None:
    draft = draft_with(dimensions, [(CodeHunk("pkg/a.py", 1, 1),)] * len(dimensions))
    drawn = draw_gap_criteria(seed, TASK_ID, draft)
    assert len(drawn) == 2 and drawn[0] < drawn[1] <= len(dimensions)
    assert drawn == draw_gap_criteria(seed, TASK_ID, draft)
