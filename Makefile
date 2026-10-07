# Deterministic front door for THIS repo (determinism > AI: prefer scripts over
# re-deriving repeatable work with the model — cheaper, predictable, testable).
# Keep recipes thin; put real work in script/. `make help` lists verbs.
# The pilot verbs (setup, verify, test, run) drive experiments/limits-1-3; the
# product verbs (go-test, go-lint, ci) drive every Go module of go.work through
# script/*; name modules with MODULES="format discpower" to run a subset.
.PHONY: help setup verify test run go-test go-lint ci shallnot-build plugin-gen

help:  ## list verbs
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN{FS=":.*?## "}{printf "  %-8s %s\n",$$1,$$2}'

setup:  ## install deps / bootstrap
	cd experiments/limits-1-3/harness && uv sync --frozen

# 'verify' = fast, read-only, commit-safe. The commit gate runs this and blocks the
# commit if it fails — keep it quick and never destructive.
verify:  ## fast read-only checks (lint, typecheck, drift, Go architecture) — runs at commit time
	$(MAKE) -C experiments/limits-1-3 leak-check
	cd experiments/limits-1-3/harness && uv run --frozen lint-imports
	script/lint
	cd tools/archtest && go test -count=1 -p 1 ./...

test:  ## full pilot test suite (slow — kept out of the commit gate)
	$(MAKE) -C experiments/limits-1-3 test

run:  ## list the pilot experiment verbs
	$(MAKE) -C experiments/limits-1-3 help

go-test:  ## run the Go tests of each module and write JUnit XML (script/test)
	script/test $(MODULES)

go-lint:  ## gofmt check and go vet on each Go module (script/lint)
	script/lint $(MODULES)

ci:  ## build the pinned shallnot when SHALLNOT_SRC exists, lint, then self-gate each Go module (script/ci)
	script/ci $(MODULES)

shallnot-build:  ## build the pinned Shallnot into .tools/bin (script/shallnot-build)
	script/shallnot-build

.PHONY: mem-check
mem-check:  ## refuse heavy local work unless memory is free enough (script/mem-guard)
	./script/mem-guard check

plugin-gen:  ## regenerate the committed plugin trees (script/plugin-gen)
	script/plugin-gen
