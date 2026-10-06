# Deterministic front door for THIS repo (determinism > AI: prefer scripts over
# re-deriving repeatable work with the model — cheaper, predictable, testable).
# Keep recipes thin; put real work in script/. `make help` lists verbs.
.PHONY: help setup verify test run

help:  ## list verbs
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN{FS=":.*?## "}{printf "  %-8s %s\n",$$1,$$2}'

setup:  ## install deps / bootstrap
	cd experiments/limits-1-3/harness && uv sync --frozen

# 'verify' = fast, read-only, commit-safe. The commit gate runs this and blocks the
# commit if it fails — keep it quick and never destructive.
verify:  ## fast read-only checks (lint, typecheck, drift) — runs at commit time
	$(MAKE) -C experiments/limits-1-3 leak-check
	cd experiments/limits-1-3/harness && uv run --frozen lint-imports

test:  ## full test suite (slow — kept out of the commit gate)
	$(MAKE) -C experiments/limits-1-3 test

run:  ## list the pilot experiment verbs
	$(MAKE) -C experiments/limits-1-3 help
