# release requirements

The release pipeline of the repository (ADR 0010), in Shallnot's Markdown spec
format: the tool that plans, checks, builds and verifies the release of one module,
the front door scripts around it and the workflow that runs it. These requirements
trace to the Go tests of this module, which build a fixture module and never call
the network.

## Tags and the release table

- **SP-1800~1**: WHEN a tag is parsed THE SYSTEM SHALL accept exactly
  `<module>/vMAJOR.MINOR.PATCH` with an optional `-pre` suffix for a module of the
  release table, with no leading zero in a number, and SHALL refuse a bare
  version, a module outside the table such as one under `tools/`, and a
  malformed version.
- **SP-1801~1**: WHEN the release table is read THE SYSTEM SHALL list `format` as
  a library, `discpower`, `alignment`, `completeness` and `s1relay` as binary and
  plugin, `ledger` and `verify` as binary and `umbrella` as plugin, each binary
  with its main package and each plugin with its marketplace name; and WHEN a
  plan is made for a tag THE SYSTEM SHALL output its module, kind, binary, version,
  what it ships and the modules whose CI gates it.
- **SP-1802~1**: WHEN two versions are compared THE SYSTEM SHALL order them by
  semantic-version precedence: the numbers first, a pre-release below its
  release, and the pre-release identifiers one by one.

## Build, archives and checksums

- **SP-1803~1**: WHEN the build matrix is read THE SYSTEM SHALL list linux, darwin
  and windows each on amd64 and arm64, and name the archive of a target
  `<bin>_<semver>_<os>_<arch>.tar.gz`, or `.zip` for windows, so that no two
  targets share a name.
- **SP-1804~1**: WHEN a binary is cross-built THE SYSTEM SHALL build outside the
  workspace with cgo off, the pinned toolchain and read-only modules, with
  `-trimpath`, no VCS stamping and link flags that strip the binary, drop the build
  id and link the version and the commit, dropping the caller's own values of
  those variables.
- **SP-1805~2**: WHEN an archive is written THE SYSTEM SHALL list the binary, the
  licence, the README, the third-party notices and, when the repository root has
  a NOTICE file, the NOTICE, sorted by name with fixed modes, owner root and the
  commit time, and no name or time in the compression header, so that writing it
  twice gives the same bytes and the extracted tree equals the entries given.
- **SP-1806~1**: WHEN checksums are rendered THE SYSTEM SHALL write
  `<sha256>  <name>` lines sorted by name that `sha256sum -c` accepts, and
  reading them back SHALL give the same list, refusing a malformed digest, a
  malformed or repeated name and a missing final line break.
- **SP-1807~1**: WHEN the SBOM of an archive is rendered THE SYSTEM SHALL give an
  SPDX 2.3 JSON document that validates against the SPDX 2.3 schema, names the main
  module, the Go standard library and every linked module with its hash, carries
  the commit time as its creation time and the archive in its namespace, and is
  the same bytes each time it is rendered.
- **SP-1808~1**: WHEN the formula of a binary release is rendered THE SYSTEM SHALL
  give a Homebrew formula with the version of the tag and one url and sha256 per
  macOS and Linux architecture, taken from SHA256SUMS, and SHALL refuse a module
  that ships no binary and a checksum list that lacks an archive.
- **SP-1809~1**: WHEN release notes are rendered THE SYSTEM SHALL list the changes
  since the previous tag and the way to install what the release ships.
- **SP-1810~1**: WHEN a fixture module is built twice for each target THE SYSTEM
  SHALL produce byte-identical archives and SBOMs.

## Pre-flight

- **SP-1811~1**: WHEN the pre-flight reads the tag THE SYSTEM SHALL find the tag at
  HEAD, finding a tag that names another commit as a stop and a missing tag or a
  changed working tree as a finding only a real release needs.
- **SP-1812~1**: WHEN the pre-flight reads the go.mod of the module THE SYSTEM
  SHALL find every replace and exclude rule, and a format requirement that no
  `format/vX.Y.Z` tag satisfies, as findings only a real release needs.
- **SP-1813~1**: WHEN the pre-flight runs the build and the tests of the module
  outside the workspace THE SYSTEM SHALL find a failure as a stop, and a skipped
  step as no finding.
- **SP-1814~1**: WHEN the module ships a plugin THE SYSTEM SHALL find a plugin
  manifest that carries another version than the tag as a finding only a real
  release needs, and a plugin generator that changes the committed files as a
  stop.
- **SP-1815~1**: WHEN the module ships a plugin THE SYSTEM SHALL find a marketplace
  entry of the module that lacks the tag as its ref, and a ref of this repository
  that is no tag, as findings only a real release needs, and SHALL leave an entry
  whose source is a path inside the marketplace without a ref.
- **SP-1816~1**: WHEN the pre-flight is told to require a public repository THE
  SYSTEM SHALL find a private or unknown visibility as a finding only a real
  release needs.
- **SP-1817~1**: WHEN a pre-flight ends THE SYSTEM SHALL refuse a real release on
  every finding with exit code 3, and a dry run only on the findings that stop it,
  printing the others as what a real release would refuse and going on.
- **SP-1824~1**: WHEN the module ships a binary THE SYSTEM SHALL find a repository
  root that lacks `THIRD_PARTY_NOTICES.md` as a finding that stops every run, and
  SHALL ask a module that ships no binary for no such file.

## Dist directory

- **SP-1818~1**: WHEN a dist directory is verified THE SYSTEM SHALL find a missing
  or extra file, a file whose digest differs from SHA256SUMS, an archive whose
  entries, modes, owner or time differ from the release, a binary not built as the
  release builds it and an SBOM that does not name its archive.
- **SP-1819~1**: WHEN the plan, preflight, build, verify-dist, formula and notes
  verbs are run THE SYSTEM SHALL exit 0 on success, 2 on a command line that does
  not parse, 3 on a refused pre-flight, 4 on an invalid dist directory and 1 on
  any other failure.

## End to end

- **SP-1820~1**: WHEN the dry run goes through a throwaway tag in a fixture
  repository THE SYSTEM SHALL plan, run the pre-flight, build every target one at
  a time, verify the dist directory, build again and find the same SHA256SUMS,
  install from the dist directory and run the installed binary, which SHALL print
  exactly the version of the tag.
- **SP-1821~1**: WHEN the installer is run THE SYSTEM SHALL refuse a tampered
  archive, a binary whose version differs from the one asked for, an unsupported
  platform and, without a dist directory, the absence of both cosign and gh, and
  SHALL leave nothing installed.
- **SP-1822~1**: WHEN the release workflow is read THE SYSTEM SHALL start on tags
  `*/v*` only, deny every permission by default, grant each job only what it names,
  pin every action by a full commit SHA, and name in the installer the certificate
  identity of that workflow file at the tag.
