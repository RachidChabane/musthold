import ast
import importlib
import json
import os
import sys


def reach(payload):
    """The package functions each criterion's inline examples run, by criterion."""
    package = payload["package"]
    reached = {}
    errors = []

    def owned(frame):
        name = frame.f_globals.get("__name__", "")
        return name == package or name.startswith(package + ".")

    for example in payload["examples"]:
        functions = reached.setdefault(example["criterion"], set())

        def trace(frame, event, _argument):
            if event == "call" and owned(frame):
                code = frame.f_code
                functions.add(frame.f_globals["__name__"] + ":" + getattr(code, "co_qualname", code.co_name))

        try:
            operation = getattr(importlib.import_module(example["module"]), example["operation"])
            sys.setprofile(trace)
            try:
                eval("__operation(" + example["arguments"] + ")", {"__operation": operation})
            finally:
                sys.setprofile(None)
        except Exception as error:
            errors.append(example["criterion"] + ": " + type(error).__name__ + ": " + str(error))
    return {"reach": {criterion: sorted(found) for criterion, found in reached.items()}, "errors": errors}


def definitions(root):
    """Every function defined under a package directory, by module:qualname, as its AST."""
    found = {}
    for directory, _subdirectories, files in os.walk(root):
        for file in sorted(files):
            if not file.endswith(".py"):
                continue
            location = os.path.join(directory, file)
            relative = os.path.relpath(location, os.path.dirname(root))[: -len(".py")]
            module = relative.replace(os.sep, ".")
            if module.endswith(".__init__"):
                module = module[: -len(".__init__")]
            with open(location, encoding="utf-8") as handle:
                tree = ast.parse(handle.read())

            def visit(node, prefix):
                for child in ast.iter_child_nodes(node):
                    if isinstance(child, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
                        name = prefix + child.name
                        if not isinstance(child, ast.ClassDef):
                            found[module + ":" + name] = ast.dump(child)
                        visit(child, name + ".")

            visit(tree, "")
    return found


def changed(fixture, reference):
    """The functions of the fixture's package and those the reference changes."""
    before, after = definitions(fixture), definitions(reference)
    return {"defined": sorted(before), "changed": sorted(name for name in before if after.get(name) != before[name])}


mode, payload = sys.argv[1], sys.argv[2]
if mode == "reach":
    print(json.dumps(reach(json.loads(payload)), sort_keys=True))
else:
    fixture, reference = json.loads(payload)
    print(json.dumps(changed(fixture, reference), sort_keys=True))
