# CI: prove, push and pull, ledger and verification

Proofs made on a laptop are advisory. CI is where a proof becomes one the gate
accepts under `proofs.trust: signed`. CI holds no model key and calls no model:
it recomputes everything it can, re-executes every executed claim, and takes the
judged answers the developer's host already gave from a pushed bundle.

This page orders the commands; each is specified in the page of its program:
[commands](commands.md), [ledger](ledger.md) and [verify](verify.md). CI needs the
three proof binaries, `spec-proof-ledger`, `spec-proof-verify`, Shallnot and your
project's toolchains on the runner.

## 1. On the developer machine: push the host's answers

After the proof loop has answered every ticket and the work is committed:

```
spec-proof-dp host push --remote origin
spec-proof-alignment host push --remote origin
spec-proof-completeness host push --remote origin
```

Each command publishes the answered tickets of its module as a tree under
`refs/spec-proof/host/<module>/<head sha>`, where `<module>` is `discpower`,
`alignment` or `completeness`. The tree holds the responses keyed by request id, the
journal digest, the marks against each response (leak, exposure, conflict) and the
module version. The working tree and the current branch are untouched. Commit
first: the command warns when a tracked file differs from the head commit, because
CI recomputes request ids from that commit and finds no answer for a request whose
inputs changed.

Mutant bodies a host wrote travel in this tree in plaintext and are known to the
agent. Only the operator mutants and sentinels CI draws are hidden from it.

## 2. In CI: check out the pull request head

Check out the head commit of the pull request, not the merge commit, so the
recomputed request ids match the pushed ones. Fetch the refs and export each tree
into a directory per module:

```
git fetch origin refs/spec-proof/host/discpower/<head sha>
mkdir -p host/discpower && git archive FETCH_HEAD | tar -x -C host/discpower
```

Repeat for `alignment` and `completeness`. A module bundle is the flat set of the
files in that tree.

## 3. In CI: prove each module

Run the modules in the order discriminating power, alignment, completeness. Set
`SPEC_PROOF_CI_RUN_ID` (GitHub Actions' `GITHUB_RUN_ID` is used when it is unset):

```
spec-proof-dp prove --ci --import-host host/discpower
spec-proof-alignment prove --ci --import-host host/alignment
spec-proof-completeness prove --ci --import-host host/completeness
```

With `--ci` a module:

- draws the seed of the head commit from the first CI run that saw it (the SHA-256
  of the head commit and the run id joined by a newline), records it under
  `.spec-proof/cache` and reuses it on every later run of the commit, so
  re-triggering CI cannot shop for a favourable draw;
- recomputes every request id from the tree, ignores bundle responses it does not
  ask for, and re-validates schemas;
- re-executes every executed claim (builds, tests, examples, kills) and draws fresh
  operator mutants and sentinels;
- serves the tickets it asks for from the bundle through the same checks as any
  answer, and marks again what the pushing host marked;
- writes statements with `producedBy: ci`;
- refuses a bundle made by another module version when requests are left
  unanswered, and names both versions.

A request the bundle does not answer stays pending (exit 3, `awaiting_host`). A
CI-fresh surviving mutant that needs a judgment becomes a CI-issued request:
`prove --ci` writes it under `.spec-proof/ci-issued/` for the job to upload as an
artifact named `spec-proof-host-<module>`, and the criterion escalates with the
pending reason `awaiting_host_judgment` until it is answered.

## 4. The CI-issued round trip

On the developer machine:

```
spec-proof-dp host pull --ci-run <run id> --repo <owner>/<name>
```

`pull` downloads the artifact with `gh run download`, so `gh` must be installed and
authenticated, and makes tickets of its requests. The host answers them as in the
local loop, `host push` publishes, and CI runs again.

## 5. In CI: sign, append, export, verify

```
spec-proof-ledger sign --identity ed25519 --in .spec-proof/cache --out signed
spec-proof-ledger ledger append --signed signed --head-sha <head sha> --run-id <run id> --remote origin
spec-proof-ledger ledger export --to bundles
spec-proof-verify --bundles bundles --offline --out .spec-proof/verified
shallnot gate
```

- `sign` admits only CI-produced statements at the paths their content dictates.
  Use `--identity sigstore` for keyless signing from a CI identity.
- `append` adds the proofs to the ledger branch `spec-proof/ledger`, records the
  seed of the commit once, and with `--remote` fetches, fast-forwards and pushes it
  without forcing. Pass `--mutants DIR` to store encrypted mutant bodies and
  `--trust DIR`, from `ledger fetch-trust`, to store the trust snapshot that makes
  `--offline` verification possible.
- `export` writes the proofs and the trust snapshot of the ledger tip.
- `verify` checks every bundle against `.spec-proof/trust-policy.yaml` (see
  [configuration](configuration.md#spec-prooftrust-policyyaml)), folds the pending
  files and writes the verified set. It rejects the whole run when any bundle fails.
- `shallnot gate` runs the project's tests, reads `.spec-proof/verified` and gives
  the verdict.

`spec-proof-ledger ledger audit` checks at any time that the ledger is append-only
and content-addressed.

## Repository settings

- `proofs.trust: signed`, the Shallnot default, in `shallnot.yaml`.
- `proofs.code` naming the source the proofs may reference.
- A `trust-policy.yaml` pinning the signing key or the CI identity.
- For Sigstore keyless signing on GitHub Actions, the workflow permission
  `id-token: write`.
