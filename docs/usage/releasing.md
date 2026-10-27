# Releasing

Every module is released on its own, from a tag of its own. The pipeline builds
reproducible binaries for linux, macOS and Windows on amd64 and arm64, signs and
attests them, and publishes them as a GitHub release. The decision and its
alternatives are in [ADR 0010](../adr/0010-release-pipeline.md).

## What a tag releases

A tag reads `<module>/vMAJOR.MINOR.PATCH`, with an optional `-pre` suffix and no
leading zero in a number. Any other prefix, a bare `v1.2.3` and every module under
`tools/` are refused.

| Module | Kind | The release ships |
|---|---|---|
| `format` | library | notes; the tag is the release |
| `discpower`, `alignment`, `completeness`, `s1relay` | binary and plugin | archives, checksums, signature, attestations, SBOMs, formula, installer, notes |
| `ledger`, `verify` | binary | the same, without plugin checks |
| `umbrella` | plugin | notes; the plugin files are pinned by the tag |

`format` is tagged before the modules that require it. `go install` resolves the
tag of a module in a subdirectory by its path:

```
go install github.com/RachidChabane/musthold/discpower/cmd/musthold-dp@v0.2.0
```

## Cut a release

1. Make the release commit of the module:
   - remove the `replace …/format => ../format` line from its `go.mod` and require
     the tagged `format` version;
   - set the version constant the plugin files are generated from
     (`hostfiles.PluginVersion` for `discpower`, `alignment` and `completeness`,
     `plugin.Version` for `s1relay`, `umbrella.Version` for `umbrella`);
   - run `make plugin-gen` and commit what it changes: it writes the plugin manifests
     and, in the root marketplace, the `"ref": "<module>/vX.Y.Z"` of every module
     plugin, taken from the version in that module's plugin manifest.
2. Rehearse it: `make release-dry-run TAG=<module>/vX.Y.Z`. The report lists what a
   real release would refuse. Fix those findings.
3. Tag `format` first when the module requires a newer one, push the tag and let
   its release finish.
4. Tag the release commit and push the tag. The workflow starts on the push and on
   nothing else.

A release is published only from a public repository. Tags are mutable unless a
ruleset protects `*/v*`; protect them.

## Tag the first release of every module

Every module is released at `v0.1.0`. The tags name one commit, the release commit,
which holds the `NOTICE` with its copyright holder, the `go.mod` files that require
`format v0.1.0` without a `replace` rule, and the marketplace `ref` of every module
plugin. Run the commands from a clone of the public repository whose `main` is that
commit. Each command pushes named tags; `--tags` is never used.

1. Tag `format` and rehearse it:

   ```
   git tag format/v0.1.0
   make release-dry-run TAG=format/v0.1.0
   ```

   The report lists no finding.
2. Push the tag and wait until its release workflow succeeds:

   ```
   git push origin format/v0.1.0
   ```

   Until `format/v0.1.0` exists on the remote, `GOWORK=off go build ./...` and
   `go install` fail in every other module, because its `go.mod` requires that
   version and no `replace` rule points at `../format`. The workspace (`go.work`)
   keeps development and the gates working.
3. Tag the other modules on the same commit. The marketplace pins every plugin of
   this repository to its tag, and the pre-flight of a plugin release requires each
   of those tags to exist, so all of them are created before any is pushed:

   ```
   for module in ledger verify discpower alignment completeness s1relay umbrella; do
     git tag "$module/v0.1.0"
   done
   ```
4. Rehearse each one; the report lists no finding:

   ```
   make release-dry-run TAG=discpower/v0.1.0
   ```

   Repeat for `ledger`, `verify`, `alignment`, `completeness`, `s1relay` and
   `umbrella`.
5. Push the tags in one command; each starts its own release workflow:

   ```
   git push origin ledger/v0.1.0 verify/v0.1.0 discpower/v0.1.0 alignment/v0.1.0 \
     completeness/v0.1.0 s1relay/v0.1.0 umbrella/v0.1.0
   ```

The `go.sum` of each module holds the hash of `format v0.1.0`, which covers the
files of `format/` and the root `LICENSE`. A change to either after the tag needs a
new `format` version and `go get github.com/RachidChabane/musthold/format@<version>`
in each module that requires it.

## The pre-flight

One rule set runs in the dry run, on the pull requests that touch the release
files, and in the workflow. A finding either stops every run or only a real
release.

| Rule | Checks | Stops |
|---|---|---|
| `tag-at-head` | the tag names HEAD; a tag that is missing is reported | a tag on another commit stops every run |
| `clean-tree` | the working tree has no change | a real release |
| `go-mod-releasable` | the `go.mod` has no `replace` and no `exclude` | a real release |
| `format-tagged` | the required `format` version has a `format/vX.Y.Z` tag | a real release |
| `build`, `test` | `GOWORK=off go build ./...` and `GOWORK=off go test ./...` pass | every run |
| `plugin-versions` | every plugin manifest carries the version of the tag | a real release |
| `plugin-generated` | `script/plugin-gen` changes no committed file | every run |
| `marketplace-ref` | the module's marketplace entry has the tag as its `ref` | a real release |
| `marketplace-refs-resolve` | every `ref` of this repository in the marketplace is a tag | a real release |
| `notice-holder` | the `NOTICE` file exists and no longer holds the placeholder `[COPYRIGHT HOLDER]` | a real release |
| `repository-public` | the repository is public, when the run requires it | a real release |
| `third-party-notices` | the repository root holds `THIRD_PARTY_NOTICES.md`, for a module that ships a binary | every run |

A plugin whose marketplace source is a path inside the marketplace needs no `ref`.

## Run it locally

```
make release-dry-run TAG=discpower/v0.2.0
make release TAG=discpower/v0.2.0
```

Both call `script/release`, which refuses to start when `script/mem-guard check`
does, builds `tools/release`, and runs the pre-flight, the build and the check of
`dist/` under `nice -n 10` with `GOMAXPROCS=2` and at most two targets at a time
(`RELEASE_JOBS` changes the number). `make release` leaves the release files in
`dist/` and publishes nothing. `make release-dry-run` reports the findings only a
real release needs and goes on, then installs the result from `dist/` into a
throwaway prefix and runs it. Pass options with `ARGS`:

| Option | Effect |
|---|---|
| `--skip-tests` | leaves out the Go tests and the plugin regeneration of the pre-flight |
| `--targets LIST` | builds only the `os/arch` pairs of the comma-separated list |
| `--jobs N` | builds N targets at once |
| `--dist DIR` | writes the release files to DIR |

The exit code is 0 when the run is done, 1 on a failure, 2 on a command line that
does not parse, 3 when the pre-flight refuses and 4 when `dist/` is invalid.
`script/release plan <tag>` prints what a tag releases.

`GOPRIVATE=github.com/RachidChabane/*` and git credentials let a build resolve the
modules of a private repository.

## What the workflow does

`.github/workflows/release.yml` runs on tags `*/v*`. It denies every permission by
default and pins every action by commit SHA.

| Job | Does |
|---|---|
| `plan` | prints the module, kind, version and CI modules of the tag |
| `ci` | runs the reusable CI of `.github/workflows/ci.yml` for `format` and the module |
| `build` | runs `script/release`, which runs the pre-flight, builds every target and checks `dist/` |
| `reproduce` | builds again on an arm64 runner and keeps its `SHA256SUMS` |
| `publish` | requires the two `SHA256SUMS` to match, signs, attests, renders the formula and the notes, and creates the release |

`publish` is the only job that writes. It signs `SHA256SUMS` with `cosign
sign-blob` into `SHA256SUMS.sigstore.json`, attests the build provenance of every
archive from `SHA256SUMS`, attests each archive with its SBOM, and creates the
release with `gh release create --verify-tag`.

## Build

Each binary is built with `GOWORK=off CGO_ENABLED=0 GOTOOLCHAIN=go1.26.4
GOFLAGS=-mod=readonly` and `go build -trimpath -buildvcs=false -ldflags "-s -w
-buildid= -X main.version=<semver> -X main.commit=<sha>"`. An archive is
`<bin>_<semver>_<os>_<arch>.tar.gz`, or `.zip` for Windows, and holds the binary,
`LICENSE`, `README.md`, `NOTICE` (Apache-2.0 section 4(d)) and
`THIRD_PARTY_NOTICES.md`. Its entries are sorted, with fixed modes, owner root and
the commit time. A build fails when the repository root lacks `NOTICE`, and
`verify-dist` requires all five entries in every archive. `SHA256SUMS` lists the archives, their SPDX 2.3 SBOMs
(`<archive>.spdx.json`) and the installer.

## Install a release

Install with Go, or with the verified installer published with every binary
release:

```
go install github.com/RachidChabane/musthold/ledger/cmd/musthold-ledger@v0.2.0
curl -fsSLO https://github.com/RachidChabane/musthold/releases/download/ledger/v0.2.0/install
bash install ledger 0.2.0
```

The installer is listed in `SHA256SUMS` and attested like an archive. It needs `cosign` or `gh`. It downloads the archive and `SHA256SUMS`,
verifies the signature, checks the archive against its line of `SHA256SUMS`, runs
the extracted binary with `--version` and requires exactly the version asked for.
A binary for another platform than the machine's (`--os`, `--arch`) is not run: the
installer says so on stderr, and the version is bound by the verified signature and
checksum of the release.
Any failure, or the absence of both verifiers, aborts the install and leaves
nothing installed. `--prefix DIR` sets the install prefix (`~/.local` by default).
`--from DIR` installs from a directory that holds the release files and checks the
checksum and the version only.

## Verify a release

With `cosign`, check the signature of `SHA256SUMS` against the identity of the
release workflow at the tag, then the archive against `SHA256SUMS`:

```
cosign verify-blob --bundle SHA256SUMS.sigstore.json \
  --certificate-identity https://github.com/RachidChabane/musthold/.github/workflows/release.yml@refs/tags/ledger/v0.2.0 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS
sha256sum --ignore-missing -c SHA256SUMS
```

With `gh`, verify the attestation of an archive:

```
gh attestation verify ledger_0.2.0_linux_amd64.tar.gz --repo RachidChabane/musthold \
  --signer-workflow RachidChabane/musthold/.github/workflows/release.yml
```

A binary prints exactly its version with `--version`, and its name, version and
commit with the `version` command.

## Homebrew

The release carries a formula, `<bin>.rb`, with one url and checksum per macOS and
Linux architecture. Place it in a tap to install it with `brew`. A formula, not a
cask, avoids the Gatekeeper quarantine of an unsigned macOS binary.

## Plugin trees

The plugin files are pinned by tag name: a marketplace entry carries
`"ref": "<module>/vX.Y.Z"`, because a commit cannot contain its own hash. The
pre-flight refuses a release whose manifests, generated files or marketplace entry
disagree with the tag.
