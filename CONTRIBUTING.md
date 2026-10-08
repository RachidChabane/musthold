# Contributing

This repository is built with the tools it ships: every behaviour is a requirement
with an id, every requirement is traced to tests by Shallnot, and the modules
self-gate in CI. Read [the architecture](docs/architecture.md) first.

## Prerequisites

- Go (the version in `go.work`), `git`, `make`, `bash`.
- Shallnot with report schema 1.2 or later, to run the self-gate. Build the pinned
  one with `script/shallnot-build`; it reads the commit in `script/shallnot.pin`
  from the git repository at `$SHALLNOT_SRC` (default `../shallnot-proof-gate`) and
  writes `.tools/bin/shallnot`.
- For the ecosystem fixtures of `discpower`: Python with pytest, Node with npm, a JDK
  21, and Maven on `PATH` (the Gradle fixture carries its wrapper), as the CI job
  installs them.
- `uv`, for the pilot and benchmark checks that `make verify` and `make ci` also run.

## Front door

Every verb is a shebang script in `script/` and a `make` target that calls it, so
`script/<verb>` works without `make`. `make help` lists the verbs.

| Verb | Script | Does |
|---|---|---|
| `make verify` | none | fast read-only checks: pilot leak check and import contracts, `script/lint`, the architecture tests; the commit gate |
| `make go-lint` | `script/lint [MODULE...]` | `gofmt` check and `go vet` per module |
| `make go-test` | `script/test [MODULE...]` | the Go tests of each module, writing JUnit XML to `<module>/build/test-results/go.xml` |
| `make ci` | `script/ci [MODULE...]` | build the pinned Shallnot, then lint and self-gate each module with `shallnot gate`, which runs its tests and traces its specs to the results |
| `make gate-changed` | `script/gate-changed [BASE [HEAD]]` | self-gate the modules a range of commits touches (every module when `format` or `go.work` changes, `tools/e2e` for changes outside the modules) |
| `make install-git-hooks` | `script/install-git-hooks` | install the pre-push hook that runs `script/gate-changed` on what a push carries and refuses the push when a gate fails |
| `make watch-ci` | `script/watch-ci [COMMIT]` | wait for the CI run of a commit and report its outcome |
| `make plugin-gen` | `script/plugin-gen` | regenerate the committed plugin trees |
| `make shallnot-build` | `script/shallnot-build` | build the pinned Shallnot |
| `make mem-check` | `script/mem-guard check` | refuse heavy work when memory is short |
| `make release-dry-run TAG=<module>/vX.Y.Z` | `script/release <tag> --dry-run` | rehearse a release: report what a real release would refuse, build every target, verify `dist/` and install from it |
| `make release TAG=<module>/vX.Y.Z` | `script/release <tag>` | run the pre-flight, build and verify the release files into `dist/`; publishes nothing |

Name modules to run a subset: `make ci MODULES="format discpower"` or
`script/ci format discpower`. The modules are those listed in `go.work`
(`script/modules`). `.github/workflows/ci.yml` calls the same scripts, so what
passes locally passes in CI.

`script/install` is the verified installer a release publishes. Only the release
workflow publishes, on a tag the owner pushes; [releasing](docs/usage/releasing.md)
describes how a release is cut, checked and verified.

Before you open a change run `make verify`, then `script/ci` for the modules you
touched; `make install-git-hooks` makes every push do the second step. Run the tests of a package directly while you work:

```
cd discpower && nice -n 10 go test -p 1 -count=1 ./internal/domain/...
```

## Machine safety

The tests build binaries, execute mutants and start JVMs and Node processes. Keep
the machine usable:

- Run heavy commands under `nice -n 10` and with `-p 1` (one compiler or test
  package at a time), for example `nice -n 10 go build -p 1 ./...`. The scripts
  already do.
- `script/mem-guard check` exits 0 when at least 40 percent of memory is free and
  the pressure level is normal; `script/mem-guard run -- CMD ARGS` checks, runs the
  command under `nice` and stops it cleanly when pressure leaves normal.
  `MEM_GUARD_MIN_FREE_PCT` and `MEM_GUARD_INTERVAL_S` tune it. Exit 4 means refused,
  5 means interrupted.
- Do not start two heavy jobs at once.
- Tests that need much memory skip with a recorded reason when the guard refuses;
  the gate reports the skipped requirement and never drops it.

## Requirement-first workflow

1. **State the behaviour first.** Add the requirement to `specs/<module>.md` of the
   module that owns it, in Shallnot's Markdown format, with the next free id:

   ```markdown
   - **SP-1400~1**: WHEN a document names a command THE SYSTEM SHALL find it in the binary.
   ```

   One behaviour each, observable and decidable. Ids are unique across the
   repository and are never reused. A change of meaning bumps the revision
   (`SP-1400~2`) and every test that cites the old revision then fails the gate
   until it is re-checked against the new statement.
2. **Write the test, then the code.** Cite the requirement in the name of the test
   with `[verifies SP-1400~1]`, for a Go test in a `t.Run` subtest name:

   ```go
   t.Run("every command the documentation names exists [verifies SP-1400~1]", func(t *testing.T) { ... })
   ```

3. **Cover every layer the change touches**: unit tests for domain rules,
   integration tests for adapters and real I/O, contract tests at module
   boundaries (golden vectors, the `Store` contract suite), end-to-end tests of the
   built binaries for critical paths, and property or fuzz tests where the input
   space is wide.
4. **Run the self-gate** of the module (`script/ci <module>`). A requirement with no
   passing test, or whose tests were all skipped, is a finding.

Keep the structure the architecture tests enforce: domain without I/O, use cases
without adapters, adapters that do not import each other, no module importing
another except `format`. Name every meaningful literal as a constant. Write
identifiers, comments, test names, log messages and error strings in English.

Generated files (`<module>/plugin/`, the marketplace files) are never edited by
hand: change the generator and run `make plugin-gen`.

## Documentation

User documentation lives in `README.md`, the module READMEs and `docs/usage/`;
contributor documentation in this file and `docs/architecture.md`; decisions in
`docs/adr/`. Write for the reader of the page: state rules and facts that hold, not
the history of how they came to be. Rationale and history belong in an ADR, the
commit message or the pull request. A test in `tools/e2e` fails when a command, a
subcommand or a flag named in the documentation does not exist, or when a relative
link does not resolve.

## Commits

- Conventional commits: `type(scope): subject`, with types such as `feat`, `fix`,
  `refactor`, `test`, `docs`, `build`, `ci`, and the module as the scope.
- Keep a change small and separately reviewable; one concern per commit.
- Stage files explicitly by path (`git add path/to/file`); do not use `git add -A`.
- Write the message in English. Explain why in the body when it is not obvious.
- Do not add AI attribution to commits or pull requests: no `Co-Authored-By` trailer
  for a tool and no "generated with" line. A commit carries the author's identity
  only.
