import json
import os

OUT = "CITATIONS_OUT"


def pytest_collection_modifyitems(session, config, items):
    cited = {}
    for item in items:
        cited[item.nodeid] = sorted(str(mark.args[0]) for mark in item.iter_markers("verifies") if mark.args)
    with open(os.environ[OUT], "w", encoding="utf-8") as handle:
        json.dump(cited, handle, sort_keys=True)
