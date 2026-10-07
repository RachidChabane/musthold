# Front door of the benchmark core. Every recipe is a thin call to bench/script/<verb>, which runs with no
# tooling beyond uv (scripts work without make). Pass verb arguments with ARGS="--run runs/sample-01".
.PHONY: help setup test lint check mine keygen seal unseal verify-corpus build-env verify-task build-workspace analyze report-lint window-status

help:  ## list verbs
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN{FS=":.*?## "}{printf "  %-16s %s\n",$$1,$$2}'

setup:  ## install the locked dependencies (needs the network)
	script/setup

test:  ## run the tests in one process under mem-guard (no network)
	script/test $(ARGS)

lint:  ## ruff, pyright strict, import-linter
	script/lint

check:  ## lint, then test: what CI runs
	script/check

mine:  ## mine candidate tasks (ARGS="--out DIR --archive DIR [--dry-run]")
	script/mine $(ARGS)

keygen:  ## create the sealing identity outside the repository
	script/keygen $(ARGS)

seal:  ## seal a plain task into the corpus and update the lock
	script/seal $(ARGS)

unseal:  ## decrypt a task outside the repository
	script/unseal $(ARGS)

verify-corpus:  ## R0 corpus check against corpus.lock.json
	script/verify-corpus $(ARGS)

build-env:  ## build a native per-task environment under mem-guard
	script/build-env $(ARGS)

verify-task:  ## two-way verification of a task, no model call
	script/verify-task $(ARGS)

build-workspace:  ## fresh single-commit agent workspace
	script/build-workspace $(ARGS)

analyze:  ## results.json and REPORT.md of a run directory
	script/analyze $(ARGS)

report-lint:  ## lint a rendered report
	script/report-lint $(ARGS)

window-status:  ## exit 0 inside the nightly window, 3 outside
	script/window-status $(ARGS)
