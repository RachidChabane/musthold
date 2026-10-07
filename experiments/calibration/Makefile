# Front door of the calibration experiment: thin recipes over script/<verb>. Every verb also runs without make:
# ./script/<verb>. Pass verb arguments with ARGS="--run-id run-01".
.PHONY: help setup lint test check snapshot build-items pin canary run run-session log-deviation analyze derive report-lint

help:  ## list verbs
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN{FS=":.*?## "}{printf "  %-16s %s\n",$$1,$$2}'

setup:  ## install the locked dependencies (needs the network)
	script/setup

lint:  ## ruff, pyright strict, import-linter and gofmt of the builders
	script/lint

test:  ## the harness tests in one process under mem-guard (no network, no model)
	script/test $(ARGS)

check:  ## lint, then test: what CI runs
	script/check

snapshot:  ## unpack the pilot snapshot of the a1 tag
	script/snapshot $(ARGS)

build-items:  ## product export, Go builders and every item, before any model call
	script/build-items $(ARGS)

pin:  ## pin the models into models.lock.json under the static memory rule
	script/pin $(ARGS)

canary:  ## the canary of one model (ARGS="--run-id run-01 --model M3")
	script/canary $(ARGS)

run:  ## one model block (ARGS="--run-id run-01 --model M3")
	script/run $(ARGS)

run-session:  ## one nightly session over the next unfinished blocks
	script/run-session $(ARGS)

log-deviation:  ## append a blocked or protocol entry (ARGS="--run-id run-01 --kind protocol --scope all --text ...")
	script/log-deviation $(ARGS)

analyze:  ## recompute every figure of results/ from the recorded bytes
	script/analyze $(ARGS)

derive:  ## results/policy-inputs.json from results/decisions.json
	script/derive $(ARGS)

report-lint:  ## lint results/REPORT.md
	script/report-lint $(ARGS)
