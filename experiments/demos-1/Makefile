# Front door of the first scan demos (demos-1): thin recipes over script/<verb>. Every verb also runs
# without make: ./script/<verb>. Pass verb arguments with ARGS="--replay DIR --out DIR".
.PHONY: help setup lint test test-containers check build-product build-image select attempt summarize report-lint

help:  ## list verbs
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN{FS=":.*?## "}{printf "  %-16s %s\n",$$1,$$2}'

setup:  ## install the locked dependencies of the harness (needs the network)
	script/setup

lint:  ## ruff, pyright strict, import-linter and shellcheck
	script/lint

test:  ## the harness tests without containers, in one process under mem-guard
	script/test $(ARGS)

test-containers:  ## PREREG 15.4 fixture tests in the engine's machine (builds the product from HEAD and the image)
	script/test-containers $(ARGS)

check:  ## lint, then test: what CI runs
	script/check

build-product:  ## PREREG 2 linux build of spec-proof and spec-proof-dp (ARGS="--ref REF", default demos-1-product)
	script/build-product $(ARGS)

build-image:  ## PREREG 2 image from both source images by digest
	script/build-image

select:  ## PREREG 5-7 the four lists, once (ARGS="--replay DIR --out DIR" recomputes them offline)
	script/select $(ARGS)

attempt:  ## PREREG 8-10 the attempts, pytest first, resumable
	script/attempt

summarize:  ## PREREG 12.2 notes/demos-1/summary.json and SUMMARY.md
	script/summarize

report-lint:  ## PREREG 12.3 lint of SUMMARY.md
	script/report-lint
