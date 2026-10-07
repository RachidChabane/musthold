import datetime
import decimal
import importlib
import inspect
import json
import signal
import sys
import types
import typing

CALL_TIMEOUT_SECONDS = 5
TARGET_SEPARATOR = ":"
ATTRIBUTE_SEPARATOR = "."

HINT_BOOLEAN = "boolean"
HINT_INTEGER = "integer"
HINT_DECIMAL = "decimal"
HINT_STRING = "string"
HINT_DATETIME = "datetime"
HINT_UNKNOWN = "unknown"

ANNOTATION_HINTS = {
    bool: HINT_BOOLEAN,
    int: HINT_INTEGER,
    float: HINT_DECIMAL,
    decimal.Decimal: HINT_DECIMAL,
    str: HINT_STRING,
    datetime.datetime: HINT_DATETIME,
    datetime.date: HINT_DATETIME,
}
ANNOTATION_NAME_HINTS = {
    "bool": HINT_BOOLEAN,
    "int": HINT_INTEGER,
    "float": HINT_DECIMAL,
    "Decimal": HINT_DECIMAL,
    "str": HINT_STRING,
    "datetime": HINT_DATETIME,
    "date": HINT_DATETIME,
}
SKIPPED_KINDS = (inspect.Parameter.VAR_POSITIONAL, inspect.Parameter.VAR_KEYWORD)
UNION_ORIGINS = tuple(
    origin for origin in (typing.Union, getattr(types, "UnionType", None)) if origin is not None
)


def resolve(target):
    module_name, _, function_name = target.partition(TARGET_SEPARATOR)
    function = importlib.import_module(module_name)
    for part in function_name.split(ATTRIBUTE_SEPARATOR):
        function = getattr(function, part)
    return function


def unwrap_optional(annotation):
    if typing.get_origin(annotation) in UNION_ORIGINS:
        members = [member for member in typing.get_args(annotation) if member is not type(None)]
        if len(members) == 1:
            return members[0]
    return annotation


def annotation_hint(annotation):
    if isinstance(annotation, str):
        return ANNOTATION_NAME_HINTS.get(annotation.split(ATTRIBUTE_SEPARATOR)[-1], HINT_UNKNOWN)
    try:
        return ANNOTATION_HINTS.get(unwrap_optional(annotation), HINT_UNKNOWN)
    except TypeError:
        return HINT_UNKNOWN


def default_hint(default):
    return ANNOTATION_HINTS.get(type(default), HINT_UNKNOWN)


def hint_of(parameter):
    if parameter.annotation is not inspect.Parameter.empty:
        hint = annotation_hint(parameter.annotation)
        if hint != HINT_UNKNOWN:
            return hint
    if parameter.default is not inspect.Parameter.empty:
        return default_hint(parameter.default)
    return HINT_UNKNOWN


def signature_of(target):
    function = resolve(target)
    params = [
        {"name": name, "hint": hint_of(parameter)}
        for name, parameter in inspect.signature(function).parameters.items()
        if parameter.kind not in SKIPPED_KINDS
    ]
    return {"target": target, "params": params}


def parse_datetime(text):
    try:
        return datetime.datetime.fromisoformat(text)
    except ValueError:
        return datetime.date.fromisoformat(text)


ARGUMENT_CONVERTERS = {
    "integer": int,
    "decimal": decimal.Decimal,
    "string": str,
    "boolean": lambda text: text == "true",
    "null": lambda text: None,
    "datetime": parse_datetime,
}


def convert_argument(literal):
    return ARGUMENT_CONVERTERS[literal["type"]](literal["value"])


def literal(kind, value):
    return {"type": kind, "value": value}


def convert_result(value):
    if isinstance(value, bool):
        return literal("boolean", "true" if value else "false")
    if isinstance(value, int):
        return literal("integer", str(value))
    if isinstance(value, float):
        return literal("decimal", repr(value))
    if isinstance(value, decimal.Decimal):
        return literal("decimal", str(value))
    if isinstance(value, str):
        return literal("string", value)
    if value is None:
        return literal("null", "null")
    if isinstance(value, (datetime.datetime, datetime.date, datetime.time)):
        return literal("datetime", value.isoformat())
    return literal("string", repr(value))


def returned(value):
    observation = convert_result(value)
    observation["kind"] = "returns"
    return observation


def raised(error):
    return {"kind": "raises", "value": type(error).__name__}


def on_alarm(signum, frame):
    raise TimeoutError()


def with_deadline(action):
    armed = hasattr(signal, "SIGALRM")
    if armed:
        signal.signal(signal.SIGALRM, on_alarm)
        signal.alarm(CALL_TIMEOUT_SECONDS)
    try:
        return action()
    finally:
        if armed:
            signal.alarm(0)


def run_call(call):
    function = resolve(call["target"])
    arguments = {name: convert_argument(value) for name, value in call["args"].items()}
    return function(**arguments)


def observe(call):
    try:
        return returned(with_deadline(lambda: run_call(call)))
    except Exception as error:
        return raised(error)


def handle(request):
    if request["op"] == "signatures":
        return {"signatures": [signature_of(target) for target in request["targets"]]}
    return {"observations": [observe(call) for call in request["calls"]]}


def main(request_path, response_path):
    with open(request_path, encoding="utf-8") as source:
        request = json.load(source)
    try:
        response = handle(request)
    except Exception as error:
        response = {"error": "%s: %s" % (type(error).__name__, error)}
    with open(response_path, "w", encoding="utf-8") as sink:
        json.dump(response, sink)


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
