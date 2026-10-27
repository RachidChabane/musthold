"""Reads the Python syntax tree of a project at a git commit and in its working
tree, and prints the facts the detectors judge as one JSON document.

Usage: helper.py <project directory> <commit>
"""

import ast
import configparser
import hashlib
import json
import os
import subprocess
import sys

try:
    import tomllib
except ImportError:
    tomllib = None

PRUNED_DIRS = {".git", "__pycache__", "node_modules", ".venv", "venv", ".tox", ".pytest_cache", "site-packages"}
PYTEST_MODULE = "pytest"
MARK_VERIFIES = "mark.verifies"
MARK_CONSTRUCTS = {"mark.skip", "mark.skipif", "mark.xfail"}
CALL_CONSTRUCTS = {"skip", "xfail", "importorskip"}
RAISES = "raises"
HOOK_NAMES = {"pytest_collection_modifyitems", "pytest_ignore_collect"}
IGNORE_VARIABLES = {"collect_ignore", "collect_ignore_glob"}
TEST_FUNCTION_PREFIX = "test"
TEST_CLASS_PREFIX = "Test"
PYTESTMARK = "pytestmark"
TESTS_VARIABLE = "__test__"
CONFIG_KEYS = ("addopts", "testpaths", "python_files", "python_classes", "python_functions")
TEXT_LIMIT = 200
HASH_LENGTH = 16


def digest(text):
    return hashlib.sha256(text.encode("utf-8")).hexdigest()[:HASH_LENGTH]


class DiskTree:
    def __init__(self, root):
        self.root = root

    def files(self):
        found = []
        for directory, names, files in os.walk(self.root):
            names[:] = [name for name in names if name not in PRUNED_DIRS]
            for name in files:
                found.append(os.path.relpath(os.path.join(directory, name), self.root).replace(os.sep, "/"))
        return sorted(found)

    def read(self, path):
        with open(os.path.join(self.root, path), "rb") as handle:
            return handle.read().decode("utf-8")


class GitTree:
    def __init__(self, root, commit):
        self.root = root
        self.commit = commit

    def _git(self, *args):
        return subprocess.run(["git", "-C", self.root, *args], check=True, capture_output=True).stdout

    def files(self):
        listing = self._git("ls-tree", "-r", "-z", "--name-only", self.commit).decode("utf-8")
        names = [name for name in listing.split("\0") if name]
        return sorted(name for name in names if not set(name.split("/")) & PRUNED_DIRS)

    def read(self, path):
        return self._git("show", "%s:%s" % (self.commit, path)).decode("utf-8")


def flatten(node):
    parts = []
    while isinstance(node, ast.Attribute):
        parts.append(node.attr)
        node = node.value
    if isinstance(node, ast.Name):
        parts.append(node.id)
        return list(reversed(parts))
    return None


class Names:
    """Resolves expressions to dotted paths below the pytest module."""

    def __init__(self, tree):
        self.modules = {PYTEST_MODULE}
        self.members = {}
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                for alias in node.names:
                    if alias.name == PYTEST_MODULE:
                        self.modules.add(alias.asname or alias.name)
            elif isinstance(node, ast.ImportFrom) and node.module == PYTEST_MODULE:
                for alias in node.names:
                    self.members[alias.asname or alias.name] = alias.name

    def resolve(self, node):
        parts = flatten(node)
        if not parts:
            return None
        head, rest = parts[0], parts[1:]
        if head in self.modules and rest:
            return ".".join(rest)
        if head in self.members:
            return ".".join([self.members[head]] + rest)
        return None


def unparse(node):
    return ast.unparse(node)[:TEXT_LIMIT]


def fingerprint(node):
    return {"hash": digest(ast.dump(node)), "text": unparse(node)}


def verified_criteria(names, expression):
    """Criteria cited by a mark expression or a list of them."""
    cited = []
    if isinstance(expression, (ast.List, ast.Tuple)):
        for element in expression.elts:
            cited.extend(verified_criteria(names, element))
    elif isinstance(expression, ast.Call) and names.resolve(expression.func) == MARK_VERIFIES:
        for argument in expression.args:
            if isinstance(argument, ast.Constant) and isinstance(argument.value, str):
                cited.append({"criterion": argument.value, "line": expression.lineno})
    return cited


def assigned_marks(names, body):
    cited = []
    for statement in body:
        if isinstance(statement, ast.Assign) and any(
            isinstance(target, ast.Name) and target.id == PYTESTMARK for target in statement.targets
        ):
            cited.extend(verified_criteria(names, statement.value))
    return cited


def assertions(names, function):
    block_expressions = set()
    prints = []
    for node in ast.walk(function):
        if isinstance(node, (ast.With, ast.AsyncWith)):
            raising = [item for item in node.items if isinstance(item.context_expr, ast.Call) and names.resolve(item.context_expr.func) == RAISES]
            if raising:
                prints.append(fingerprint(node))
                block_expressions.update(id(item.context_expr) for item in raising)
        elif isinstance(node, ast.Assert):
            prints.append(fingerprint(node))
    for node in ast.walk(function):
        if isinstance(node, ast.Call) and names.resolve(node.func) == RAISES and id(node) not in block_expressions:
            prints.append(fingerprint(node))
    return prints


def collect_tests(names, tree, path):
    tests = []
    module_marks = assigned_marks(names, tree.body)

    def add(function, qualname, inherited):
        cited = list(inherited)
        for decorator in function.decorator_list:
            cited.extend(verified_criteria(names, decorator))
        tests.append(
            {
                "id": "%s::%s" % (path, qualname),
                "file": path,
                "line": function.lineno,
                "citations": cited,
                "assertions": assertions(names, function),
                "body": digest("".join(ast.dump(statement) for statement in function.body)),
            }
        )

    functions = (ast.FunctionDef, ast.AsyncFunctionDef)
    for node in tree.body:
        if isinstance(node, functions) and node.name.startswith(TEST_FUNCTION_PREFIX):
            add(node, node.name, module_marks)
        elif isinstance(node, ast.ClassDef) and node.name.startswith(TEST_CLASS_PREFIX):
            class_marks = list(module_marks) + assigned_marks(names, node.body)
            for decorator in node.decorator_list:
                class_marks.extend(verified_criteria(names, decorator))
            for member in node.body:
                if isinstance(member, functions) and member.name.startswith(TEST_FUNCTION_PREFIX):
                    add(member, "%s::%s" % (node.name, member.name), class_marks)
    return tests


def assigned_names(node):
    targets = node.targets if isinstance(node, ast.Assign) else [node.target]
    return [target.id for target in targets if isinstance(target, ast.Name)]


def collect_constructs(names, tree, path):
    constructs = []

    def record(node, kind, scope):
        constructs.append({"file": path, "line": node.lineno, "kind": kind, "scope": scope})

    def visit(node, scope):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
            if node.name in HOOK_NAMES:
                record(node, node.name, scope)
            scope = scope + [node.name]
        elif isinstance(node, (ast.Assign, ast.AnnAssign)):
            for name in assigned_names(node):
                if name in HOOK_NAMES | IGNORE_VARIABLES:
                    record(node, name, scope)
                if name == TESTS_VARIABLE and isinstance(node.value, ast.Constant) and node.value.value is False:
                    record(node, "%s = False" % TESTS_VARIABLE, scope)
        elif isinstance(node, (ast.Attribute, ast.Name)):
            resolved = names.resolve(node)
            if resolved in MARK_CONSTRUCTS:
                record(node, "pytest.%s" % resolved, scope)
        elif isinstance(node, ast.Call):
            resolved = names.resolve(node.func)
            if resolved in CALL_CONSTRUCTS:
                record(node, "pytest.%s()" % resolved, scope)
        for child in ast.iter_child_nodes(node):
            visit(child, scope)

    visit(tree, [])
    for construct in constructs:
        construct["scope"] = "::".join(construct["scope"])
    return constructs


def normalise(value):
    if isinstance(value, (list, tuple)):
        value = " ".join(str(item) for item in value)
    return " ".join(str(value).split())


def selected(section):
    return {key: normalise(section[key]) for key in CONFIG_KEYS if key in section}


def ini_options(text, section):
    parser = configparser.RawConfigParser(strict=False)
    parser.read_string(text)
    return selected(parser[section]) if parser.has_section(section) else None


def pyproject_options(text):
    if tomllib is None:
        raise ValueError("tomllib is not available")
    options = tomllib.loads(text).get("tool", {}).get("pytest", {}).get("ini_options")
    return selected(options) if options is not None else None


CONFIG_READERS = {
    "pytest.ini": lambda text: ini_options(text, "pytest"),
    "tox.ini": lambda text: ini_options(text, "pytest"),
    "setup.cfg": lambda text: ini_options(text, "tool:pytest"),
    "pyproject.toml": pyproject_options,
}


def extract(tree):
    facts = {"tests": [], "constructs": [], "config": {}, "unparsed": []}
    for path in tree.files():
        reader = CONFIG_READERS.get(path.rsplit("/", 1)[-1])
        try:
            text = tree.read(path) if reader or path.endswith(".py") else None
            if reader:
                options = reader(text)
                if options is not None:
                    facts["config"][path] = options
            elif text is not None:
                syntax = ast.parse(text)
                names = Names(syntax)
                facts["tests"].extend(collect_tests(names, syntax, path))
                facts["constructs"].extend(collect_constructs(names, syntax, path))
        except (SyntaxError, ValueError, UnicodeDecodeError, configparser.Error) as error:
            facts["unparsed"].append({"file": path, "error": str(error)})
    return facts


def main(arguments):
    root, commit = arguments
    json.dump({"before": extract(GitTree(root, commit)), "after": extract(DiskTree(root))}, sys.stdout)


if __name__ == "__main__":
    main(sys.argv[1:])
