# ADR 0010: Release pipeline, one release per module

Status: accepted. Implements the "one release per module" decision of ADR 0005.

## Context

ADR 0005 gives every module its own `go.mod`, binary, plugin, version and release.
The released modules are:
- the `format` library;
- the binaries and plugins of `discpower` (`spec-proof-dp`), `alignment`,
  `completeness` and `s1relay` (`spec-proof-s1`);
- the binaries of `ledger` and `verify`;
- the `umbrella` plugin.

The internal tools under `tools/` are never released. The pipeline must:
- version and tag each module on its own;
- build reproducible binaries for linux, macOS and Windows on amd64 and arm64;
- publish archives with checksums, a keyless Sigstore signature, GitHub artifact
  attestations (SLSA provenance) and an SBOM;
- provide `go install`, a verified install script, the plugin trees pinned to the
  release, and a Homebrew formula;
- run as a local dry run, cost nothing, and start only on a tag the owner pushes.

The choice was debated between two advocates, with a blind judge:
- **A:** GoReleaser's open-source edition, one configuration per module.
- **B:** a release tool in the repository.

The judge checked the decisive claims against GoReleaser's source:
- **Prefixed tags.** The open-source edition keeps a path-prefixed tag as its
  version. `TrimPrefix(tag, "v")` leaves `discpower/v0.2.0`.
- **Validation.** It accepts such a tag only with `--skip=validate`, which also
  drops the dirty-tree and exact-tag checks. `monorepo.tag_prefix` is a Pro
  feature.
- **Homebrew.** The cask takes its version from that value with no override, so
  A would ship a cask versioned `discpower/v0.2.0`.
- **Go's VCS stamping** does not resolve subdirectory-prefixed tags
  (golang/go#71738), so the version must come from the linker.

## Decisions

1. **An in-repository release tool, `tools/release`** (B), with three mechanisms
   taken from A:
   - the release gate reuses `ci.yml` through `workflow_call`;
   - an independent `reproduce` job rebuilds and compares checksums;
   - a binary built by `go install` reads its version from
     `debug.ReadBuildInfo`.

   The cryptography is not hand-written: signatures come from `cosign`,
   provenance from `actions/attest`, and the release itself from `gh`. The tool
   writes only:
   - the cross-builds;
   - deterministic archives;
   - `SHA256SUMS`;
   - SPDX 2.3 SBOMs read from the binaries' build information;
   - the Homebrew formula;
   - the release notes.

   Its domain is pure, and `tools/archtest` keeps I/O out of it and keeps product
   modules out of its imports.
2. **Tags.** A tag reads `<module>/vMAJOR.MINOR.PATCH[-pre]`, for a module of the
   release table. `go install
   github.com/RachidChabane/spec-proof/<module>/cmd/<bin>@vX.Y.Z` resolves it.
   `format` is tagged before the modules that require it.
3. **Release table.**
   - `format`: library. Pre-flight, then a release with notes and no assets.
   - `discpower`, `alignment`, `completeness`, `s1relay`: binary and plugin. The
     full pipeline.
   - `ledger`, `verify`: binary. The full pipeline without the plugin checks.
   - `umbrella`: plugin. Pre-flight, plugin checks, and a release with notes.

   Any other prefix is refused.
4. **Pre-flight.** CI and the dry run share one rule set:
   - the tag parses and HEAD is its commit, with a clean tree;
   - the tagged `go.mod` has no `replace` and requires `format` at an existing tag;
   - `GOWORK=off go build ./...` and `GOWORK=off go test ./...` pass;
   - every generated plugin manifest carries the tag's version;
   - `script/plugin-gen` leaves no diff;
   - the module's entry in the root marketplace carries `"ref": "<tag>"`, and
     every `ref` there resolves to a tag;
   - the root `NOTICE` exists and holds a copyright holder, not the placeholder
     `[COPYRIGHT HOLDER]`;
   - in CI, `publish` refuses a private repository.
5. **Build.**
   - Environment: `GOWORK=off CGO_ENABLED=0 GOTOOLCHAIN=go1.26.4
     GOFLAGS=-mod=readonly`.
   - Command: `go build -trimpath -buildvcs=false -ldflags "-s -w -buildid= -X
     main.version=<semver> -X main.commit=<sha>"`.
   - Archives are named `<bin>_<semver>_<os>_<arch>.tar.gz`, or `.zip` for
     Windows. They hold the binary, `LICENSE`, `README.md`, `NOTICE` and
     `THIRD_PARTY_NOTICES.md`.
   - Archive entries are sorted, with fixed modes, uid and gid 0, and the
     commit's time.
6. **Workflow `release.yml`.**
   - Trigger: tags `*/v*` only. Default permissions: `{}`. Every action is pinned
     by SHA.
   - Jobs, in order:
     - `plan`;
     - `ci` (the reusable CI for `format` and the module);
     - `build`, together with `reproduce` on another runner;
     - `publish`, with `contents: write`, `id-token: write`, `attestations: write`
       and the attest action's metadata permission.
   - `publish` signs `SHA256SUMS` with `cosign sign-blob` (bundle
     `SHA256SUMS.sigstore.json`), attests provenance over `SHA256SUMS`, attests
     each archive with its SBOM, renders the formula, and creates the release.
7. **Install.** `script/install <module> <semver>` checks the signature, either
   with `cosign verify-blob` against the release workflow's identity at that tag
   or with `gh attestation verify`. It then checks the archive's checksum, and the
   binary's `--version` when the target platform is the machine's own; a binary for
   another platform (`--os`, `--arch`) is installed without being run, its version
   bound by the verified signature and checksum. Any failure, or no verifier at all, aborts the install.
8. **Local front door.** The local entry points are `script/release <tag>
   [--dry-run]`, `make release-dry-run TAG=…` and `make release TAG=…`. They run
   under `nice`, with at most two builds at once, after `script/mem-guard check`.

## Consequences

- **Releasing a module means removing its `replace` line.** The release commit
  drops `replace …/format => ../format` from that module's `go.mod` and requires a
  tagged `format`. The workspace keeps local development unchanged.
- **Blocked on going public:**
  - nothing is published while the repository is private. GitHub artifact
    attestations on a private repository need GitHub Enterprise Cloud, and a
    keyless signature writes the workflow identity to the public Rekor log.
  - the Homebrew tap needs its own repository and a token, which only the owner
    can create; until then the formula ships as a release asset.
- **Every pull request runs the dry run end to end.** It covers `tools/release`,
  the release scripts and the workflow: a throwaway tag goes through build,
  reproduction and the install from `dist/`. `actionlint` checks the workflow.
- **Residual risks:**
  - tags stay mutable unless a ruleset protects `*/v*`;
  - a marketplace `ref` bumped on `main` without its tag is caught only at the
    next release;
  - a deterministic logic error in the hand-written archive or SBOM code passes
    `reproduce`. The end-to-end install from `dist/` is the backstop.
