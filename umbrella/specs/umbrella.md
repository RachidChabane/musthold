# umbrella requirements

The requirements of the `umbrella` module, the marketplace and the thin plugin
that install and run the spec-proof modules together and the `spec-proof`
dispatcher, in Shallnot's Markdown spec format. `shallnot gate` traces them to the Go tests of the module in CI.

## Marketplace and thin plugin

- **SP-820~1**: WHEN the umbrella files are generated THE SYSTEM SHALL produce the
  marketplace, the manifest of the thin plugin and its skill from one definition,
  the same bytes on every run, and the committed files under `umbrella/` SHALL
  equal what the generator produces, with no other file under the marketplace
  directory of the umbrella or the tree of the thin plugin.
- **SP-821~2**: WHEN the marketplace is read against the formats Claude Code
  documents THE SYSTEM SHALL name itself `spec-proof` with an owner, and list
  each plugin once: Shallnot's plugin by its public marketplace source, the
  `plugin` directory of `RachidChabane/shallnot`; the plugins of the three proof
  modules and of the relay by the directory of their committed tree in this
  repository, each pinned by `ref` to the release tag `<module>/v<version>` of
  the version its plugin manifest carries, and refused when a module has no
  version; and the thin plugin by a relative path to its own tree.
- **SP-822~1**: WHEN the thin plugin is read THE SYSTEM SHALL name itself
  `spec-proof`, depend on Shallnot, the three module plugins and the relay, every
  one of them an entry of the marketplace other than itself, and hold exactly one
  skill, `prove-all`, with no hook, agent, command or server.
- **SP-823~2**: WHEN the `prove-all` skill is read THE SYSTEM SHALL open with the
  `doctor` of each module and name the modules that are missing, run the modules
  in the order discriminating power, alignment, completeness, advance one wave of
  each module that waits before any module starts its next, send the answering of
  every request to the fulfil skill of its module, publish the local proofs once
  every present module is done by running `spec-proof-verify --accept-local
  .spec-proof/cache --out .spec-proof/verified`, saying so when the verifier is
  missing, pre-approve only the `doctor`, `prove` and `request list` of each
  module, the `drain` of the relay and that exact verifier command and so no
  command that answers a request, never answer a request itself, and carry no text
  about its own authoring.
- **SP-906~1**: WHEN the umbrella files are generated THE SYSTEM SHALL also produce
  the marketplace at the root of the repository, which is the only one
  `/plugin marketplace add RachidChabane/spec-proof` reads, from the same definition
  as the umbrella's own: the same name, owner and plugins in the same order, with the
  thin plugin reached by `./umbrella/plugins/spec-proof` from the root, and the
  committed file SHALL equal what the generator produces, with no other file under
  the root's marketplace directory.
- **SP-824~1**: WHEN the marketplace and the skill are compared with the committed
  trees of the modules THE SYSTEM SHALL find, for each module and for the relay,
  a plugin tree whose manifest bears the name the marketplace lists, a fulfil
  skill under the name the `prove-all` skill sends the module's requests to, and,
  for a proof module, hook commands that run the binary the skill runs.

## Portable plugin

- **SP-1706~1**: WHEN the portable plugin of the umbrella is generated THE SYSTEM
  SHALL produce a root manifest valid for Agent Plugins 1.0.0, an `mcp.json` with
  no server, so that no server or skill of a module is declared twice, and the
  `prove-all` skill with no `allowed-tools` that leaves the way a ticket is
  answered to each module's fulfil skill, and the committed tree SHALL equal what
  the generator produces.

## Dispatcher

- **SP-2121~1**: WHEN `spec-proof <command>` runs THE SYSTEM SHALL run the binary
  of the module that owns the command, `spec-proof-dp` for `scan`, found beside
  the dispatcher first and on the PATH otherwise, with the same arguments and
  streams, and exit with its exit code; SHALL fail with exit 2 and the name of the
  missing binary when neither holds it; SHALL ask that binary for the help of a
  command; and SHALL print its commands for `-h`, `--help` and `help` on standard
  output with exit 0, on standard error with exit 2 for no command or an unknown
  one, and its version for `version` and `--version`.
