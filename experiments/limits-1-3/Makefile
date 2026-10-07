# Front door for the limits 1-3 pilot: thin recipes over script/<verb>.
# Every verb also runs without make: ./script/<verb>.
.PHONY: isolation-table import-run-01 run-blindspot run-luna amendment-check run-window help verify-labels runner-crosscheck leak-check traceability-check assemble-arm-p leak-audit canary relabel run-core run-secondary log-deviation test lint

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

assemble-arm-p:  ## arm-P items and labels from violations and planted items (no model call)
	./script/assemble-arm-p

leak-audit:  ## PREREG 3.3 cross-family leak audit of fixture/parkfee (one model call)
	./script/leak-audit

canary:  ## one synthetic call per (model, call type) used; needs RUN_ID=<run-id>
	./script/canary --run-id $(RUN_ID)

relabel:  ## PREREG 3.4 blind relabels, two model calls; needs RUN_ID=<run-id>
	./script/relabel --run-id $(RUN_ID)

run-core:  ## traceability, arm T and P, F0-F3, validity, J-SONNET masked and probe; needs RUN_ID=<run-id>
	./script/run-core --run-id $(RUN_ID)

run-secondary:  ## other judges, arm O, mutant audit; needs RUN_ID=<run-id>
	./script/run-secondary --run-id $(RUN_ID)

log-deviation:  ## PREREG 4.4 one DEVIATIONS.md entry; needs RUN_ID, KIND, ENDPOINTS (none|all|D3,D4), TEXT
	./script/log-deviation --run-id $(RUN_ID) --kind $(KIND) --endpoints "$(ENDPOINTS)" --text "$(TEXT)"

test:  ## harness unit and integration tests
	cd harness && uv run --frozen pytest

lint:  ## import contracts and type check
	cd harness && uv run --frozen lint-imports && uv run --frozen pyright

.PHONY: oracle-classify audit-sample analyze report-lint precision-table

oracle-classify:  ## PREREG 6.6 oracle classes of every non-stillborn mutant; needs RUN_ID=<run-id>
	./script/oracle-classify --run-id $(RUN_ID)

audit-sample:  ## PREREG 6.7 A-OPUS audit sample (ids and strata); needs RUN_ID=<run-id>
	./script/audit-sample --run-id $(RUN_ID)

analyze:  ## endpoints, tables and results/REPORT.md; needs RUN_ID=<run-id>
	./script/analyze --run-id $(RUN_ID)

report-lint:  ## PREREG 13 lint of results/REPORT.md
	./script/report-lint

precision-table:  ## Wilson interval table written to prereg/precision-table.csv
	./script/precision-table

.PHONY: run-pilot
run-pilot:  ## PREREG 14 run order end to end, resumable; needs RUN_ID=<run-id>
	./script/run-pilot $(RUN_ID)

isolation-table:  ## C_D8 and the isolation of the hand-written violations (no model call)
	./script/isolation-table

import-run-01:  ## carry the sealed run-01's completed calls into a run; needs RUN_ID=<run-id>
	./script/import-run-01 --run-id $(RUN_ID)

run-blindspot:  ## contrast mutants, test writers, operator mutants, writer kill vectors; needs RUN_ID=<run-id>
	./script/run-blindspot --run-id $(RUN_ID)

run-luna:  ## the exploratory luna stratum inside its window; needs RUN_ID=<run-id>
	./script/run-luna --run-id $(RUN_ID)

amendment-check:  ## tags, declared paths, timestamps, seal and imports; needs RUN_ID=<run-id>
	./script/amendment-check --run-id $(RUN_ID)

run-window:  ## exit 0 inside the run window (PILOT_RUN_WINDOW, default 21:00-07:30), 3 outside
	./script/run-window
