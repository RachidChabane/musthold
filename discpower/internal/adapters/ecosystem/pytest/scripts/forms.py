import hashlib
import json
import marshal
import sys
import types

SITE_SEPARATOR = "::"
NEUTRAL_LINE = 1


def neutral(code):
    """The code object with every position and file name left out, its nested code objects too."""
    constants = tuple(neutral(item) if isinstance(item, types.CodeType) else item for item in code.co_consts)
    changes = {"co_consts": constants, "co_filename": "", "co_firstlineno": NEUTRAL_LINE}
    if hasattr(code, "co_linetable"):
        changes["co_linetable"] = b""
    if hasattr(code, "co_lnotab") and not hasattr(code, "co_linetable"):
        changes["co_lnotab"] = b""
    return code.replace(**changes)


def qualname_of(code, outer):
    name = getattr(code, "co_qualname", None)
    if name:
        return name
    return code.co_name if not outer else outer + "." + code.co_name


def units(code, outer=""):
    """Every function and method code object of a module, by qualified name."""
    found = {}
    for item in code.co_consts:
        if isinstance(item, types.CodeType):
            name = qualname_of(item, outer)
            found.setdefault(name, item)
            found.update({key: value for key, value in units(item, name).items() if key not in found})
    return found


def forms(sites):
    by_file = {}
    for site in sites:
        path, _, qualname = site.partition(SITE_SEPARATOR)
        by_file.setdefault(path, []).append(qualname)
    result = {}
    for path, qualnames in by_file.items():
        try:
            with open(path, encoding="utf-8") as source:
                module = compile(source.read(), path, "exec")
        except (OSError, SyntaxError, ValueError):
            continue
        compiled = units(module)
        for qualname in qualnames:
            code = compiled.get(qualname)
            if code is not None:
                result[path + SITE_SEPARATOR + qualname] = hashlib.sha256(marshal.dumps(neutral(code))).hexdigest()
    return result


def main(request_path, response_path):
    with open(request_path, encoding="utf-8") as request_file:
        request = json.load(request_file)
    try:
        response = {"forms": forms(request.get("sites", []))}
    except Exception as error:  # noqa: BLE001 - every failure is reported, never raised
        response = {"error": f"{type(error).__name__}: {error}"}
    with open(response_path, "w", encoding="utf-8") as response_file:
        json.dump(response, response_file)


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
