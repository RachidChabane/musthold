import importlib
import inspect
import json
import sys

module_name, operation = sys.argv[1], sys.argv[2]
report = {"importable": False, "callable": False, "public_functions": [], "parameters": [], "error": ""}
try:
    module = importlib.import_module(module_name)
    report["importable"] = True
    target = getattr(module, operation, None)
    report["callable"] = callable(target)
    report["public_functions"] = sorted(
        name
        for name, value in vars(module).items()
        if inspect.isfunction(value) and value.__module__ == module_name and not name.startswith("_")
    )
    if report["callable"]:
        report["parameters"] = [
            {"name": parameter.name, "kind": parameter.kind.name}
            for parameter in inspect.signature(target).parameters.values()
        ]
except Exception as error:
    report["error"] = type(error).__name__ + ": " + str(error)
print(json.dumps(report, sort_keys=True))
