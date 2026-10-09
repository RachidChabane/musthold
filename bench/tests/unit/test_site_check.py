"""The site check: each rule fails on a page that breaks it, and passes on the fixture build."""

from __future__ import annotations

import json
from collections.abc import Callable, Mapping

import pytest

from specbench.site.check import check_site
from specbench.site.model import BuildMode
from support.site import REPOSITORY_ROOT, fixture_site

RESULTS = json.loads((REPOSITORY_ROOT / "bench/tests/golden/results.json").read_text(encoding="utf-8"))
SITE = fixture_site()


def edited(path: str, change: Callable[[str], str]) -> Mapping[str, str]:
    return {**SITE, path: change(SITE[path])}


def test_the_fixture_build_passes_its_check() -> None:
    assert check_site(SITE, BuildMode.SYNTHETIC, RESULTS) == []


def test_every_page_of_a_fixture_build_carries_the_synthetic_badge() -> None:
    pages = [content for path, content in SITE.items() if path.endswith(".html")]
    assert pages
    assert all('data-synthetic="true"' in page and "Synthetic data" in page for page in pages)
    assert all('data-build="synthetic"' in page for page in pages)


def test_a_fixture_build_is_never_accepted_as_a_sealed_run() -> None:
    findings = check_site(SITE, BuildMode.SEALED_RUN, RESULTS)
    assert any("carries the synthetic badge" in f for f in findings)
    assert any("declares build synthetic" in f for f in findings)


def swap(old: str, new: str) -> Callable[[str], str]:
    """An edit that replaces the first `old` of a page by `new`."""

    def change(page: str) -> str:
        return page.replace(old, new, 1)

    return change


@pytest.mark.parametrize(
    ("change", "fragment"),
    [
        (swap('data-synthetic="true"', 'data-other="true"'), "carries no synthetic badge"),
        (swap('href="arms.html"', 'href="missing.html"'), "resolves to nothing"),
        (swap("</main>", "<div></main>"), "closes div"),
        (
            swap("</head>", '<link rel="stylesheet" href="https://cdn.example/x.css"></head>'),
            "loads from another origin",
        ),
        (swap("</main>", "<script>track()</script></main>"), "a page runs a script"),
        (swap("</main>", "<p>Fifty percent: 50 %</p></main>"), "percentage outside a figure"),
        (swap("</main>", "<p>The gate proves it.</p></main>"), "the word 'proves'"),
    ],
)
def test_each_rule_fails_on_a_page_that_breaks_it(change: Callable[[str], str], fragment: str) -> None:
    findings = check_site(edited("index.html", change), BuildMode.SYNTHETIC, RESULTS)
    assert any(fragment in f for f in findings), findings


def test_a_figure_that_differs_from_the_results_fails() -> None:
    page = SITE["arms.html"]
    marker = 'data-figure="harnesses/claude-code/arms/ALL/defect_rate_accepted" data-estimate="'
    start = page.index(marker) + len(marker)
    end = page.index('"', start)
    tampered = {**SITE, "arms.html": page[:start] + "0.123" + page[end:]}
    findings = check_site(tampered, BuildMode.SYNTHETIC, RESULTS)
    assert any("defect_rate_accepted shows estimate 0.123" in f for f in findings)


def test_a_figure_without_its_sample_size_fails() -> None:
    page = SITE["arms.html"]
    tampered = {**SITE, "arms.html": page.replace('<span class="n">', '<span class="m">')}
    findings = check_site(tampered, BuildMode.SYNTHETIC, RESULTS)
    assert any("does not show its sample size" in f for f in findings)


def test_a_count_that_differs_from_the_results_fails() -> None:
    page = SITE["index.html"]
    tampered = {
        **SITE,
        "index.html": page.replace(
            'data-figure="sample/tasks" data-value="6"', 'data-figure="sample/tasks" data-value="7"'
        ),
    }
    findings = check_site(tampered, BuildMode.SYNTHETIC, RESULTS)
    assert any("count sample/tasks shows 7" in f for f in findings)
