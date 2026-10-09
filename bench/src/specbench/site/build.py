"""The whole site as files: every page rendered from the inputs, and the stylesheet, keyed by their published path."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Final

from specbench.site import pages
from specbench.site.model import SiteInputs
from specbench.site.results_view import results_view
from specbench.site.template_files import STYLESHEET, stylesheet

STYLESHEET_PATH: Final = f"assets/{STYLESHEET}"


def build_site(inputs: SiteInputs) -> Mapping[str, str]:
    """Every file of the site; the same inputs give the same bytes, whatever order they were read in."""
    results = results_view(inputs.results)
    cases = tuple(sorted(inputs.cases, key=lambda case: case.record.case_id))
    ordered = SiteInputs(
        inputs.mode, inputs.results, inputs.manifest, inputs.deviations, cases, inputs.source_commit, inputs.generator
    )
    site_pages = [
        pages.overview(ordered, results),
        pages.cases_index(ordered),
        *(pages.case_page(case) for case in cases),
        pages.arms_page(ordered, results),
        pages.modules_page(ordered, results),
        pages.methodology_page(ordered, results),
        pages.limits_page(ordered, results),
        pages.reproduce_page(ordered),
    ]
    files = {page.path: pages.render_page(ordered, page) for page in site_pages}
    files[STYLESHEET_PATH] = stylesheet()
    return dict(sorted(files.items()))
