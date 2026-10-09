"""Build-time rendering: highlighting, diffs, charts, templates without numbers, and the page shell."""

from __future__ import annotations

from html.parser import HTMLParser

import pytest

from specbench.domain.languages import Language
from specbench.site.charts import ChartRow, interval_chart
from specbench.site.figures import Comparison
from specbench.site.highlight import DiffLine, Token, classify_lines, diff_stats, render_diff, tokens, unified_diff
from specbench.site.html import Markup, element, escape
from specbench.site.template_files import PageTemplate, fill, template_text
from support.site import make_figure

ATTRIBUTES_ALLOWED_DIGITS = frozenset({"meta"})


class _Digits(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.found: list[str] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag in ATTRIBUTES_ALLOWED_DIGITS:
            return
        self.found += [f"<{tag} {name}={value}>" for name, value in attrs if value and any(c.isdigit() for c in value)]

    def handle_data(self, data: str) -> None:
        if any(c.isdigit() for c in data):
            self.found.append(data.strip())


@pytest.mark.parametrize("template", list(PageTemplate), ids=lambda t: t.value)
def test_a_template_holds_no_number_every_figure_comes_through_a_slot(template: PageTemplate) -> None:
    scan = _Digits()
    scan.feed(template_text(template))
    assert scan.found == []


def test_a_slot_left_unfilled_is_an_error() -> None:
    with pytest.raises(KeyError):
        fill(PageTemplate.REPRODUCE, {})


def test_text_is_escaped_and_markup_is_kept() -> None:
    assert escape("<b>") == "&lt;b&gt;"
    assert escape(Markup("<b>")) == "<b>"
    assert element("a", {"href": 'x"y'}, "<t>") == '<a href="x&quot;y">&lt;t&gt;</a>'


def test_python_tokens() -> None:
    kinds = {value: kind for kind, value in tokens(Language.PYTHON, "def f(x):  # note") if kind is not Token.TEXT}
    assert kinds == {"def": Token.KEYWORD, "# note": Token.COMMENT}
    strings = [value for kind, value in tokens(Language.PYTHON, 'return "a # b", 4.5') if kind is Token.STRING]
    assert strings == ['"a # b"']


def test_go_tokens() -> None:
    found = [
        (kind, value)
        for kind, value in tokens(Language.GO, "func F() int { return 50 } // x")
        if kind is not Token.TEXT
    ]
    assert found == [(Token.KEYWORD, "func"), (Token.KEYWORD, "return"), (Token.NUMBER, "50"), (Token.COMMENT, "// x")]


def test_a_unified_diff_has_no_timestamps_and_marks_lines_by_sign() -> None:
    diff = unified_diff("a = 1\nb = 2\n", "a = 1\nb = 3\n", "m.py")
    assert diff.splitlines()[:2] == ["--- a/m.py", "+++ b/m.py"]
    assert diff_stats(diff).added == 1 and diff_stats(diff).removed == 1
    html = render_diff(Language.PYTHON, diff, "Mutant")
    assert 'class="line diff-add">+' in html and 'class="line diff-del">-' in html


def test_dashes_inside_a_hunk_are_removed_lines_not_file_headers() -> None:
    kinds = [kind for kind, _ in classify_lines("--- a/x\n+++ b/x\n@@ -1 +1 @@\n---- old\n++++ new\n")]
    assert kinds == [DiffLine.FILE, DiffLine.FILE, DiffLine.HUNK, DiffLine.REMOVED, DiffLine.ADDED]


def test_a_chart_draws_an_error_bar_per_estimable_row_and_says_when_one_is_not() -> None:
    rows = [
        ChartRow("T", make_figure(0.83, 0.5, 1.0)),
        ChartRow("ALL", make_figure(None, None, None, n=0)),
    ]
    svg = interval_chart("Residual defects", rows, True, "c")
    assert svg.count('class="chart-bar"') == 1
    assert "not estimable" in svg
    assert 'role="img"' in svg and "<desc" in svg


def test_an_unbounded_chart_spans_zero_and_its_data() -> None:
    rows = [ChartRow("ALL", make_figure(0.6, -1.5, 1.0, comparison=Comparison.REDUCTION))]
    svg = interval_chart("X", rows, False, "x")
    assert 'class="chart-zero"' in svg
