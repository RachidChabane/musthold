# Front door of the benchmark core. Every recipe is a thin call to bench/script/<verb>, which runs with no
# tooling beyond uv (scripts work without make). Pass verb arguments with ARGS="--run runs/sample-01".
#
# Corpus construction, in order (each step is resumable; native scripts first, make mirrors them):
#   1. script/setup
#   2. script/mine --out mining/<seed> --archive mining/<seed>/archive --seed <seed>     (network: gh, throttled)
#   3. script/select --mining mining/<seed> --out selection/<seed>                         (network for clones and builds,
#                                                                                          tests run offline, mem-guard)
#   4. script/keygen --identity ~/.specbench/identity.txt --recipients corpus/recipients.txt   (once, outside the repository)
#   5. script/construct --night          repeat each night until it exits 0; exit 3 means paused (window, budget or quota)
#   6. script/verify-corpus --corpus corpus --identity ~/.specbench/identity.txt
# construct --night shares the per-night Claude cap with any other job on the same night: lower --budget accordingly.
# Construction files that hold sealed material live under $SPECBENCH_WORK (default ~/.specbench-work), never in the repository.
.PHONY: help setup test lint check mine select construct keygen seal unseal verify-corpus build-env verify-task build-workspace analyze report-lint window-status

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

select:  ## probe mined candidates offline in seeded order (ARGS="--mining DIR --out DIR")
	script/select $(ARGS)

construct:  ## build, verify and seal the selected tasks, paced and resumable (ARGS="--night")
	script/construct $(ARGS)

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
