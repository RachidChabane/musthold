"""HTML building blocks: escaped text, safe fragments and elements with their attributes in a fixed order."""

from __future__ import annotations

import html
from collections.abc import Iterable, Mapping
from typing import Final

VOID_ELEMENTS: Final = frozenset({"br", "hr", "img", "input", "link", "meta"})


class Markup(str):
    """A fragment that is already HTML: it is inserted as is, never escaped again."""

    __slots__ = ()


Content = Markup | str | int | None
Attributes = Mapping[str, str | None]


def escape(value: Content) -> Markup:
    """The HTML of a piece of content: plain text is escaped, a `Markup` fragment is kept."""
    if value is None:
        return Markup("")
    if isinstance(value, Markup):
        return value
    return Markup(html.escape(str(value), quote=True))


def join(parts: Iterable[Content], separator: str = "") -> Markup:
    return Markup(escape(separator).join(escape(part) for part in parts))


def _attributes(attributes: Attributes | None) -> str:
    if not attributes:
        return ""
    rendered = [
        name if value is None else f'{name}="{html.escape(value, quote=True)}"' for name, value in attributes.items()
    ]
    return " " + " ".join(rendered)


def element(name: str, attributes: Attributes | None = None, *children: Content) -> Markup:
    """`<name attributes>children</name>`; attributes keep the order they are given in."""
    opening = f"<{name}{_attributes(attributes)}>"
    if name in VOID_ELEMENTS:
        return Markup(opening)
    return Markup(f"{opening}{join(children)}</{name}>")


def classed(name: str, css_class: str, *children: Content) -> Markup:
    return element(name, {"class": css_class}, *children)


def link(href: str, *children: Content, css_class: str | None = None) -> Markup:
    attributes: dict[str, str | None] = {"href": href}
    if css_class is not None:
        attributes["class"] = css_class
    return element("a", attributes, *children)


def table(header: Iterable[Content], rows: Iterable[Iterable[Content]], caption: Content = None) -> Markup:
    """A data table in a scrolling frame, so a wide table never widens the page."""
    head = element("thead", None, element("tr", None, *(element("th", {"scope": "col"}, cell) for cell in header)))
    body = element("tbody", None, *(element("tr", None, *(element("td", None, cell) for cell in row)) for row in rows))
    parts: list[Content] = []
    if caption is not None:
        parts.append(element("caption", None, caption))
    parts += [head, body]
    return classed("div", "table-frame", element("table", None, *parts))
