from __future__ import annotations

import pytest

from specbench.domain.hunks import (
    MAX_HUNKS_PER_CRITERION,
    CodeHunk,
    changed_regions,
    hunk_from_record,
    hunk_problems,
    hunks_from_record,
    new_side_intervals,
)
from specbench.domain.jsonread import RecordError
from specbench.domain.languages import Language

EDIT = (
    "diff --git a/pkg/fee.py b/pkg/fee.py\n--- a/pkg/fee.py\n+++ b/pkg/fee.py\n"
    "@@ -1,2 +1,4 @@\n def fee(m):\n+    if m < 10:\n+        return 0\n     return 5\n"
    "@@ -20,3 +22,3 @@\n context\n-old\n+new\n context\n"
)
NEW_FILE = (
    "diff --git a/pkg/new.py b/pkg/new.py\nnew file mode 100644\n--- /dev/null\n+++ b/pkg/new.py\n"
    "@@ -0,0 +1,3 @@\n+a\n+b\n+c\n"
)
DELETED_FILE = "diff --git a/pkg/old.py b/pkg/old.py\ndeleted file mode 100644\n--- a/pkg/old.py\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-a\n-b\n"
PURE_RENAME = (
    "diff --git a/pkg/old.py b/pkg/moved.py\nsimilarity index 100%\nrename from pkg/old.py\nrename to pkg/moved.py\n"
)
RENAME_WITH_EDIT = (
    "diff --git a/pkg/old.py b/pkg/moved.py\nsimilarity index 80%\nrename from pkg/old.py\nrename to pkg/moved.py\n"
    "--- a/pkg/old.py\n+++ b/pkg/moved.py\n@@ -1,3 +1,4 @@\n a\n+b\n c\n d\n"
)
DELETION_ONLY = (
    "diff --git a/pkg/gone.py b/pkg/gone.py\n--- a/pkg/gone.py\n+++ b/pkg/gone.py\n@@ -5,3 +4,0 @@\n-x\n-y\n-z\n"
)
ADJACENT = (
    "diff --git a/pkg/adj.py b/pkg/adj.py\n--- a/pkg/adj.py\n+++ b/pkg/adj.py\n"
    "@@ -1,2 +1,2 @@\n-a\n+A\n b\n@@ -3,2 +3,2 @@\n c\n-d\n+D\n"
)
GAPPED = (
    "diff --git a/pkg/gap.py b/pkg/gap.py\n--- a/pkg/gap.py\n+++ b/pkg/gap.py\n"
    "@@ -1,2 +1,2 @@\n-a\n+A\n b\n@@ -10,2 +10,2 @@\n c\n-d\n+D\n"
)
LOOKALIKE_HEADERS = (
    "diff --git a/pkg/real.py b/pkg/real.py\n--- a/pkg/real.py\n+++ b/pkg/real.py\n"
    "@@ -1,1 +1,3 @@\n x\n+++ b/pkg/evil.py\n+@@ -1,1 +9,9 @@\n"
)
NO_NEWLINE = (
    "diff --git a/pkg/nl.py b/pkg/nl.py\n--- a/pkg/nl.py\n+++ b/pkg/nl.py\n"
    "@@ -1,1 +1,1 @@\n-a\n\\ No newline at end of file\n+b\n\\ No newline at end of file\n"
)
TEST_FILES = (
    "diff --git a/tests/test_fee.py b/tests/test_fee.py\n--- a/tests/test_fee.py\n+++ b/tests/test_fee.py\n"
    "@@ -1,1 +1,2 @@\n a\n+b\n"
)
QUOTED = 'diff --git "a/pkg/é.py" "b/pkg/é.py"\n--- "a/pkg/\\303\\251.py"\n+++ "b/pkg/\\303\\251.py"\n@@ -1,1 +1,2 @@\n a\n+b\n'
TAB_TERMINATED_PATH = (
    "diff --git a/my pkg/a.py b/my pkg/a.py\n--- a/my pkg/a.py\t\n+++ b/my pkg/a.py\t\n@@ -1,1 +1,2 @@\n a\n+b\n"
)


def regions(diff: str, language: Language = Language.PYTHON):
    return changed_regions(language, diff)


def problems(diff: str, *hunks: CodeHunk, language: Language = Language.PYTHON) -> list[str]:
    return hunk_problems("criterion 1", hunks, regions(diff, language))


def test_the_new_side_lines_of_a_hunk_include_its_context_and_follow_the_second_range() -> None:
    assert new_side_intervals(EDIT) == {"pkg/fee.py": [(1, 4), (22, 24)]}


def test_a_hunk_inside_a_diff_hunk_is_accepted_whatever_the_marks_of_its_lines() -> None:
    for start, end in ((1, 4), (2, 3), (3, 3), (1, 1), (22, 24), (23, 23)):
        assert problems(EDIT, CodeHunk("pkg/fee.py", start, end)) == []


def test_hunks_at_the_boundaries_of_a_file_are_accepted_and_one_line_beyond_is_not() -> None:
    assert (
        problems(NEW_FILE, CodeHunk("pkg/new.py", 1, 1), CodeHunk("pkg/new.py", 3, 3), CodeHunk("pkg/new.py", 1, 3))
        == []
    )
    assert problems(NEW_FILE, CodeHunk("pkg/new.py", 1, 4)) != []
    assert problems(NEW_FILE, CodeHunk("pkg/new.py", 4, 4)) != []
    assert problems(EDIT, CodeHunk("pkg/fee.py", 24, 25)) != []
    assert problems(EDIT, CodeHunk("pkg/fee.py", 5, 5)) != []


def test_a_range_that_spans_two_separate_diff_hunks_is_rejected() -> None:
    assert problems(GAPPED, CodeHunk("pkg/gap.py", 1, 10)) != []
    assert problems(GAPPED, CodeHunk("pkg/gap.py", 2, 3)) != []


def test_diff_hunks_that_touch_are_one_region() -> None:
    assert regions(ADJACENT).intervals == {"pkg/adj.py": ((1, 4),)}
    assert problems(ADJACENT, CodeHunk("pkg/adj.py", 2, 3)) == []


def test_a_range_that_ends_before_it_starts_is_rejected() -> None:
    assert any("not a line range" in p for p in problems(EDIT, CodeHunk("pkg/fee.py", 3, 2)))


def test_a_hunk_in_a_test_file_is_rejected_because_the_region_is_not_a_non_test_file() -> None:
    assert regions(TEST_FILES).paths == frozenset()
    assert any("not a non-test file" in p for p in problems(TEST_FILES, CodeHunk("tests/test_fee.py", 1, 2)))
    assert any("not a non-test file" in p for p in problems(EDIT + TEST_FILES, CodeHunk("tests/test_fee.py", 1, 2)))


def test_a_go_test_file_and_a_go_test_data_file_are_test_files() -> None:
    diff = EDIT.replace("pkg/fee.py", "pkg/fee_test.go") + NEW_FILE.replace("pkg/new.py", "pkg/testdata/in.txt")
    assert regions(diff, Language.GO).paths == frozenset()
    assert regions(EDIT.replace("fee.py", "fee.go"), Language.GO).paths == {"pkg/fee.go"}


def test_a_hunk_of_an_unknown_file_is_rejected() -> None:
    assert any("not a non-test file" in p for p in problems(EDIT, CodeHunk("pkg/other.py", 1, 2)))


def test_a_deleted_file_cannot_be_cited() -> None:
    assert regions(DELETED_FILE).paths == frozenset()
    assert problems(DELETED_FILE, CodeHunk("pkg/old.py", 1, 1)) != []


def test_a_pure_rename_has_nothing_to_cite_under_either_name() -> None:
    assert regions(PURE_RENAME).paths == frozenset()
    assert problems(PURE_RENAME, CodeHunk("pkg/moved.py", 1, 1)) != []
    assert problems(PURE_RENAME, CodeHunk("pkg/old.py", 1, 1)) != []


def test_a_renamed_file_is_cited_by_its_new_path_only() -> None:
    assert problems(RENAME_WITH_EDIT, CodeHunk("pkg/moved.py", 2, 2)) == []
    assert problems(RENAME_WITH_EDIT, CodeHunk("pkg/old.py", 2, 2)) != []


def test_a_rename_into_a_test_path_leaves_no_citable_file() -> None:
    diff = RENAME_WITH_EDIT.replace("pkg/moved.py", "tests/test_moved.py")
    assert regions(diff).paths == frozenset()


def test_a_deletion_only_hunk_keeps_its_context_lines_citable_but_has_no_line_of_its_own() -> None:
    assert new_side_intervals(DELETION_ONLY) == {}
    with_context = DELETION_ONLY.replace(
        "@@ -5,3 +4,0 @@\n-x\n-y\n-z\n", "@@ -3,7 +3,4 @@\n a\n-x\n-y\n-z\n b\n c\n d\n"
    )
    assert new_side_intervals(with_context) == {"pkg/gone.py": [(3, 6)]}
    assert problems(with_context, CodeHunk("pkg/gone.py", 3, 6)) == []


def test_lines_of_a_hunk_body_that_look_like_headers_are_content() -> None:
    assert regions(LOOKALIKE_HEADERS).intervals == {"pkg/real.py": ((1, 3),)}


def test_the_no_newline_marker_is_not_a_line() -> None:
    assert new_side_intervals(NO_NEWLINE) == {"pkg/nl.py": [(1, 1)]}


def test_a_path_git_quotes_is_not_citable_and_a_tab_ends_a_path_with_spaces() -> None:
    assert regions(QUOTED).paths == frozenset()
    assert regions(TAB_TERMINATED_PATH).paths == {"my pkg/a.py"}


def test_a_truncated_diff_still_yields_the_hunks_that_were_cut_short() -> None:
    assert regions(EDIT[: EDIT.index("@@ -20")]).intervals == {"pkg/fee.py": ((1, 4),)}


def test_too_many_hunks_for_one_criterion_are_refused() -> None:
    hunks = [CodeHunk("pkg/fee.py", 1, 1)] * (MAX_HUNKS_PER_CRITERION + 1)
    assert any("at most" in p for p in problems(EDIT, *hunks))


def test_every_problem_names_the_criterion_and_the_hunk() -> None:
    (problem,) = problems(EDIT, CodeHunk("pkg/fee.py", 5, 6))
    assert problem.startswith("criterion 1: hunk pkg/fee.py:5-6")


def test_a_hunk_record_round_trips_and_refuses_a_missing_or_non_positive_field() -> None:
    hunk = CodeHunk("pkg/fee.py", 2, 3)
    assert hunk_from_record(hunk.to_record()) == hunk
    assert hunks_from_record([hunk.to_record()]) == (hunk,)
    for broken in (
        {"path": "a", "startLine": 0, "endLine": 1},
        {"path": "a", "startLine": 1},
        {"startLine": 1, "endLine": 1},
    ):
        with pytest.raises(RecordError):
            hunk_from_record(broken)
    with pytest.raises(RecordError):
        hunks_from_record({"path": "a"})
