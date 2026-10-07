"""JSON Schemas of the committed records, loaded from the package and used to validate documents."""

from __future__ import annotations

import json
from enum import Enum
from importlib import resources
from typing import Any, cast

import jsonschema
from jsonschema.exceptions import ValidationError

from specbench.domain.canonical import JsonValue


class SchemaName(Enum):
    TASK = "task"
    TRUNK = "trunk"
    MANIFEST = "manifest"
    CORPUS_LOCK = "corpus-lock"


def load_schema(name: SchemaName) -> dict[str, Any]:
    text = resources.files("specbench.schemas").joinpath(f"{name.value}.schema.json").read_text(encoding="utf-8")
    return cast(dict[str, Any], json.loads(text))


def schema_problems(name: SchemaName, document: JsonValue) -> list[str]:
    """Every violation of the schema by the document, in a stable order; empty when it conforms."""
    validator = jsonschema.Draft202012Validator(load_schema(name))
    raw = cast("list[ValidationError]", list(validator.iter_errors(document)))  # pyright: ignore[reportUnknownMemberType]
    errors = sorted(raw, key=lambda e: [str(p) for p in e.absolute_path])
    return [f"{'/'.join(str(p) for p in e.absolute_path) or '$'}: {e.message}" for e in errors]
