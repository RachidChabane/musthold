"""OSI-permissive licences (ADR 0004): SPDX ids from the licence API, and recognition of a LICENSE text."""

from __future__ import annotations

import re
from typing import Final

MIT: Final = "MIT"
APACHE_2: Final = "Apache-2.0"
BSD_2: Final = "BSD-2-Clause"
BSD_3: Final = "BSD-3-Clause"
ISC: Final = "ISC"

PERMISSIVE_SPDX: Final = frozenset({MIT, APACHE_2, BSD_2, BSD_3, ISC})
SEARCH_LICENCE_KEYS: Final = ("mit", "apache-2.0", "bsd-2-clause", "bsd-3-clause", "isc")
LICENCE_FILE_NAMES: Final = ("LICENSE", "LICENSE.md", "LICENSE.txt", "COPYING")
NOTICE_FILE_NAMES: Final = ("NOTICE", "NOTICE.md", "NOTICE.txt")

_WHITESPACE = re.compile(r"\s+")
_MIT_GRANT = "permission is hereby granted, free of charge, to any person obtaining a copy"
_APACHE_TITLE = "apache license"
_APACHE_VERSION = "version 2.0"
_BSD_GRANT = "redistribution and use in source and binary forms, with or without modification, are permitted"
_BSD_NO_ENDORSEMENT = "neither the name of"
_ISC_GRANT = "permission to use, copy, modify, and/or distribute this software for any purpose with or without fee"


def is_permissive(spdx_id: str | None) -> bool:
    return spdx_id in PERMISSIVE_SPDX


def classify_licence_text(text: str) -> str | None:
    """The permissive SPDX id a LICENSE text states, or None when it matches none of them."""
    flat = _WHITESPACE.sub(" ", text).lower()
    if _APACHE_TITLE in flat and _APACHE_VERSION in flat:
        return APACHE_2
    if _BSD_GRANT in flat:
        return BSD_3 if _BSD_NO_ENDORSEMENT in flat else BSD_2
    if _ISC_GRANT in flat:
        return ISC
    if _MIT_GRANT in flat:
        return MIT
    return None
