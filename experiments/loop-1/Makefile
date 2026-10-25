# Front door of the agent-loop study (loop-1): thin recipes over script/<verb>. Every verb also runs
# without make: ./script/<verb>. Pass verb arguments with ARGS="--dry-run".
.PHONY: help power build construct accept canary resolve freeze-check run oracle detect analyze report-lint log-deviation

help:  ## list verbs
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | sort | awk 'BEGIN{FS=":.*?## "}{printf "  %-16s %s\n",$$1,$$2}'

power:  ## PREREG 10.6 power table of the primary test (no input, fixed seed)
	script/power

build:  ## PREREG 2, 7.2 linux product binaries, runner, plugin trees and image
	script/build $(ARGS)

construct:  ## PREREG 3.3 author and oracle calls, domain by domain (spends Codex quota)
	script/construct $(ARGS)

accept:  ## PREREG 3.4 acceptance checks A1-A9 of every task
	script/accept $(ARGS)

canary:  ## PREREG 13.3 canary sessions C1-C3 (spends model quota, nightly window)
	script/canary $(ARGS)

resolve:  ## PREREG 13.4 freeze/inputs.json and freeze/inputs-SHA256SUMS
	script/resolve $(ARGS)

freeze-check:  ## PREREG 13.5 tags, timestamps, sums, pins and digests
	script/freeze-check $(ARGS)

run:  ## PREREG 6 the runs in the frozen order, resumable (ARGS="--dry-run" starts nothing)
	script/run $(ARGS)

oracle:  ## PREREG 4 blind oracle and held-out violations, after the run phase
	script/oracle $(ARGS)

detect:  ## PREREG 9.8 cheating detectors K1-K6, after the run phase
	script/detect $(ARGS)

analyze:  ## PREREG 10, 11 decisions, results/summary.json and results/REPORT.md
	script/analyze $(ARGS)

report-lint:  ## PREREG 11.4 lint of results/REPORT.md
	script/report-lint $(ARGS)

log-deviation:  ## PREREG 12 append an operator entry to DEVIATIONS.md
	script/log-deviation $(ARGS)
