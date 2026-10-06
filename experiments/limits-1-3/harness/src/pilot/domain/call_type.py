"""Call types of the pipeline: which template, schema and effort each model call uses (PREREG 4.5)."""

from __future__ import annotations

from enum import Enum
from typing import Final

from pilot.domain.model_call import Effort


class CallType(Enum):
    GENERATE = "generate"
    VALIDITY = "validity"
    ALIGNMENT = "alignment"
    PROBE = "probe"
    RELABEL = "relabel"
    AUDIT = "audit"
    LEAK_AUDIT = "leak_audit"


CALL_EFFORT: Final = {
    CallType.GENERATE: Effort.HIGH,
    CallType.VALIDITY: Effort.MEDIUM,
    CallType.ALIGNMENT: Effort.MEDIUM,
    CallType.PROBE: Effort.MEDIUM,
    CallType.RELABEL: Effort.HIGH,
    CallType.AUDIT: Effort.HIGH,
    CallType.LEAK_AUDIT: Effort.HIGH,
}

TEMPLATE_STEM: Final = {
    CallType.GENERATE: "generate",
    CallType.VALIDITY: "validity",
    CallType.ALIGNMENT: "alignment",
    CallType.PROBE: "probe",
    CallType.RELABEL: "relabel",
    CallType.AUDIT: "validity",
    CallType.LEAK_AUDIT: "leak_audit",
}

SCHEMA_STEM: Final = {
    CallType.GENERATE: "generate.schema.json",
    CallType.VALIDITY: "validity.schema.json",
    CallType.ALIGNMENT: "alignment.tmpl.schema.json",
    CallType.PROBE: "probe.schema.json",
    CallType.RELABEL: "relabel.schema.json",
    CallType.AUDIT: "validity.schema.json",
    CallType.LEAK_AUDIT: "leak_audit.schema.json",
}

TEMPLATE_SUFFIX: Final = ".md"

ROLE_CALL_TYPES: Final = (
    ("mutant_generator", (CallType.GENERATE,)),
    ("relabel_", (CallType.RELABEL,)),
    ("mutant_audit", (CallType.AUDIT,)),
    ("validity_judge", (CallType.VALIDITY,)),
    ("alignment_judge", (CallType.ALIGNMENT, CallType.PROBE)),
    ("leak_audit", (CallType.LEAK_AUDIT,)),
)


def call_types_of_roles(roles: tuple[str, ...]) -> tuple[CallType, ...]:
    """Call types a model with these lock roles performs, in enum order."""
    found = {
        call_type
        for role in roles
        for prefix, call_types in ROLE_CALL_TYPES
        if role.startswith(prefix)
        for call_type in call_types
    }
    return tuple(call_type for call_type in CallType if call_type in found)
