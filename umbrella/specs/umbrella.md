# umbrella requirements

The requirements of the `umbrella` module, the marketplace and the thin plugin
that install and run the spec-proof modules together, in Shallnot's Markdown
spec format. `shallnot gate` traces them to the Go tests of the module in CI.

## Marketplace and thin plugin

- **SP-820~1**: WHEN the umbrella files are generated THE SYSTEM SHALL produce the
  marketplace, the manifest of the thin plugin and its skill from one definition,
  the same bytes on every run, and the committed files under `umbrella/` SHALL
  equal what the generator produces, with no other file under the marketplace
  directory of the umbrella or the tree of the thin plugin.
- **SP-821~1**: WHEN the marketplace is read against the formats Claude Code
  documents THE SYSTEM SHALL name itself `spec-proof` with an owner, and list
  each plugin once: Shallnot's plugin by its public marketplace source, the
  `plugin` directory of `RachidChabane/shallnot`; the plugins of the three proof
  modules and of the relay by the directory of their committed tree in this
  repository; and the thin plugin by a relative path to its own tree.
- **SP-822~1**: WHEN the thin plugin is read THE SYSTEM SHALL name itself
  `spec-proof`, depend on Shallnot, the three module plugins and the relay, every
  one of them an entry of the marketplace other than itself, and hold exactly one
  skill, `prove-all`, with no hook, agent, command or server.
- **SP-823~1**: WHEN the `prove-all` skill is read THE SYSTEM SHALL open with the
  `doctor` of each module and name the modules that are missing, run the modules
  in the order discriminating power, alignment, completeness, advance one wave of
  each module that waits before any module starts its next, send the answering of
  every request to the fulfil skill of its module, pre-approve only the `doctor`,
  `prove` and `request list` of each module and the `drain` of the relay and so no
  command that answers a request, never answer a request itself, and carry no text
  about its own authoring.
- **SP-824~1**: WHEN the marketplace and the skill are compared with the committed
  trees of the modules THE SYSTEM SHALL find, for each module and for the relay,
  a plugin tree whose manifest bears the name the marketplace lists, a fulfil
  skill under the name the `prove-all` skill sends the module's requests to, and,
  for a proof module, hook commands that run the binary the skill runs.
