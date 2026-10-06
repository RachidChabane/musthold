# Front door for the limits 1-3 pilot: thin recipes over script/<verb>.
# Every verb also runs without make: ./script/<verb>.
.PHONY: help verify-labels runner-crosscheck leak-check traceability-check canary test lint

help:  ## list verbs
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-20s %s\n",$$1,$$2}'

verify-labels:  ## PREREG 3.2 label verification by execution (no model call)
	./script/verify-labels

runner-crosscheck:  ## matrix runner must reproduce prereg/label_matrix.csv exactly
	./script/runner-crosscheck

leak-check:  ## static in-universe rules on the fixture test files
	./script/leak-check

traceability-check:  ## null gate G0 precondition through shallnot
	./script/traceability-check

canary:  ## one synthetic call per (model, CLI) pair; needs RUN_ID=<run-id>
	./script/canary --run-id $(RUN_ID)

test:  ## harness unit and integration tests
	cd harness && uv run --frozen pytest

lint:  ## import contracts and type check
	cd harness && uv run --frozen lint-imports && uv run --frozen pyright
