"""The committed page templates and stylesheet, read as package data and filled by name.

A template holds the site's wording and structure only. Every figure reaches a page through a named slot, so a
template carries no number; `string.Template` refuses a slot left unfilled.
"""

from __future__ import annotations

from collections.abc import Mapping
from enum import Enum
from functools import cache
from importlib import resources
from string import Template
from typing import Final

from specbench.site.html import Markup

TEMPLATE_PACKAGE: Final = "specbench.site.templates"
ASSET_PACKAGE: Final = "specbench.site.assets"
STYLESHEET: Final = "site.css"
ENCODING: Final = "utf-8"


class PageTemplate(Enum):
    PAGE = "page.html"
    OVERVIEW = "overview.html"
    METHODOLOGY = "methodology.html"
    LIMITS = "limits.html"
    REPRODUCE = "reproduce.html"


@cache
def template_text(template: PageTemplate) -> str:
    return resources.files(TEMPLATE_PACKAGE).joinpath(template.value).read_text(encoding=ENCODING)


@cache
def stylesheet() -> str:
    return resources.files(ASSET_PACKAGE).joinpath(STYLESHEET).read_text(encoding=ENCODING)


def fill(template: PageTemplate, slots: Mapping[str, Markup]) -> Markup:
    """The template with every slot replaced by its HTML; a missing slot raises `KeyError`."""
    return Markup(Template(template_text(template)).substitute(slots))
