# Deterministic front door for THIS repo (determinism > AI: prefer scripts over
# re-deriving repeatable work with the model — cheaper, predictable, testable).
# Keep recipes thin; put real work in script/. `make help` lists verbs.
# The pilot verbs (setup, verify, test, run) drive experiments/limits-1-3; the
# product verbs (go-test, go-lint, ci) drive every Go module of go.work through
# script/*; name modules with MODULES="format discpower" to run a subset.
.PHONY: help setup verify test run go-test go-lint ci shallnot-build product-build plugin-gen release release-dry-run third-party-notices third-party-notices-check

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
	script/third-party-notices --check
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

product-build:  ## build every product binary and the umbrella plugin tree into .tools (script/product-build)
	script/product-build

.PHONY: mem-check
mem-check:  ## refuse heavy local work unless memory is free enough (script/mem-guard)
	./script/mem-guard check

.PHONY: gate-changed install-git-hooks watch-ci wait-job
gate-changed:  ## self-gate the modules changed since the upstream (script/gate-changed; ARGS="BASE HEAD")
	./script/gate-changed $(ARGS)

install-git-hooks:  ## install the pre-push hook that gates the modules a push touches (script/install-git-hooks)
	./script/install-git-hooks

watch-ci:  ## wait for the CI run of a commit and report its outcome (script/watch-ci; ARGS=COMMIT)
	./script/watch-ci $(ARGS)

wait-job:  ## wait for a background job by its PID with a bounded timeout, never by its output file alone (script/wait-job; ARGS="--pid PID --timeout S --log FILE")
	./script/wait-job $(ARGS)

.PHONY: escalation-rate
escalation-rate:  ## verdicts and escalation rate of the honest end-to-end fixtures per commit (script/escalation-rate; ARGS="BEFORE AFTER", SHALLNOT_SRC set)
	./script/escalation-rate $(ARGS)

.PHONY: regen-statement-fixtures
regen-statement-fixtures:  ## regenerate every fixture derived from the proof statements, in dependency order (script/regen-statement-fixtures)
	./script/regen-statement-fixtures

.PHONY: secrets-scan
secrets-scan:  ## scan the whole git history for secrets with gitleaks (script/secrets-scan)
	./script/secrets-scan

third-party-notices:  ## regenerate THIRD_PARTY_NOTICES.md from the Go module graph and the uv.lock files (script/third-party-notices)
	script/third-party-notices

third-party-notices-check:  ## fail when THIRD_PARTY_NOTICES.md differs from what script/third-party-notices writes
	script/third-party-notices --check

plugin-gen:  ## regenerate the committed plugin trees (script/plugin-gen)
	script/plugin-gen

# The release pipeline (ADR 0010): TAG=<module>/vMAJOR.MINOR.PATCH, ARGS for extra flags such as
# ARGS="--skip-tests --targets linux/amd64". Only the release workflow publishes.
release:  ## build and verify the release files of TAG into dist/ without publishing (script/release)
	$(if $(TAG),,$(error TAG is required, for example TAG=discpower/v0.2.0))
	script/release $(TAG) $(ARGS)

release-dry-run:  ## rehearse the release of TAG: report what a real release would refuse, build, verify and install from dist/ (script/release --dry-run)
	$(if $(TAG),,$(error TAG is required, for example TAG=discpower/v0.2.0))
	script/release $(TAG) --dry-run $(ARGS)

# The live canaries (ADR 0005, "Live canary"): one short model session per harness,
# recorded and evaluated. They spend model quota and run only in the nightly window;
# pass flags with ARGS="--dry-run" (see script/canary).
.PHONY: canary-build canary-claude canary-codex canary-copilot claude-code-install check-claude-code-pin

claude-code-install:  ## install the pinned Claude Code CLI into .tools/claude-code/<version> (script/claude-code-install; the version is script/claude-code.pin)
	script/claude-code-install

check-claude-code-pin:  ## fail when the Go constants that name the Claude Code version differ from script/claude-code.pin (script/check-claude-code-pin)
	script/check-claude-code-pin

canary-build:  ## build what the canaries need, a front door of script/product-build (script/canary-build)
	script/canary-build

canary-claude:  ## live canary of Claude Code under -p (ARGS="--dry-run --shared-home")
	script/canary-claude $(ARGS)

canary-codex:  ## live canary of Codex under exec (ARGS="--dry-run")
	script/canary-codex $(ARGS)

canary-copilot:  ## live canary of GitHub Copilot CLI under -p (ARGS="--dry-run --shared-home")
	script/canary-copilot $(ARGS)

# The benchmark core (bench/): every target calls bench/script/<verb> from the repository root, so paths in ARGS are
# relative to the root. bench/Makefile offers the same verbs from inside bench/.
.PHONY: bench-setup bench-test bench-lint bench-check bench-recordings bench-recordings-check bench-verify-corpus bench-analyze bench-report-lint bench-site

bench-setup:  ## install the benchmark core's locked dependencies (bench/script/setup)
	bench/script/setup

bench-test:  ## benchmark core tests, one process, under mem-guard (bench/script/test)
	bench/script/test

bench-lint:  ## benchmark core ruff, pyright strict and import contracts (bench/script/lint)
	bench/script/lint

bench-check:  ## benchmark core lint then tests, as CI runs them (bench/script/check)
	bench/script/check

bench-recordings:  ## regenerate the parcel answers the clean-room gate tests replay (script/regen-bench-recordings; needs make product-build)
	script/regen-bench-recordings

bench-recordings-check:  ## fail when the committed parcel answers differ from what the generator writes (script/regen-bench-recordings --check)
	script/regen-bench-recordings --check

bench-verify-corpus:  ## R0: corpus digests against corpus.lock.json (ARGS="--corpus bench/corpus")
	bench/script/verify-corpus $(ARGS)

bench-analyze:  ## R0: results.json and REPORT.md of a run (ARGS="--run bench/runs/<id>")
	bench/script/analyze $(ARGS)

bench-report-lint:  ## R0: lint a rendered report (ARGS="--report bench/runs/<id>/REPORT.md")
	bench/script/report-lint $(ARGS)

bench-site:  ## the results site into bench/build/site (ARGS="--fixtures" or ARGS="--run bench/runs/<id>"; --check writes nothing)
	bench/script/site $(ARGS)
