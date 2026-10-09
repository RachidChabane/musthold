"""The check of a built site: what CI runs on the fixture build, and what a sealed build must pass before it is published.

- every page is well-formed: each element closes, in order;
- every internal link and fragment resolves, and nothing loads from another origin;
- every figure carries its sample size and interval, and matches the value at its path in the results;
- every count matches the results; a percent sign appears only inside a figure, a count or a chart;
- a synthetic build is badged on every page, a sealed build on none;
- no page uses a word the report lint forbids, outside quoted text.
"""

from __future__ import annotations

import json
import posixpath
import re
from collections.abc import Mapping
from dataclasses import dataclass, field
from html.parser import HTMLParser
from typing import Final

from specbench.domain.reporting import FORBIDDEN_WORDS
from specbench.site.html import VOID_ELEMENTS
from specbench.site.model import BuildMode
from specbench.site.results_view import resolve

HTML_SUFFIX: Final = ".html"
EXTERNAL_SCHEMES: Final = ("http://", "https://", "//")
NON_PAGE_SCHEMES: Final = ("mailto:",)
LOADING_ATTRIBUTES: Final = {("link", "href"), ("script", "src"), ("img", "src"), ("iframe", "src"), ("source", "src")}
FIGURE_ATTRIBUTES: Final = ("data-figure", "data-estimate", "data-lo", "data-hi", "data-n")
PERCENT: Final = "%"
PERCENT_CONTEXTS: Final = frozenset({"fig", "count"})
QUOTED_CLASS: Final = "quoted"
EXEMPT_TAGS: Final = frozenset({"code", "pre", "blockquote", "style", "title", "desc"})
SAMPLE_MARKER: Final = "n\u202f="
FIGURE_CLASS: Final = "fig"
SAMPLE_CLASS: Final = "n"
INTERVAL_CLASS: Final = "ci"
FORBIDDEN: Final = re.compile(r"\b(" + "|".join(re.escape(word) for word in FORBIDDEN_WORDS) + r")\b", re.IGNORECASE)
TOLERANCE: Final = 1e-9


@dataclass
class _Open:
    tag: str
    classes: frozenset[str]
    attributes: dict[str, str | None]
    text: list[str] = field(default_factory=list[str])
    inner_classes: set[str] = field(default_factory=set[str])


class _PageScan(HTMLParser):
    """One pass over a page: its elements, ids, links, figures and counts, and what is wrong with them."""

    def __init__(self, path: str) -> None:
        super().__init__(convert_charrefs=True)
        self.path = path
        self.findings: list[str] = []
        self.stack: list[_Open] = []
        self.ids: set[str] = set()
        self.links: list[str] = []
        self.figures: list[_Open] = []
        self.counts: list[_Open] = []
        self.badges = 0
        self.build: str | None = None

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        attributes = dict(attrs)
        classes = frozenset((attributes.get("class") or "").split())
        identifier = attributes.get("id")
        if identifier:
            self.ids.add(identifier)
        if tag == "body":
            self.build = attributes.get("data-build")
        if attributes.get("data-synthetic") == "true":
            self.badges += 1
        if tag == "script":
            self.findings.append(f"{self.path}: a page runs a script")
        self._references(tag, attributes)
        for outer in self.stack:
            if FIGURE_CLASS in outer.classes:
                outer.inner_classes |= classes
        if tag in VOID_ELEMENTS:
            return
        opened = _Open(tag, classes, attributes)
        self.stack.append(opened)
        if FIGURE_CLASS in classes:
            self.figures.append(opened)
        if "count" in classes:
            self.counts.append(opened)

    def _references(self, tag: str, attributes: dict[str, str | None]) -> None:
        """Record the internal links of an element, and flag what it would load from another origin."""
        for name in ("href", "src"):
            value = attributes.get(name)
            if value is None:
                continue
            if (tag, name) in LOADING_ATTRIBUTES and value.startswith(EXTERNAL_SCHEMES):
                self.findings.append(f"{self.path}: <{tag} {name}> loads from another origin: {value}")
            if not value.startswith(EXTERNAL_SCHEMES + NON_PAGE_SCHEMES):
                self.links.append(value)

    def handle_startendtag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag in VOID_ELEMENTS:
            self.handle_starttag(tag, attrs)
            return
        self.handle_starttag(tag, attrs)
        self.handle_endtag(tag)

    def handle_endtag(self, tag: str) -> None:
        if tag in VOID_ELEMENTS:
            return
        if not self.stack or self.stack[-1].tag != tag:
            expected = self.stack[-1].tag if self.stack else "nothing"
            self.findings.append(f"{self.path}: </{tag}> closes {expected}")
            return
        self.stack.pop()

    def handle_data(self, data: str) -> None:
        for opened in self.stack:
            if "fig" in opened.classes or "count" in opened.classes:
                opened.text.append(data)
        in_context = any(o.classes & PERCENT_CONTEXTS or o.tag == "svg" for o in self.stack)
        if PERCENT in data and not in_context:
            self.findings.append(f"{self.path}: a percentage outside a figure: {data.strip()!r}")
        exempt = any(o.tag in EXEMPT_TAGS or QUOTED_CLASS in o.classes for o in self.stack)
        match = FORBIDDEN.search(data)
        if match is not None and not exempt:
            self.findings.append(f"{self.path}: the word {match.group(1)!r} is not used in a results page")

    def close(self) -> None:
        super().close()
        for opened in self.stack:
            self.findings.append(f"{self.path}: <{opened.tag}> is never closed")


def _close(left: object, right: object) -> bool:
    if isinstance(left, int | float) and isinstance(right, int | float) and not isinstance(left, bool):
        return abs(float(left) - float(right)) <= TOLERANCE
    return left == right


def _figure_problems(path: str, figure: _Open, results: object) -> list[str]:
    attributes = figure.attributes
    missing = [name for name in FIGURE_ATTRIBUTES if attributes.get(name) is None]
    if missing:
        return [f"{path}: a figure lacks {', '.join(missing)}"]
    text = "".join(figure.text)
    pointer = attributes["data-figure"] or ""
    problems: list[str] = []
    if SAMPLE_CLASS not in figure.inner_classes or SAMPLE_MARKER not in text:
        problems.append(f"{path}: figure {pointer} does not show its sample size")
    try:
        value = resolve(results, pointer)
    except KeyError:
        return [*problems, f"{path}: figure {pointer} names no value of the results"]
    if not isinstance(value, Mapping):
        return [*problems, f"{path}: figure {pointer} is not an estimate of the results"]
    record: Mapping[str, object] = value  # pyright: ignore[reportUnknownVariableType]
    shown = {name: json.loads(attributes[f"data-{name}"] or "null") for name in ("estimate", "lo", "hi")}
    if "successes" in record:
        n, successes = record.get("n"), record.get("successes")
        expected: dict[str, object] = {
            "estimate": successes / n if isinstance(n, int) and isinstance(successes, int) and n else None,
            "lo": record.get("lo"),
            "hi": record.get("hi"),
        }
        if str(n) != attributes["data-n"]:
            problems.append(f"{path}: figure {pointer} shows n = {attributes['data-n']}, the results hold {n}")
    else:
        expected = {name: record.get(name) for name in ("estimate", "lo", "hi")}
    for name, held in expected.items():
        if not _close(shown[name], held):
            problems.append(f"{path}: figure {pointer} shows {name} {shown[name]}, the results hold {held}")
    defined = all(shown[name] is not None for name in ("estimate", "lo", "hi"))
    if defined and INTERVAL_CLASS not in figure.inner_classes and "is-reference" not in figure.classes:
        problems.append(f"{path}: figure {pointer} shows no interval")
    return problems


def _count_problems(path: str, count: _Open, results: object) -> list[str]:
    pointer = count.attributes.get("data-figure") or ""
    try:
        held = resolve(results, pointer)
    except KeyError:
        return [f"{path}: count {pointer} names no value of the results"]
    shown = json.loads(count.attributes.get("data-value") or "null")
    return [] if _close(shown, held) else [f"{path}: count {pointer} shows {shown}, the results hold {held}"]


def _link_problems(path: str, href: str, files: Mapping[str, str], ids: Mapping[str, set[str]]) -> list[str]:
    target, _, fragment = href.partition("#")
    resolved = posixpath.normpath(posixpath.join(posixpath.dirname(path), target)) if target else path
    if resolved not in files:
        return [f"{path}: the link {href} resolves to nothing"]
    if fragment and fragment not in ids.get(resolved, set()):
        return [f"{path}: the link {href} names a missing fragment"]
    return []


def check_site(files: Mapping[str, str], mode: BuildMode, results: object) -> list[str]:
    """Every problem of a built site, in a stable order; an empty list means the site may be published."""
    scans: dict[str, _PageScan] = {}
    for path, content in sorted(files.items()):
        if not path.endswith(HTML_SUFFIX):
            continue
        scan = _PageScan(path)
        scan.feed(content)
        scan.close()
        scans[path] = scan
    ids = {path: scan.ids for path, scan in scans.items()}
    findings: list[str] = []
    for path, scan in scans.items():
        findings += scan.findings
        for href in scan.links:
            findings += _link_problems(path, href, files, ids)
        for figure in scan.figures:
            findings += _figure_problems(path, figure, results)
        for count in scan.counts:
            findings += _count_problems(path, count, results)
        if scan.build != mode.value:
            findings.append(f"{path}: the page declares build {scan.build}, the site is {mode.value}")
        if mode.is_synthetic and scan.badges == 0:
            findings.append(f"{path}: a synthetic page carries no synthetic badge")
        if not mode.is_synthetic and scan.badges:
            findings.append(f"{path}: a sealed-run page carries the synthetic badge")
    if not scans:
        findings.append("the site has no page")
    return sorted(findings)
